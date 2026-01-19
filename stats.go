package main

import (
	"fmt"
	"math"
	"sort"
)

// CalculateSummary calculates summary statistics from samples
func CalculateSummary(samples []Sample) *ContainerSummary {
	if len(samples) == 0 {
		return nil
	}

	summary := &ContainerSummary{
		SampleCount: len(samples),
	}

	// Collect values for each metric
	cpuValues := make([]float64, len(samples))
	memUsageValues := make([]float64, len(samples))
	memPercentValues := make([]float64, len(samples))
	netRxRateValues := make([]float64, len(samples))
	netTxRateValues := make([]float64, len(samples))
	blockReadRateValues := make([]float64, len(samples))
	blockWriteRateValues := make([]float64, len(samples))
	pidsValues := make([]float64, len(samples))

	for i, s := range samples {
		cpuValues[i] = s.CPUPercent
		memUsageValues[i] = float64(s.MemoryUsage)
		memPercentValues[i] = s.MemoryPercent
		netRxRateValues[i] = s.NetRxRate
		netTxRateValues[i] = s.NetTxRate
		blockReadRateValues[i] = s.BlockReadRate
		blockWriteRateValues[i] = s.BlockWriteRate
		pidsValues[i] = float64(s.PidsCount)
	}

	// Calculate summaries
	summary.CPUPercent = calculateStats(cpuValues)
	summary.MemoryUsage = calculateStats(memUsageValues)
	summary.MemoryPercent = calculateStats(memPercentValues)
	summary.NetRxRate = calculateStats(netRxRateValues)
	summary.NetTxRate = calculateStats(netTxRateValues)
	summary.BlockRead = calculateStats(blockReadRateValues)
	summary.BlockWrite = calculateStats(blockWriteRateValues)
	summary.PidsCount = calculateStats(pidsValues)

	// Get totals for monitoring period (delta between first and last sample)
	// Docker stats are cumulative since container start, so we need the difference
	firstSample := samples[0]
	lastSample := samples[len(samples)-1]

	// Handle counter resets (container restart) - if counters went backwards, use last sample value
	if lastSample.NetRxBytes >= firstSample.NetRxBytes {
		summary.NetRxTotal = lastSample.NetRxBytes - firstSample.NetRxBytes
	} else {
		summary.NetRxTotal = lastSample.NetRxBytes // Counter reset, use current value
	}

	if lastSample.NetTxBytes >= firstSample.NetTxBytes {
		summary.NetTxTotal = lastSample.NetTxBytes - firstSample.NetTxBytes
	} else {
		summary.NetTxTotal = lastSample.NetTxBytes
	}

	if lastSample.BlockRead >= firstSample.BlockRead {
		summary.BlockReadTotal = lastSample.BlockRead - firstSample.BlockRead
	} else {
		summary.BlockReadTotal = lastSample.BlockRead
	}

	if lastSample.BlockWrite >= firstSample.BlockWrite {
		summary.BlockWriteTotal = lastSample.BlockWrite - firstSample.BlockWrite
	} else {
		summary.BlockWriteTotal = lastSample.BlockWrite
	}

	// Calculate network breakdown percentages
	// Prefer byte-based data (from conntrack) when available, fall back to connection counts
	var totalBytesInterContainer, totalBytesInternal, totalBytesInternet uint64
	var totalConnInterContainer, totalConnInternal, totalConnInternet int
	var samplesWithBytes, samplesWithConnections int

	for _, s := range samples {
		// Check for byte-based data (conntrack)
		totalBytes := s.NetBytesInterContainer + s.NetBytesInternal + s.NetBytesInternet
		if totalBytes > 0 {
			totalBytesInterContainer += s.NetBytesInterContainer
			totalBytesInternal += s.NetBytesInternal
			totalBytesInternet += s.NetBytesInternet
			samplesWithBytes++
		}

		// Also collect connection counts as fallback
		totalConns := s.NetConnInterContainer + s.NetConnInternal + s.NetConnInternet
		if totalConns > 0 {
			totalConnInterContainer += s.NetConnInterContainer
			totalConnInternal += s.NetConnInternal
			totalConnInternet += s.NetConnInternet
			samplesWithConnections++
		}
	}

	// Use byte-based breakdown if available (more accurate)
	if samplesWithBytes > 0 && (totalBytesInterContainer+totalBytesInternal+totalBytesInternet) > 0 {
		totalBytes := float64(totalBytesInterContainer + totalBytesInternal + totalBytesInternet)
		summary.NetworkBreakdown = &NetworkBreakdown{
			InterContainerPct: float64(totalBytesInterContainer) / totalBytes * 100,
			InternalPct:       float64(totalBytesInternal) / totalBytes * 100,
			InternetPct:       float64(totalBytesInternet) / totalBytes * 100,
		}
	} else if samplesWithConnections > 0 && (totalConnInterContainer+totalConnInternal+totalConnInternet) > 0 {
		// Fall back to connection-based estimate
		totalConns := float64(totalConnInterContainer + totalConnInternal + totalConnInternet)
		summary.NetworkBreakdown = &NetworkBreakdown{
			InterContainerPct: float64(totalConnInterContainer) / totalConns * 100,
			InternalPct:       float64(totalConnInternal) / totalConns * 100,
			InternetPct:       float64(totalConnInternet) / totalConns * 100,
		}
	}

	return summary
}

