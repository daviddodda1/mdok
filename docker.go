package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// DockerClient wraps the Docker API client
type DockerClient struct {
	cli *client.Client
}

// NewDockerClient creates a new Docker client
func NewDockerClient() (*DockerClient, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}
	return &DockerClient{cli: cli}, nil
}

// Close closes the Docker client connection
func (d *DockerClient) Close() error {
	return d.cli.Close()
}

// ListContainers returns a list of running containers
func (d *DockerClient) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	containers, err := d.cli.ContainerList(ctx, container.ListOptions{All: false})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var result []ContainerInfo
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		result = append(result, ContainerInfo{
			ID:      c.ID[:12],
			Name:    name,
			Image:   c.Image,
			Status:  c.Status,
			Created: time.Unix(c.Created, 0),
		})
	}
	return result, nil
}

// GetContainerByName finds a container by name or ID prefix
func (d *DockerClient) GetContainerByName(ctx context.Context, nameOrID string) (*ContainerInfo, error) {
	containers, err := d.ListContainers(ctx)
	if err != nil {
		return nil, err
	}

	for _, c := range containers {
		if c.Name == nameOrID || c.ID == nameOrID || strings.HasPrefix(c.ID, nameOrID) {
			return &c, nil
		}
	}
	return nil, fmt.Errorf("container not found: %s", nameOrID)
}

// GetContainerLimits retrieves resource limits for a container
func (d *DockerClient) GetContainerLimits(ctx context.Context, containerID string) (ContainerLimits, error) {
	inspect, err := d.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return ContainerLimits{}, fmt.Errorf("failed to inspect container: %w", err)
	}

	var pidsLimit int64
	if inspect.HostConfig.PidsLimit != nil {
		pidsLimit = *inspect.HostConfig.PidsLimit
	}

	limits := ContainerLimits{
		CPUQuota:  inspect.HostConfig.CPUQuota,
		CPUPeriod: inspect.HostConfig.CPUPeriod,
		CPUShares: inspect.HostConfig.CPUShares,
		MemLimit:  uint64(inspect.HostConfig.Memory),
		MemSwap:   inspect.HostConfig.MemorySwap,
		PidsLimit: pidsLimit,
	}
	return limits, nil
}

// GetContainerFullID returns the full container ID from a short ID or name
func (d *DockerClient) GetContainerFullID(ctx context.Context, nameOrID string) (string, error) {
	inspect, err := d.cli.ContainerInspect(ctx, nameOrID)
	if err != nil {
		return "", fmt.Errorf("failed to inspect container: %w", err)
	}
	return inspect.ID, nil
}

// GetContainerImage returns the image name for a container
func (d *DockerClient) GetContainerImage(ctx context.Context, containerID string) (string, error) {
	inspect, err := d.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", fmt.Errorf("failed to inspect container: %w", err)
	}
	return inspect.Config.Image, nil
}

// GetHostInfo retrieves host system information
func (d *DockerClient) GetHostInfo(ctx context.Context) (HostInfo, error) {
	info, err := d.cli.Info(ctx)
	if err != nil {
		return HostInfo{}, fmt.Errorf("failed to get Docker info: %w", err)
	}

	version, err := d.cli.ServerVersion(ctx)
	if err != nil {
		return HostInfo{}, fmt.Errorf("failed to get Docker version: %w", err)
	}

	// Get CPU model from /proc/cpuinfo on Linux
	cpuModel := getCPUModel()

	return HostInfo{
		Hostname:     info.Name,
		CPUModel:     cpuModel,
		CPUCores:     info.NCPU,
		MemoryTotal:  uint64(info.MemTotal),
		Architecture: info.Architecture,
		OS:           info.OperatingSystem,
		KernelVer:    info.KernelVersion,
		DockerVer:    version.Version,
	}, nil
}

// getCPUModel attempts to get CPU model from /proc/cpuinfo
func getCPUModel() string {
	if runtime.GOOS != "linux" {
		return "unknown"
	}

	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return "unknown"
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "model name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return "unknown"
}

// StatsResult contains parsed container stats
type StatsResult struct {
	Sample       Sample
	PrevCPU      uint64
	PrevSystem   uint64
	PrevNetRx    uint64
	PrevNetTx    uint64
	PrevBlockRd  uint64
	PrevBlockWr  uint64
	Error        error
}

