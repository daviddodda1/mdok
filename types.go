package main

import "time"

// Config represents a monitoring configuration
type Config struct {
	Name       string   `json:"name"`
	Containers []string `json:"containers"`
	Interval   int      `json:"interval"` // seconds
	CreatedAt  string   `json:"created_at"`
}

// HostInfo contains information about the host system
type HostInfo struct {
	Hostname     string `json:"hostname"`
	CPUModel     string `json:"cpu_model"`
	CPUCores     int    `json:"cpu_cores"`
	MemoryTotal  uint64 `json:"memory_total"`
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
	KernelVer    string `json:"kernel_version"`
	DockerVer    string `json:"docker_version"`
}

// ContainerLimits represents resource limits for a container
type ContainerLimits struct {
	CPUQuota   int64  `json:"cpu_quota"`
	CPUPeriod  int64  `json:"cpu_period"`
	CPUShares  int64  `json:"cpu_shares"`
	MemLimit   uint64 `json:"memory_limit"`
	MemSwap    int64  `json:"memory_swap"`
	PidsLimit  int64  `json:"pids_limit"`
}

// Sample represents a single metric snapshot
type Sample struct {
	Timestamp       time.Time `json:"timestamp"`
	CPUPercent      float64   `json:"cpu_percent"`
	MemoryUsage     uint64    `json:"memory_usage"`
	MemoryPercent   float64   `json:"memory_percent"`
	MemoryCache     uint64    `json:"memory_cache"`
	NetRxBytes      uint64    `json:"net_rx_bytes"`
	NetTxBytes      uint64    `json:"net_tx_bytes"`
	NetRxRate       float64   `json:"net_rx_rate"`       // bytes/sec
	NetTxRate       float64   `json:"net_tx_rate"`       // bytes/sec
	BlockRead       uint64    `json:"block_read"`
	BlockWrite      uint64    `json:"block_write"`
	BlockReadRate   float64   `json:"block_read_rate"`   // bytes/sec
	BlockWriteRate  float64   `json:"block_write_rate"`  // bytes/sec
	PidsCount       uint64    `json:"pids_count"`

	// Network connection breakdown (approximate)
	NetConnInterContainer int `json:"net_conn_inter_container,omitempty"` // Connections to other containers
	NetConnInternal       int `json:"net_conn_internal,omitempty"`        // Connections to internal/private IPs
	NetConnInternet       int `json:"net_conn_internet,omitempty"`        // Connections to public IPs
}

// Summary contains calculated statistics for a metric
type Summary struct {
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	Avg   float64 `json:"avg"`
	P95   float64 `json:"p95"`
	P99   float64 `json:"p99"`
	Total float64 `json:"total,omitempty"` // for cumulative metrics like network
}

// NetworkBreakdown contains estimated traffic distribution
type NetworkBreakdown struct {
	InterContainerPct float64 `json:"inter_container_pct"` // Estimated % to other containers
	InternalPct       float64 `json:"internal_pct"`        // Estimated % to internal/private IPs
	InternetPct       float64 `json:"internet_pct"`        // Estimated % to public internet
}

// ContainerSummary contains all summaries for a container
type ContainerSummary struct {
	CPUPercent    Summary `json:"cpu_percent"`
	MemoryUsage   Summary `json:"memory_usage"`
	MemoryPercent Summary `json:"memory_percent"`
	NetRxRate     Summary `json:"net_rx_rate"`
	NetTxRate     Summary `json:"net_tx_rate"`
	NetRxTotal    uint64  `json:"net_rx_total"`
	NetTxTotal    uint64  `json:"net_tx_total"`
	BlockRead     Summary `json:"block_read_rate"`
	BlockWrite    Summary `json:"block_write_rate"`
	BlockReadTotal  uint64 `json:"block_read_total"`
	BlockWriteTotal uint64 `json:"block_write_total"`
	PidsCount     Summary `json:"pids_count"`
	SampleCount   int     `json:"sample_count"`
	Duration      string  `json:"duration"`
	Warnings      []string `json:"warnings,omitempty"`
	NetworkBreakdown *NetworkBreakdown `json:"network_breakdown,omitempty"` // Traffic distribution estimate
}

// NetworkCostEstimate contains AWS data transfer cost estimates
type NetworkCostEstimate struct {
	Region           string  `json:"region"`
	EgressGB         float64 `json:"egress_gb"`
	IngressGB        float64 `json:"ingress_gb"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
	PricePerGB       float64 `json:"price_per_gb"`
	Notes            string  `json:"notes,omitempty"`
}

// InstanceRecommendation contains AWS instance type suggestions
type InstanceRecommendation struct {
	InstanceType  string  `json:"instance_type"`
	VCPU          int     `json:"vcpu"`
	MemoryGB      float64 `json:"memory_gb"`
	Reason        string  `json:"reason"`
	HourlyPrice   float64 `json:"hourly_price_usd,omitempty"`
	Architecture  string  `json:"architecture,omitempty"` // "x86" or "arm"
}

// ContainerData represents the full metrics file structure for a container
type ContainerData struct {
	ContainerID   string              `json:"container_id"`
	ContainerName string              `json:"container_name"`
	ImageName     string              `json:"image_name"`
	Host          HostInfo            `json:"host"`
	Limits        ContainerLimits     `json:"limits"`
	StartTime     time.Time           `json:"start_time"`
	EndTime       time.Time           `json:"end_time,omitempty"`
	Interval      int                 `json:"interval_seconds"`
	Samples       []Sample            `json:"samples"`
	Summary       *ContainerSummary   `json:"summary,omitempty"`
	NetworkCost   *NetworkCostEstimate `json:"network_cost,omitempty"`
	Recommendation *InstanceRecommendation `json:"recommendation,omitempty"`
}

// MonitoringSession represents an active monitoring session
type MonitoringSession struct {
	ConfigName string
	Config     Config
	StartTime  time.Time
	PID        int
	DataDir    string
	LogFile    string
}

// DaemonStatus represents the status of a daemon instance
type DaemonStatus struct {
	ConfigName string    `json:"config_name"`
	PID        int       `json:"pid"`
	StartTime  time.Time `json:"start_time"`
	Running    bool      `json:"running"`
	Containers []string  `json:"containers"`
}

// ContainerInfo represents basic container information for selection
type ContainerInfo struct {
	ID      string
	Name    string
	Image   string
	Status  string
	Created time.Time
}

// ExportOptions contains options for exporting data
type ExportOptions struct {
	Format   string    // json, csv, markdown, html
	Last     string    // duration like "1h", "30m"
	From     time.Time
	To       time.Time
	All      bool
	Output   string    // output file path
}
