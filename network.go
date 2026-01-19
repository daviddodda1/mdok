package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"net"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

// isPrivateIP checks if an IP is in private ranges (RFC1918 + others)
func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	privateBlocks := []string{
		"10.0.0.0/8",     // RFC1918
		"172.16.0.0/12",  // RFC1918
		"192.168.0.0/16", // RFC1918
		"169.254.0.0/16", // Link-local
		"fc00::/7",       // IPv6 Unique Local
		"fe80::/10",      // IPv6 Link-local
	}

	for _, block := range privateBlocks {
		_, subnet, _ := net.ParseCIDR(block)
		if subnet != nil && subnet.Contains(ip) {
			return true
		}
	}

	return false
}

const proxyLabelKey = "mdok.proxy"

// proxyImagePatterns are image substrings that indicate a proxy container
// These are reverse proxies / API gateways that route traffic to the internet
var proxyImagePatterns = []string{
	"traefik",
	"nginx",
	"caddy",
	"haproxy",
	"envoy",
	"litellm", // LLM API proxy (OpenAI, Anthropic, etc.)
}

func isProxyContainer(c container.Summary) bool {
	// Explicit label takes precedence (can also be used to exclude with "false")
	if val, ok := c.Labels[proxyLabelKey]; ok {
		// Return false for explicit "false"/"no"/"0" to allow excluding containers
		if strings.EqualFold(val, "false") || strings.EqualFold(val, "no") || strings.EqualFold(val, "0") {
			return false
		}
		return strings.EqualFold(val, "true") || strings.EqualFold(val, "1") || strings.EqualFold(val, "yes")
	}

	image := strings.ToLower(c.Image)
	for _, pattern := range proxyImagePatterns {
		if strings.Contains(image, pattern) {
			return true
		}
	}

	for _, name := range c.Names {
		lower := strings.ToLower(name)
		for _, pattern := range proxyImagePatterns {
			if strings.Contains(lower, pattern) {
				return true
			}
		}
	}

	return false
}

// getContainerIPs gets all IPs of containers in the same Docker networks.
// It also returns ALL proxy container IPs (regardless of network) so that
// traffic through proxies on different networks is correctly classified.
func (d *DockerClient) getContainerIPs(ctx context.Context, targetContainerID string) (map[string]bool, map[string]bool, error) {
	containerIPs := make(map[string]bool)
	proxyIPs := make(map[string]bool)

	// Get target container's networks
	targetInfo, err := d.cli.ContainerInspect(ctx, targetContainerID)
	if err != nil {
		return containerIPs, proxyIPs, err
	}

	// Get all network IDs the target is connected to
	targetNetworks := make(map[string]bool)
	if targetInfo.NetworkSettings != nil {
		for netName := range targetInfo.NetworkSettings.Networks {
			targetNetworks[netName] = true
		}
	}

	// List all containers
	containers, err := d.cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return containerIPs, proxyIPs, err
	}

	// First pass: collect ALL proxy IPs (regardless of network)
	// This ensures traffic to proxies on different networks is classified as internet
	for _, c := range containers {
		if c.ID == targetContainerID {
			continue
		}

		if isProxyContainer(c) {
			for _, network := range c.NetworkSettings.Networks {
				if network.IPAddress != "" {
					proxyIPs[network.IPAddress] = true
				}
				if network.GlobalIPv6Address != "" {
					proxyIPs[network.GlobalIPv6Address] = true
				}
			}
		}
	}

	// Second pass: collect IPs of non-proxy containers on same networks
	for _, c := range containers {
		if c.ID == targetContainerID {
			continue
		}

		// Skip proxies - they're already handled and shouldn't count as inter-container
		if isProxyContainer(c) {
			continue
		}

		// Check if this container shares any networks
		sharesNetwork := false
		for netName := range c.NetworkSettings.Networks {
			if targetNetworks[netName] {
				sharesNetwork = true
				break
			}
		}

		if sharesNetwork {
			for _, network := range c.NetworkSettings.Networks {
				if network.IPAddress != "" {
					containerIPs[network.IPAddress] = true
				}
				if network.GlobalIPv6Address != "" {
					containerIPs[network.GlobalIPv6Address] = true
				}
			}
		}
	}

	return containerIPs, proxyIPs, nil
}