// calculateStats calculates min, max, avg, p95, p99 for a slice of values
func calculateStats(values []float64) Summary {
	if len(values) == 0 {
		return Summary{}
	}

	// Sort for percentiles
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	// Calculate min, max, avg
	var sum float64
	min := sorted[0]
	max := sorted[len(sorted)-1]

	for _, v := range values {
		sum += v
	}
	avg := sum / float64(len(values))

	// Calculate percentiles
	p95 := percentile(sorted, 0.95)
	p99 := percentile(sorted, 0.99)

	return Summary{
		Min: min,
		Max: max,
		Avg: avg,
		P95: p95,
		P99: p99,
	}
}

// percentile calculates the p-th percentile of a sorted slice
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}

	// Calculate rank
	rank := p * float64(len(sorted)-1)
	lower := int(math.Floor(rank))
	upper := int(math.Ceil(rank))

	if lower == upper {
		return sorted[lower]
	}

	// Linear interpolation
	weight := rank - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

// AWS region pricing for data transfer (approximate, as of 2024)
var awsDataTransferPricing = map[string]float64{
	"us-east-1":      0.09, // per GB after first 10TB
	"us-west-2":      0.09,
	"eu-west-1":      0.09,
	"ap-southeast-1": 0.12,
	"default":        0.09,
}

// CalculateNetworkCost estimates AWS data transfer costs
func CalculateNetworkCost(egressBytes uint64) *NetworkCostEstimate {
	region := "us-east-1" // Default region
	pricePerGB := awsDataTransferPricing[region]

	egressGB := float64(egressBytes) / (1024 * 1024 * 1024)

	// AWS pricing is tiered, but we use simplified model
	// First 1GB/month is free, then tiered pricing
	// We'll use average rate for simplicity
	estimatedCost := egressGB * pricePerGB

	return &NetworkCostEstimate{
		Region:           region,
		EgressGB:         egressGB,
		IngressGB:        0, // Ingress is typically free
		EstimatedCostUSD: estimatedCost,
		PricePerGB:       pricePerGB,
		Notes:            "Estimate based on standard data transfer rates. Actual costs may vary.",
	}
}

// AWS instance types (simplified subset)
type InstanceType struct {
	Type     string
	VCPU     int
	MemoryGB float64
	Hourly   float64
	Arch     string // "x86" or "arm"
}

var awsInstanceTypes = []InstanceType{
	// x86 instances
	{"t3.micro", 2, 1, 0.0104, "x86"},
	{"t3.small", 2, 2, 0.0208, "x86"},
	{"t3.medium", 2, 4, 0.0416, "x86"},
	{"t3.large", 2, 8, 0.0832, "x86"},
	{"t3.xlarge", 4, 16, 0.1664, "x86"},
	{"m5.large", 2, 8, 0.096, "x86"},
	{"m5.xlarge", 4, 16, 0.192, "x86"},
	{"m5.2xlarge", 8, 32, 0.384, "x86"},
	{"c5.large", 2, 4, 0.085, "x86"},
	{"c5.xlarge", 4, 8, 0.17, "x86"},
	{"c5.2xlarge", 8, 16, 0.34, "x86"},
	{"r5.large", 2, 16, 0.126, "x86"},
	{"r5.xlarge", 4, 32, 0.252, "x86"},

	// ARM (Graviton) instances - typically 20% cheaper
	{"t4g.micro", 2, 1, 0.0084, "arm"},
	{"t4g.small", 2, 2, 0.0168, "arm"},
	{"t4g.medium", 2, 4, 0.0336, "arm"},
	{"t4g.large", 2, 8, 0.0672, "arm"},
	{"t4g.xlarge", 4, 16, 0.1344, "arm"},
	{"m7g.large", 2, 8, 0.0816, "arm"},
	{"m7g.xlarge", 4, 16, 0.1632, "arm"},
	{"m7g.2xlarge", 8, 32, 0.3264, "arm"},
	{"c7g.large", 2, 4, 0.0725, "arm"},
	{"c7g.xlarge", 4, 8, 0.145, "arm"},
	{"c7g.2xlarge", 8, 16, 0.29, "arm"},
	{"r7g.large", 2, 16, 0.1008, "arm"},
	{"r7g.xlarge", 4, 32, 0.2016, "arm"},
}

