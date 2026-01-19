package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/guptarohit/asciigraph"
)

// HistoryTUIModel contains the state for the interactive history viewer
type HistoryTUIModel struct {
	configName     string
	sessionID      string             // Optional: filter to specific session
	containerFiles []string           // Paths to JSON files
	currentIndex   int                // Currently focused container
	containerData  []*ContainerData   // Loaded data
	fileModTimes   map[string]time.Time // For file watching

	// Scrolling
	viewport      viewport.Model // From bubbles/viewport
	viewportReady bool

	// State
	lastUpdate    time.Time
	isLive        bool               // Is daemon running?
	errorMsg      string
	width, height int

	// Render cache
	renderedContent string
	needsRender     bool
}

// Message types for the TUI
type historyTickMsg time.Time
type fileUpdateMsg struct {
	updates      []fileUpdate
	newModTimes  map[string]time.Time
}

type fileUpdate struct {
	index int
	data  *ContainerData
}
type daemonStatusMsg struct {
	running bool
}

// NewHistoryTUIModel creates a new history TUI model
func NewHistoryTUIModel(configName string, sessionID string) HistoryTUIModel {
	return HistoryTUIModel{
		configName:   configName,
		sessionID:    sessionID,
		fileModTimes: make(map[string]time.Time),
		needsRender:  true,
	}
}

// Init initializes the model
func (m HistoryTUIModel) Init() tea.Cmd {
	return m.loadInitialData()
}

// loadInitialData loads the initial container data
func (m HistoryTUIModel) loadInitialData() tea.Cmd {
	return func() tea.Msg {
		// Load all container data files
		dataDir := GetDataDir(m.configName)
		files, err := filepath.Glob(filepath.Join(dataDir, "*.json"))
		if err != nil || len(files) == 0 {
			return errMsg{error: fmt.Errorf("no monitoring data found")}
		}

		containerData := make([]*ContainerData, len(files))
		fileModTimes := make(map[string]time.Time)

		// Load data and track modification times
		for i, file := range files {
			data, err := LoadContainerData(file)
			if err != nil {
				continue
			}

			// Filter based on sessionID or show most recent session
			if m.sessionID != "" {
				data = filterToSession(data, m.sessionID)
			} else {
				data = filterToCurrentSession(data)
			}
			containerData[i] = data

			// Track file modification time
			if stat, err := os.Stat(file); err == nil {
				fileModTimes[file] = stat.ModTime()
			}
		}

		return initialDataMsg{
			files:        files,
			data:         containerData,
			fileModTimes: fileModTimes,
		}
	}
}

type errMsg struct {
	error error
}

type initialDataMsg struct {
	files        []string
	data         []*ContainerData
	fileModTimes map[string]time.Time
}

// Update handles messages
func (m HistoryTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case errMsg:
		m.errorMsg = msg.error.Error()
		return m, tea.Quit

	case initialDataMsg:
		m.containerFiles = msg.files
		m.containerData = msg.data
		m.fileModTimes = msg.fileModTimes
		m.needsRender = true
		return m, tea.Batch(m.tick(), m.checkDaemonStatus())

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "left", "h":
			if len(m.containerData) > 0 {
				m.currentIndex--
				if m.currentIndex < 0 {
					m.currentIndex = len(m.containerData) - 1
				}
				m.needsRender = true
			}

		case "right", "l":
			if len(m.containerData) > 0 {
				m.currentIndex++
				if m.currentIndex >= len(m.containerData) {
					m.currentIndex = 0
				}
				m.needsRender = true
			}

		case "r":
			// Force refresh
			cmds = append(cmds, m.checkFileChanges())
			m.needsRender = true

		case "up", "k", "down", "j", "pgup", "pgdown":
			// Delegate to viewport
			if m.viewportReady {
				m.viewport, cmd = m.viewport.Update(msg)
				cmds = append(cmds, cmd)
			}
		}

	case tea.MouseMsg:
		// Forward mouse events (including scroll wheel) to viewport
		if m.viewportReady {
			m.viewport, cmd = m.viewport.Update(msg)
			cmds = append(cmds, cmd)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Reserve space for header (3 lines) and footer (1 line)
		headerHeight := 3
		footerHeight := 1
		verticalMargins := headerHeight + footerHeight

		if !m.viewportReady {
			m.viewport = viewport.New(msg.Width, msg.Height-verticalMargins)
			m.viewport.YPosition = headerHeight
			m.viewportReady = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - verticalMargins
		}

		m.needsRender = true

	case historyTickMsg:
		m.lastUpdate = time.Time(msg)
		cmds = append(cmds, m.tick(), m.checkFileChanges(), m.checkDaemonStatus())

	case fileUpdateMsg:
		// Process all file updates
		for _, update := range msg.updates {
			if update.index >= 0 && update.index < len(m.containerData) {
				m.containerData[update.index] = update.data
			}
		}
		// Update modification times to prevent re-detecting same changes
		for filepath, modTime := range msg.newModTimes {
			m.fileModTimes[filepath] = modTime
		}
		if len(msg.updates) > 0 {
			m.needsRender = true
		}

	case daemonStatusMsg:
		m.isLive = msg.running
	}

	// Re-render content if needed
	if m.needsRender && m.viewportReady && len(m.containerData) > 0 {
		m.renderedContent = m.renderContainerContent()
		m.viewport.SetContent(m.renderedContent)
		m.needsRender = false
	}

	return m, tea.Batch(cmds...)
}