// parseHexIP parses a hex IP address from /proc/net/tcp format
func parseHexIP(hexIP string) net.IP {
	// Format is little-endian hex, e.g., "0100007F" for 127.0.0.1
	if len(hexIP) == 8 {
		// IPv4
		val, _ := strconv.ParseUint(hexIP, 16, 32)
		ip := make(net.IP, 4)
		binary.LittleEndian.PutUint32(ip, uint32(val))
		return ip
	} else if len(hexIP) == 32 {
		// IPv6
		ip := make(net.IP, 16)
		for i := 0; i < 4; i++ {
			val, _ := strconv.ParseUint(hexIP[i*8:(i+1)*8], 16, 32)
			binary.LittleEndian.PutUint32(ip[i*4:], uint32(val))
		}
		return ip
	}
	return nil
}

// classifyConnections reads /proc/net/tcp and /proc/net/tcp6 from a container
// and classifies connections by destination
func (d *DockerClient) classifyConnections(ctx context.Context, containerID string, containerIPs map[string]bool, proxyIPs map[string]bool) (interContainer, internal, internet int, err error) {
	// Read both IPv4 and IPv6 connection tables
	for _, file := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		counts, err := d.readProcNetFile(ctx, containerID, file, containerIPs, proxyIPs)
		if err != nil {
			continue // Silently skip if file not readable
		}
		interContainer += counts[0]
		internal += counts[1]
		internet += counts[2]
	}

	return interContainer, internal, internet, nil
}

// readProcNetFile reads a /proc/net/tcp* file and classifies connections
func (d *DockerClient) readProcNetFile(ctx context.Context, containerID string, procFile string, containerIPs map[string]bool, proxyIPs map[string]bool) ([3]int, error) {
	var counts [3]int // [interContainer, internal, internet]

	// Execute cat to read the proc file using Docker exec
	execConfig := types.ExecConfig{
		Cmd:          []string{"cat", procFile},
		AttachStdout: true,
		AttachStderr: true,
	}

	execID, err := d.cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return counts, err
	}

	resp, err := d.cli.ContainerExecAttach(ctx, execID.ID, types.ExecStartCheck{})
	if err != nil {
		return counts, err
	}
	defer resp.Close()

	scanner := bufio.NewScanner(resp.Reader)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		if lineNum == 1 {
			continue // Skip header
		}

		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		// Field 2 is remote_address in format "IP:PORT"
		remoteAddr := fields[2]
		parts := strings.Split(remoteAddr, ":")
		if len(parts) != 2 {
			continue
		}

		hexIP := parts[0]
		ip := parseHexIP(hexIP)
		if ip == nil {
			continue
		}

		// Skip if destination is 0.0.0.0 (unconnected socket)
		if ip.IsUnspecified() {
			continue
		}

		// Classify the destination
		ipStr := ip.String()
		if proxyIPs[ipStr] {
			counts[2]++ // Internet via proxy
		} else if containerIPs[ipStr] {
			counts[0]++ // Inter-container
		} else if isPrivateIP(ip) {
			counts[1]++ // Internal/private
		} else {
			counts[2]++ // Internet
		}
	}

	return counts, nil
}

// NetworkStats contains both connection counts and byte counts
type NetworkStats struct {
	// Connection counts (from /proc/net/tcp)
	ConnInterContainer int
	ConnInternal       int
	ConnInternet       int

	// Byte counts (from conntrack, if available)
	BytesInterContainer uint64
	BytesInternal       uint64
	BytesInternet       uint64
	BytesSource         string // "conntrack" or "estimated"
}

// getNetworkBreakdown collects connection info and returns classified counts
func (d *DockerClient) getNetworkBreakdown(ctx context.Context, containerID string) (interContainer, internal, internet int) {
	stats := d.getNetworkStats(ctx, containerID)
	return stats.ConnInterContainer, stats.ConnInternal, stats.ConnInternet
}