// RecommendInstance provides a basic instance type recommendation for a specific architecture
func RecommendInstance(summary *ContainerSummary, arch string) *InstanceRecommendation {
	if summary == nil {
		return nil
	}

	// Determine requirements based on P95 usage
	// Add 20% buffer for headroom
	requiredCPU := summary.CPUPercent.P95 / 100 * 1.2
	requiredMemGB := summary.MemoryUsage.P95 / (1024 * 1024 * 1024) * 1.2

	// Determine if workload is CPU or memory bound
	cpuBound := summary.CPUPercent.P95 > summary.MemoryPercent.P95

	// Find suitable instance for specified architecture
	var recommendation *InstanceRecommendation
	var lastOfArch *InstanceType

	for _, inst := range awsInstanceTypes {
		if inst.Arch != arch {
			continue
		}
		lastOfArch = &inst

		// Check if instance has enough resources
		if float64(inst.VCPU) >= requiredCPU && inst.MemoryGB >= requiredMemGB {
			reason := ""
			if cpuBound {
				reason = fmt.Sprintf("CPU-bound workload (P95: %.1f%%)", summary.CPUPercent.P95)
			} else {
				reason = fmt.Sprintf("Memory-bound workload (P95: %.1f%%, %.2f GB)",
					summary.MemoryPercent.P95, summary.MemoryUsage.P95/(1024*1024*1024))
			}

			recommendation = &InstanceRecommendation{
				InstanceType:  inst.Type,
				VCPU:          inst.VCPU,
				MemoryGB:      inst.MemoryGB,
				Reason:        reason,
				HourlyPrice:   inst.Hourly,
				Architecture:  arch,
			}
			break
		}
	}

	// If no suitable instance found, recommend largest of this architecture
	if recommendation == nil && lastOfArch != nil {
		recommendation = &InstanceRecommendation{
			InstanceType:  lastOfArch.Type,
			VCPU:          lastOfArch.VCPU,
			MemoryGB:      lastOfArch.MemoryGB,
			Reason:        "Resource requirements exceed common instance sizes",
			HourlyPrice:   lastOfArch.Hourly,
			Architecture:  arch,
		}
	}

	return recommendation
}

// RecommendBothArchitectures returns recommendations for both x86 and ARM
func RecommendBothArchitectures(summary *ContainerSummary) (x86, arm *InstanceRecommendation) {
	return RecommendInstance(summary, "x86"), RecommendInstance(summary, "arm")
}

// DetectWarnings identifies potential issues in the monitoring data
func DetectWarnings(data *ContainerData) []string {
	var warnings []string

	if data.Summary == nil {
		return warnings
	}

	// Memory limit warnings
	if data.Limits.MemLimit > 0 {
		memLimitBytes := float64(data.Limits.MemLimit)
		if data.Summary.MemoryUsage.Max >= memLimitBytes*0.95 {
			warnings = append(warnings, "Memory usage reached 95%+ of limit - OOM risk")
		} else if data.Summary.MemoryUsage.P95 >= memLimitBytes*0.80 {
			warnings = append(warnings, "Memory usage P95 above 80% of limit")
		}
	}

	// High memory usage without limit
	if data.Limits.MemLimit == 0 && data.Summary.MemoryPercent.P95 > 80 {
		warnings = append(warnings, "High memory usage with no memory limit set")
	}

	// CPU warnings
	if data.Summary.CPUPercent.P95 > 90 {
		warnings = append(warnings, "CPU usage P95 above 90%")
	}
	if data.Summary.CPUPercent.Max >= 100 {
		warnings = append(warnings, "CPU usage reached 100% - possible throttling")
	}

	// CPU quota/throttling
	if data.Limits.CPUQuota > 0 && data.Limits.CPUPeriod > 0 {
		cpuLimit := float64(data.Limits.CPUQuota) / float64(data.Limits.CPUPeriod) * 100
		if data.Summary.CPUPercent.P95 >= cpuLimit*0.90 {
			warnings = append(warnings, fmt.Sprintf("CPU usage near quota limit (%.1f%%)", cpuLimit))
		}
	}

	// High network traffic
	if data.Summary.NetTxTotal > 10*1024*1024*1024 { // 10GB
		warnings = append(warnings, fmt.Sprintf("High egress traffic: %.2f GB",
			float64(data.Summary.NetTxTotal)/(1024*1024*1024)))
	}

	// PIDs
	if data.Limits.PidsLimit > 0 {
		pidsLimit := float64(data.Limits.PidsLimit)
		if data.Summary.PidsCount.Max >= pidsLimit*0.90 {
			warnings = append(warnings, "PID count near limit")
		}
	}

	return warnings
}