// View renders the TUI
func (m HistoryTUIModel) View() string {
	if m.errorMsg != "" {
		return errorStyle.Render(fmt.Sprintf("Error: %s\n", m.errorMsg))
	}

	if len(m.containerData) == 0 {
		return "Loading data...\n"
	}

	var s strings.Builder

	// Header
	currentData := m.containerData[m.currentIndex]
	liveIndicator := ""
	if m.isLive {
		liveIndicator = successStyle.Render(" [LIVE]")
	}

	timeSinceUpdate := ""
	if !m.lastUpdate.IsZero() {
		elapsed := time.Since(m.lastUpdate)
		timeSinceUpdate = fmt.Sprintf(" | Last update: %s ago", formatDuration(elapsed))
	}

	header := fmt.Sprintf("[%d/%d] Container: %s%s%s\n",
		m.currentIndex+1,
		len(m.containerData),
		selectedStyle.Render(currentData.ContainerName),
		liveIndicator,
		dimStyle.Render(timeSinceUpdate))

	s.WriteString(header)
	s.WriteString(strings.Repeat("─", m.width) + "\n")

	// Content (viewport)
	if m.viewportReady {
		s.WriteString(m.viewport.View())
		s.WriteString("\n")
	}

	// Footer
	footer := helpStyle.Render("← → Switch | ↑↓ Scroll | R Refresh | Q Quit")
	s.WriteString(footer)

	return s.String()
}

// tick returns a command that sends a tick message every 2 seconds
func (m HistoryTUIModel) tick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return historyTickMsg(t)
	})
}

// checkFileChanges checks if any data files have been modified
func (m HistoryTUIModel) checkFileChanges() tea.Cmd {
	// Capture current mod times to compare against
	currentModTimes := make(map[string]time.Time)
	for k, v := range m.fileModTimes {
		currentModTimes[k] = v
	}
	files := m.containerFiles
	sessionID := m.sessionID

	return func() tea.Msg {
		var updates []fileUpdate
		newModTimes := make(map[string]time.Time)

		for i, filepath := range files {
			stat, err := os.Stat(filepath)
			if err != nil {
				continue
			}

			modTime := stat.ModTime()
			if modTime.After(currentModTimes[filepath]) {
				// File changed, reload
				data, err := LoadContainerData(filepath)
				if err != nil {
					continue
				}

				// Filter to session
				if sessionID != "" {
					data = filterToSession(data, sessionID)
				} else {
					data = filterToCurrentSession(data)
				}

				updates = append(updates, fileUpdate{index: i, data: data})
				newModTimes[filepath] = modTime
			}
		}

		if len(updates) == 0 {
			return nil
		}
		return fileUpdateMsg{updates: updates, newModTimes: newModTimes}
	}
}

// checkDaemonStatus checks if the daemon is running
func (m HistoryTUIModel) checkDaemonStatus() tea.Cmd {
	return func() tea.Msg {
		return daemonStatusMsg{running: IsRunning(m.configName)}
	}
}