// getNetworkStats collects both connection counts and byte counts
func (d *DockerClient) getNetworkStats(ctx context.Context, containerID string) NetworkStats {
	var stats NetworkStats

	// Get container IPs on same networks
	containerIPs, proxyIPs, err := d.getContainerIPs(ctx, containerID)
	if err != nil {
		return stats
	}

	// Get this container's own IPs for conntrack filtering
	selfIPs, err := d.getContainerSelfIPs(ctx, containerID)
	if err != nil {
		selfIPs = make(map[string]bool)
	}

	// Try conntrack first for byte counts
	byteStats, conntrackErr := d.readConntrackBytes(ctx, containerID, containerIPs, proxyIPs, selfIPs)
	if conntrackErr == nil && (byteStats[0]+byteStats[1]+byteStats[2]) > 0 {
		stats.BytesInterContainer = byteStats[0]
		stats.BytesInternal = byteStats[1]
		stats.BytesInternet = byteStats[2]
		stats.BytesSource = "conntrack"
	}

	// Always get connection counts (faster, always available)
	stats.ConnInterContainer, stats.ConnInternal, stats.ConnInternet, _ = d.classifyConnections(ctx, containerID, containerIPs, proxyIPs)

	// If conntrack failed, estimate bytes from connection ratios
	if stats.BytesSource == "" && (stats.ConnInterContainer+stats.ConnInternal+stats.ConnInternet) > 0 {
		stats.BytesSource = "estimated"
		// Byte estimation will be done in the caller using total network bytes
	}

	return stats
}

// getContainerSelfIPs returns all IPs assigned to a container
func (d *DockerClient) getContainerSelfIPs(ctx context.Context, containerID string) (map[string]bool, error) {
	selfIPs := make(map[string]bool)

	info, err := d.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return selfIPs, err
	}

	if info.NetworkSettings != nil {
		for _, network := range info.NetworkSettings.Networks {
			if network.IPAddress != "" {
				selfIPs[network.IPAddress] = true
			}
			if network.GlobalIPv6Address != "" {
				selfIPs[network.GlobalIPv6Address] = true
			}
		}
	}

	return selfIPs, nil
}

// readConntrackBytes reads /proc/net/nf_conntrack and sums bytes by destination class
func (d *DockerClient) readConntrackBytes(ctx context.Context, containerID string, containerIPs, proxyIPs, selfIPs map[string]bool) ([3]uint64, error) {
	var bytes [3]uint64 // [interContainer, internal, internet]

	// Try reading conntrack from container
	// Note: This requires the container to have access to conntrack (CAP_NET_ADMIN or host netns)
	execConfig := types.ExecConfig{
		Cmd:          []string{"cat", "/proc/net/nf_conntrack"},
		AttachStdout: true,
		AttachStderr: true,
	}

	execID, err := d.cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return bytes, err
	}

	resp, err := d.cli.ContainerExecAttach(ctx, execID.ID, types.ExecStartCheck{})
	if err != nil {
		return bytes, err
	}
	defer resp.Close()

	scanner := bufio.NewScanner(resp.Reader)

	for scanner.Scan() {
		line := scanner.Text()

		// Parse conntrack line format:
		// ipv4 2 tcp 6 431999 ESTABLISHED src=172.18.0.5 dst=172.18.0.3 sport=45678 dport=5432 packets=100 bytes=12345 ...
		if !strings.Contains(line, "src=") || !strings.Contains(line, "bytes=") {
			continue
		}

		// Extract source IP (to filter to this container's connections)
		srcIP := extractConntrackField(line, "src=")
		if srcIP == "" || !selfIPs[srcIP] {
			continue // Not from this container
		}

		// Extract destination IP
		dstIP := extractConntrackField(line, "dst=")
		if dstIP == "" {
			continue
		}

		// Extract bytes (first occurrence is outbound bytes)
		bytesStr := extractConntrackField(line, "bytes=")
		if bytesStr == "" {
			continue
		}
		byteCount, err := strconv.ParseUint(bytesStr, 10, 64)
		if err != nil {
			continue
		}

		// Classify destination
		ip := net.ParseIP(dstIP)
		if ip == nil {
			continue
		}

		if proxyIPs[dstIP] {
			bytes[2] += byteCount // Internet via proxy
		} else if containerIPs[dstIP] {
			bytes[0] += byteCount // Inter-container
		} else if isPrivateIP(ip) {
			bytes[1] += byteCount // Internal/private
		} else {
			bytes[2] += byteCount // Internet
		}
	}

	return bytes, nil
}

// extractConntrackField extracts a field value from conntrack line (e.g., "src=" -> IP)
func extractConntrackField(line, prefix string) string {
	idx := strings.Index(line, prefix)
	if idx == -1 {
		return ""
	}

	start := idx + len(prefix)
	end := start
	for end < len(line) && line[end] != ' ' && line[end] != '\t' {
		end++
	}

	return line[start:end]
}
