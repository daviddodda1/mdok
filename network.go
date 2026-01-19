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

func isProxyContainer(c container.Summary) bool {
	if val, ok := c.Labels[proxyLabelKey]; ok {
		return strings.EqualFold(val, "true") || strings.EqualFold(val, "1") || strings.EqualFold(val, "yes")
	}

	image := strings.ToLower(c.Image)
	if strings.Contains(image, "traefik") || strings.Contains(image, "nginx") {
		return true
	}

	for _, name := range c.Names {
		lower := strings.ToLower(name)
		if strings.Contains(lower, "traefik") || strings.Contains(lower, "nginx") {
			return true
		}
	}

	return false
}

// getContainerIPs gets all IPs of containers in the same Docker networks.
// It also returns a subset of those IPs that belong to proxy containers.
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

	// Find containers on same networks and collect their IPs
	for _, c := range containers {
		if c.ID == targetContainerID {
			continue // Skip self
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
			isProxy := isProxyContainer(c)
			// Collect all IPs from this container
			for _, network := range c.NetworkSettings.Networks {
				if network.IPAddress != "" {
					containerIPs[network.IPAddress] = true
					if isProxy {
						proxyIPs[network.IPAddress] = true
					}
				}
				if network.GlobalIPv6Address != "" {
					containerIPs[network.GlobalIPv6Address] = true
					if isProxy {
						proxyIPs[network.GlobalIPv6Address] = true
					}
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

// getNetworkBreakdown collects connection info and returns classified counts
func (d *DockerClient) getNetworkBreakdown(ctx context.Context, containerID string) (interContainer, internal, internet int) {
	// Get container IPs on same networks
	containerIPs, proxyIPs, err := d.getContainerIPs(ctx, containerID)
	if err != nil {
		return 0, 0, 0
	}

	// Classify active connections
	interContainer, internal, internet, err = d.classifyConnections(ctx, containerID, containerIPs, proxyIPs)
	if err != nil {
		return 0, 0, 0
	}

	return interContainer, internal, internet
}