// CollectStats collects a single stats sample from a container
func (d *DockerClient) CollectStats(ctx context.Context, containerID string, prev *StatsResult) (*StatsResult, error) {
	stats, err := d.cli.ContainerStats(ctx, containerID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get container stats: %w", err)
	}
	defer stats.Body.Close()

	var statsJSON types.StatsJSON
	if err := json.NewDecoder(stats.Body).Decode(&statsJSON); err != nil {
		return nil, fmt.Errorf("failed to decode stats: %w", err)
	}

	result := &StatsResult{
		Sample: Sample{
			Timestamp: time.Now(),
		},
	}

	// Calculate CPU percentage
	cpuDelta := float64(statsJSON.CPUStats.CPUUsage.TotalUsage - statsJSON.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(statsJSON.CPUStats.SystemUsage - statsJSON.PreCPUStats.SystemUsage)
	numCPUs := float64(statsJSON.CPUStats.OnlineCPUs)
	if numCPUs == 0 {
		numCPUs = float64(len(statsJSON.CPUStats.CPUUsage.PercpuUsage))
	}
	if numCPUs == 0 {
		numCPUs = 1
	}

	if systemDelta > 0 && cpuDelta > 0 {
		result.Sample.CPUPercent = (cpuDelta / systemDelta) * numCPUs * 100.0
	}

	// Memory stats
	result.Sample.MemoryUsage = statsJSON.MemoryStats.Usage
	if statsJSON.MemoryStats.Stats != nil {
		if cache, ok := statsJSON.MemoryStats.Stats["cache"]; ok {
			result.Sample.MemoryCache = cache
		}
	}
	if statsJSON.MemoryStats.Limit > 0 {
		result.Sample.MemoryPercent = float64(statsJSON.MemoryStats.Usage) / float64(statsJSON.MemoryStats.Limit) * 100.0
	}

	// Network stats (sum all interfaces)
	var netRx, netTx uint64
	for _, netStats := range statsJSON.Networks {
		netRx += netStats.RxBytes
		netTx += netStats.TxBytes
	}
	result.Sample.NetRxBytes = netRx
	result.Sample.NetTxBytes = netTx
	result.PrevNetRx = netRx
	result.PrevNetTx = netTx

	// Calculate network rates if we have previous data
	if prev != nil && prev.PrevNetRx > 0 {
		elapsed := result.Sample.Timestamp.Sub(prev.Sample.Timestamp).Seconds()
		if elapsed > 0 {
			result.Sample.NetRxRate = float64(netRx-prev.PrevNetRx) / elapsed
			result.Sample.NetTxRate = float64(netTx-prev.PrevNetTx) / elapsed
		}
	}

	// Block I/O stats
	var blockRead, blockWrite uint64
	for _, bioEntry := range statsJSON.BlkioStats.IoServiceBytesRecursive {
		switch bioEntry.Op {
		case "read", "Read":
			blockRead += bioEntry.Value
		case "write", "Write":
			blockWrite += bioEntry.Value
		}
	}
	result.Sample.BlockRead = blockRead
	result.Sample.BlockWrite = blockWrite
	result.PrevBlockRd = blockRead
	result.PrevBlockWr = blockWrite

	// Calculate block I/O rates if we have previous data
	if prev != nil && prev.PrevBlockRd > 0 {
		elapsed := result.Sample.Timestamp.Sub(prev.Sample.Timestamp).Seconds()
		if elapsed > 0 {
			result.Sample.BlockReadRate = float64(blockRead-prev.PrevBlockRd) / elapsed
			result.Sample.BlockWriteRate = float64(blockWrite-prev.PrevBlockWr) / elapsed
		}
	}

	// PIDs
	result.Sample.PidsCount = statsJSON.PidsStats.Current

	// Network breakdown (classify active connections and bytes)
	// This is best-effort and may fail silently
	netStats := d.getNetworkStats(ctx, containerID)
	result.Sample.NetConnInterContainer = netStats.ConnInterContainer
	result.Sample.NetConnInternal = netStats.ConnInternal
	result.Sample.NetConnInternet = netStats.ConnInternet
	result.Sample.NetBytesInterContainer = netStats.BytesInterContainer
	result.Sample.NetBytesInternal = netStats.BytesInternal
	result.Sample.NetBytesInternet = netStats.BytesInternet
	result.Sample.NetBytesSource = netStats.BytesSource

	// Store CPU values for next calculation
	result.PrevCPU = statsJSON.CPUStats.CPUUsage.TotalUsage
	result.PrevSystem = statsJSON.CPUStats.SystemUsage

	return result, nil
}

// IsContainerRunning checks if a container is still running
func (d *DockerClient) IsContainerRunning(ctx context.Context, containerID string) (bool, error) {
	inspect, err := d.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return false, err
	}
	return inspect.State.Running, nil
}
