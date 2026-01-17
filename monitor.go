package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Monitor handles the monitoring loop for containers
type Monitor struct {
	config        Config
	docker        *DockerClient
	containerData map[string]*ContainerData
	prevStats     map[string]*StatsResult
	mu            sync.Mutex
	stopChan      chan struct{}
	logger        *log.Logger
}

// NewMonitor creates a new monitor instance
func NewMonitor(config Config, logger *log.Logger) (*Monitor, error) {
	docker, err := NewDockerClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	return &Monitor{
		config:        config,
		docker:        docker,
		containerData: make(map[string]*ContainerData),
		prevStats:     make(map[string]*StatsResult),
		stopChan:      make(chan struct{}),
		logger:        logger,
	}, nil
}

// RunMonitor runs the monitor in foreground mode
func RunMonitor(config Config) error {
	logger := log.New(os.Stdout, "", log.LstdFlags)
	monitor, err := NewMonitor(config, logger)
	if err != nil {
		return err
	}

	return monitor.Run()
}

// Run starts the monitoring loop
func (m *Monitor) Run() error {
	ctx := context.Background()

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Initialize container data
	if err := m.initializeContainers(ctx); err != nil {
		return err
	}

	m.logger.Printf("Starting monitoring for %d containers (interval: %ds)\n",
		len(m.config.Containers), m.config.Interval)

	ticker := time.NewTicker(time.Duration(m.config.Interval) * time.Second)
	defer ticker.Stop()

	// Initial collection
	m.collectAllStats(ctx)

	for {
		select {
		case <-ticker.C:
			m.collectAllStats(ctx)
		case <-sigChan:
			m.logger.Println("Received shutdown signal")
			m.shutdown()
			return nil
		case <-m.stopChan:
			m.logger.Println("Stop requested")
			m.shutdown()
			return nil
		}
	}
}

// Stop signals the monitor to stop
func (m *Monitor) Stop() {
	close(m.stopChan)
}

// initializeContainers sets up initial container data structures
func (m *Monitor) initializeContainers(ctx context.Context) error {
	hostInfo, err := m.docker.GetHostInfo(ctx)
	if err != nil {
		m.logger.Printf("Warning: failed to get host info: %v\n", err)
		hostInfo = HostInfo{}
	}

	for _, containerName := range m.config.Containers {
		// Get full container ID
		fullID, err := m.docker.GetContainerFullID(ctx, containerName)
		if err != nil {
			m.logger.Printf("Warning: container %s not found: %v\n", containerName, err)
			continue
		}

		// Get container limits
		limits, err := m.docker.GetContainerLimits(ctx, fullID)
		if err != nil {
			m.logger.Printf("Warning: failed to get limits for %s: %v\n", containerName, err)
		}

		// Get image name
		imageName, err := m.docker.GetContainerImage(ctx, fullID)
		if err != nil {
			imageName = "unknown"
		}

		m.containerData[containerName] = &ContainerData{
			ContainerID:   fullID,
			ContainerName: containerName,
			ImageName:     imageName,
			Host:          hostInfo,
			Limits:        limits,
			StartTime:     time.Now(),
			Interval:      m.config.Interval,
			Samples:       make([]Sample, 0),
		}

		m.logger.Printf("Initialized monitoring for container: %s (%s)\n", containerName, fullID[:12])
	}

	return nil
}

// collectAllStats collects stats from all containers
func (m *Monitor) collectAllStats(ctx context.Context) {
	var wg sync.WaitGroup

	for _, containerName := range m.config.Containers {
		if m.containerData[containerName] == nil {
			continue
		}

		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			m.collectContainerStats(ctx, name)
		}(containerName)
	}

	wg.Wait()

	// Save data periodically (every collection)
	m.saveData()
}

// collectContainerStats collects stats for a single container
func (m *Monitor) collectContainerStats(ctx context.Context, containerName string) {
	m.mu.Lock()
	data := m.containerData[containerName]
	prev := m.prevStats[containerName]
	m.mu.Unlock()

	if data == nil {
		return
	}

	// Check if container is still running
	running, err := m.docker.IsContainerRunning(ctx, data.ContainerID)
	if err != nil || !running {
		m.logger.Printf("Container %s is no longer running\n", containerName)
		return
	}

	// Collect stats
	stats, err := m.docker.CollectStats(ctx, data.ContainerID, prev)
	if err != nil {
		m.logger.Printf("Error collecting stats for %s: %v\n", containerName, err)
		return
	}

	m.mu.Lock()
	m.prevStats[containerName] = stats
	m.containerData[containerName].Samples = append(m.containerData[containerName].Samples, stats.Sample)
	m.mu.Unlock()

	m.logger.Printf("[%s] CPU: %.1f%% | Mem: %s (%.1f%%) | Net rx/tx: %s/%s\n",
		containerName,
		stats.Sample.CPUPercent,
		formatBytes(stats.Sample.MemoryUsage),
		stats.Sample.MemoryPercent,
		formatBytes(uint64(stats.Sample.NetRxRate)),
		formatBytes(uint64(stats.Sample.NetTxRate)))
}

// saveData saves all container data to disk
func (m *Monitor) saveData() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, data := range m.containerData {
		if data == nil || len(data.Samples) == 0 {
			continue
		}

		data.EndTime = time.Now()

		if err := SaveContainerData(m.config.Name, data); err != nil {
			m.logger.Printf("Error saving data for %s: %v\n", data.ContainerName, err)
		}
	}
}

// shutdown performs cleanup and final summary generation
func (m *Monitor) shutdown() {
	m.logger.Println("Generating final summary...")

	m.mu.Lock()
	for _, data := range m.containerData {
		if data == nil || len(data.Samples) == 0 {
			continue
		}

		data.EndTime = time.Now()

		// Calculate summary statistics
		data.Summary = CalculateSummary(data.Samples)

		// Calculate network cost estimates
		data.NetworkCost = CalculateNetworkCost(data.Summary.NetTxTotal)

		// Generate instance recommendation (default to x86 for backward compatibility)
		data.Recommendation = RecommendInstance(data.Summary, "x86")

		// Detect warnings
		data.Summary.Warnings = DetectWarnings(data)

		// Set duration
		data.Summary.Duration = data.EndTime.Sub(data.StartTime).Round(time.Second).String()

		if err := SaveContainerData(m.config.Name, data); err != nil {
			m.logger.Printf("Error saving final data for %s: %v\n", data.ContainerName, err)
		}

		m.logger.Printf("Saved summary for %s (%d samples)\n", data.ContainerName, len(data.Samples))
	}
	m.mu.Unlock()

	m.docker.Close()
	m.logger.Println("Monitoring stopped")
}

// GetContainerData returns the current container data (for dashboard)
func (m *Monitor) GetContainerData() map[string]*ContainerData {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Return a copy
	result := make(map[string]*ContainerData)
	for k, v := range m.containerData {
		result[k] = v
	}
	return result
}