// renderContainerContent renders the content for the current container
func (m HistoryTUIModel) renderContainerContent() string {
	data := m.containerData[m.currentIndex]

	// Calculate summary if not present
	if data.Summary == nil && len(data.Samples) > 0 {
		data.Summary = CalculateSummary(data.Samples)
		data.Summary.Warnings = DetectWarnings(data)
		if data.EndTime.IsZero() {
			data.EndTime = time.Now()
		}
		if !data.StartTime.IsZero() && !data.EndTime.IsZero() {
			data.Summary.Duration = data.EndTime.Sub(data.StartTime).Round(time.Second).String()
		}
	}

	// Calculate network cost if not present
	if data.NetworkCost == nil && data.Summary != nil {
		data.NetworkCost = CalculateNetworkCost(data.Summary.NetTxTotal)
	}

	var s strings.Builder

	// Container info
	s.WriteString("┌─────────────────────────────────────────────────────────────────────────┐\n")
	s.WriteString(fmt.Sprintf("│ Container: %-63s │\n", data.ContainerName))
	s.WriteString(fmt.Sprintf("│ Image: %-67s │\n", data.ImageName))
	if len(data.ContainerID) >= 12 {
		s.WriteString(fmt.Sprintf("│ ID: %-70s │\n", data.ContainerID[:12]))
	}
	s.WriteString("└─────────────────────────────────────────────────────────────────────────┘\n\n")

	// Host Information
	s.WriteString("Host Information:\n")
	s.WriteString(fmt.Sprintf("  Hostname: %s\n", data.Host.Hostname))
	s.WriteString(fmt.Sprintf("  CPU: %s (%d cores, %s)\n", data.Host.CPUModel, data.Host.CPUCores, data.Host.Architecture))
	s.WriteString(fmt.Sprintf("  Memory: %s\n", formatBytes(data.Host.MemoryTotal)))
	s.WriteString(fmt.Sprintf("  OS: %s (kernel %s)\n", data.Host.OS, data.Host.KernelVer))
	s.WriteString(fmt.Sprintf("  Docker: %s\n", data.Host.DockerVer))

	// Architecture warning
	if strings.Contains(strings.ToLower(data.Host.Architecture), "arm") ||
		strings.Contains(strings.ToLower(data.Host.Architecture), "aarch") {
		s.WriteString("  ⚠️  ARM architecture - EC2 x86 vCPU performance will differ\n")
	}
	s.WriteString("\n")

	// Container Limits
	s.WriteString("Container Resource Limits:\n")
	if data.Limits.CPUQuota > 0 && data.Limits.CPUPeriod > 0 {
		cpuLimit := float64(data.Limits.CPUQuota) / float64(data.Limits.CPUPeriod)
		s.WriteString(fmt.Sprintf("  CPU Limit: %.2f cores (quota: %d, period: %d)\n",
			cpuLimit, data.Limits.CPUQuota, data.Limits.CPUPeriod))
	} else {
		s.WriteString("  CPU Limit: unlimited\n")
	}

	if data.Limits.MemLimit > 0 {
		s.WriteString(fmt.Sprintf("  Memory Limit: %s\n", formatBytes(data.Limits.MemLimit)))
	} else {
		s.WriteString("  Memory Limit: unlimited\n")
	}

	if data.Limits.PidsLimit > 0 {
		s.WriteString(fmt.Sprintf("  PIDs Limit: %d\n", data.Limits.PidsLimit))
	}
	s.WriteString("\n")

	// Monitoring Duration
	duration := data.EndTime.Sub(data.StartTime)
	s.WriteString("Monitoring Period:\n")
	s.WriteString(fmt.Sprintf("  Started: %s\n", data.StartTime.Format("2006-01-02 15:04:05")))
	s.WriteString(fmt.Sprintf("  Ended: %s\n", data.EndTime.Format("2006-01-02 15:04:05")))
	s.WriteString(fmt.Sprintf("  Duration: %s\n", duration.Round(time.Second)))
	s.WriteString(fmt.Sprintf("  Samples: %d (interval: %ds)\n\n", len(data.Samples), data.Interval))

	// Graphs
	if len(data.Samples) > 0 && data.Summary != nil {
		s.WriteString("Resource Usage Over Time:\n\n")

		// Downsample if too many data points (show max 100 points)
		step := 1
		if len(data.Samples) > 100 {
			step = len(data.Samples) / 100
		}

		// CPU Graph
		cpuData := make([]float64, 0, len(data.Samples)/step)
		memData := make([]float64, 0, len(data.Samples)/step)
		netTxData := make([]float64, 0, len(data.Samples)/step)
		netRxData := make([]float64, 0, len(data.Samples)/step)

		for i := 0; i < len(data.Samples); i += step {
			cpuData = append(cpuData, data.Samples[i].CPUPercent)
			memData = append(memData, float64(data.Samples[i].MemoryUsage)/(1024*1024)) // MB
			netTxData = append(netTxData, data.Samples[i].NetTxRate/(1024*1024))         // MB/s
			netRxData = append(netRxData, data.Samples[i].NetRxRate/(1024*1024))         // MB/s
		}

		s.WriteString("CPU Usage (%)\n")
		cpuGraph := asciigraph.Plot(cpuData, asciigraph.Height(10), asciigraph.Width(70),
			asciigraph.Caption(fmt.Sprintf("Min: %.1f%% | Avg: %.1f%% | Max: %.1f%% | P95: %.1f%%",
				data.Summary.CPUPercent.Min, data.Summary.CPUPercent.Avg,
				data.Summary.CPUPercent.Max, data.Summary.CPUPercent.P95)))
		s.WriteString(cpuGraph)
		s.WriteString("\n\n")

		s.WriteString("Memory Usage (MB)\n")
		memGraph := asciigraph.Plot(memData, asciigraph.Height(10), asciigraph.Width(70),
			asciigraph.Caption(fmt.Sprintf("Min: %s | Avg: %s | Max: %s | P95: %s",
				formatBytes(uint64(data.Summary.MemoryUsage.Min)),
				formatBytes(uint64(data.Summary.MemoryUsage.Avg)),
				formatBytes(uint64(data.Summary.MemoryUsage.Max)),
				formatBytes(uint64(data.Summary.MemoryUsage.P95)))))
		s.WriteString(memGraph)
		s.WriteString("\n\n")

		s.WriteString("Network TX (MB/s)\n")
		netTxGraph := asciigraph.Plot(netTxData, asciigraph.Height(8), asciigraph.Width(70),
			asciigraph.Caption(fmt.Sprintf("Total Egress: %s", formatBytes(data.Summary.NetTxTotal))))
		s.WriteString(netTxGraph)
		s.WriteString("\n\n")

		s.WriteString("Network RX (MB/s)\n")
		netRxGraph := asciigraph.Plot(netRxData, asciigraph.Height(8), asciigraph.Width(70),
			asciigraph.Caption(fmt.Sprintf("Total Ingress: %s", formatBytes(data.Summary.NetRxTotal))))
		s.WriteString(netRxGraph)
		s.WriteString("\n\n")
	}

	// Summary Statistics
	if data.Summary != nil {
		sum := data.Summary
		s.WriteString("Summary Statistics:\n")
		s.WriteString(fmt.Sprintf("  CPU:      min=%.1f%% avg=%.1f%% max=%.1f%% p95=%.1f%% p99=%.1f%%\n",
			sum.CPUPercent.Min, sum.CPUPercent.Avg, sum.CPUPercent.Max, sum.CPUPercent.P95, sum.CPUPercent.P99))
		s.WriteString(fmt.Sprintf("  Memory:   min=%s avg=%s max=%s p95=%s\n",
			formatBytes(uint64(sum.MemoryUsage.Min)),
			formatBytes(uint64(sum.MemoryUsage.Avg)),
			formatBytes(uint64(sum.MemoryUsage.Max)),
			formatBytes(uint64(sum.MemoryUsage.P95))))
		s.WriteString(fmt.Sprintf("  Net I/O:  rx=%s tx=%s\n",
			formatBytes(sum.NetRxTotal),
			formatBytes(sum.NetTxTotal)))

		// Network breakdown (if available)
		if sum.NetworkBreakdown != nil {
			s.WriteString("  Traffic:  ")
			if sum.NetworkBreakdown.InterContainerPct > 0 {
				s.WriteString(fmt.Sprintf("inter-container=%.1f%% ", sum.NetworkBreakdown.InterContainerPct))
			}
			if sum.NetworkBreakdown.InternalPct > 0 {
				s.WriteString(fmt.Sprintf("internal=%.1f%% ", sum.NetworkBreakdown.InternalPct))
			}
			if sum.NetworkBreakdown.InternetPct > 0 {
				s.WriteString(fmt.Sprintf("internet=%.1f%%", sum.NetworkBreakdown.InternetPct))
			}
			s.WriteString("\n")
		}

		s.WriteString(fmt.Sprintf("  Block I/O: read=%s write=%s\n",
			formatBytes(sum.BlockReadTotal),
			formatBytes(sum.BlockWriteTotal)))
		s.WriteString(fmt.Sprintf("  PIDs:     min=%.0f avg=%.0f max=%.0f\n\n",
			sum.PidsCount.Min, sum.PidsCount.Avg, sum.PidsCount.Max))

		// Warnings
		if len(sum.Warnings) > 0 {
			s.WriteString("⚠️  Warnings:\n")
			for _, w := range sum.Warnings {
				s.WriteString(fmt.Sprintf("  • %s\n", w))
			}
			s.WriteString("\n")
		}
	}

	// Network Cost with Monthly Projection
	if data.NetworkCost != nil {
		s.WriteString(fmt.Sprintf("AWS Network Cost Estimate (%s):\n", data.NetworkCost.Region))
		s.WriteString(fmt.Sprintf("  Egress (this session): %.2f GB @ $%.3f/GB = $%.2f\n",
			data.NetworkCost.EgressGB,
			data.NetworkCost.PricePerGB,
			data.NetworkCost.EstimatedCostUSD))

		// Calculate monthly projection
		duration := data.EndTime.Sub(data.StartTime)
		if duration.Hours() > 0 {
			hoursInMonth := 720.0 // 30 days
			projectedGB := data.NetworkCost.EgressGB * (hoursInMonth / duration.Hours())
			projectedCost := projectedGB * data.NetworkCost.PricePerGB
			s.WriteString(fmt.Sprintf("  Monthly projection:    %.2f GB/month = $%.2f/month (at current rate)\n",
				projectedGB, projectedCost))

			if projectedCost > 50 {
				s.WriteString("  ⚠️  High egress costs - consider same-AZ placement or caching\n")
			}
		}
		s.WriteString("\n")
	}

	// AWS Instance Recommendations (both x86 and ARM)
	if data.Summary != nil {
		x86Rec, armRec := RecommendBothArchitectures(data.Summary)

		s.WriteString("AWS Instance Recommendations:\n\n")

		if x86Rec != nil {
			monthlyPrice := x86Rec.HourlyPrice * 730 // hours in month
			s.WriteString("  x86_64 (Intel/AMD):\n")
			s.WriteString(fmt.Sprintf("    Instance: %s (%d vCPU, %.0f GB RAM)\n",
				x86Rec.InstanceType, x86Rec.VCPU, x86Rec.MemoryGB))
			s.WriteString(fmt.Sprintf("    Cost: $%.4f/hour (~$%.2f/month)\n", x86Rec.HourlyPrice, monthlyPrice))
			s.WriteString(fmt.Sprintf("    Reason: %s\n\n", x86Rec.Reason))
		}

		if armRec != nil {
			monthlyPrice := armRec.HourlyPrice * 730
			savings := 0.0
			if x86Rec != nil {
				savings = ((x86Rec.HourlyPrice - armRec.HourlyPrice) / x86Rec.HourlyPrice) * 100
			}
			s.WriteString("  ARM64 (Graviton):\n")
			s.WriteString(fmt.Sprintf("    Instance: %s (%d vCPU, %.0f GB RAM)\n",
				armRec.InstanceType, armRec.VCPU, armRec.MemoryGB))
			s.WriteString(fmt.Sprintf("    Cost: $%.4f/hour (~$%.2f/month)", armRec.HourlyPrice, monthlyPrice))
			if savings > 0 {
				s.WriteString(fmt.Sprintf(" [%.0f%% cheaper than x86]", savings))
			}
			s.WriteString(fmt.Sprintf("\n    Reason: %s\n\n", armRec.Reason))
		}

		// Architecture note
		hostArch := strings.ToLower(data.Host.Architecture)
		if strings.Contains(hostArch, "arm") || strings.Contains(hostArch, "aarch") {
			s.WriteString("  ℹ️  Measured on ARM hardware. Performance may differ on x86 instances.\n")
		} else if strings.Contains(hostArch, "x86") || strings.Contains(hostArch, "amd64") {
			s.WriteString("  ℹ️  Measured on x86 hardware. ARM instances may perform differently.\n")
		}
		s.WriteString("  ℹ️  Recommendations are estimates. Test on target instance type before committing.\n")
	}

	s.WriteString("\n")

	return s.String()
}
