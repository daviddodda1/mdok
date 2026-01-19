package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205"))

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("212"))

	cursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("82"))

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))

	barStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205"))

	barBgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))
)

// SelectionModel is the TUI model for container selection
type SelectionModel struct {
	containers         []ContainerInfo
	filteredContainers []ContainerInfo   // Containers after search filter
	cursor             int
	selected           map[int]bool
	selectedContainers []string
	configName         string
	interval           int
	phase              int // 0=selection, 1=interval, 2=name
	intervalInput      textinput.Model
	nameInput          textinput.Model
	searchInput        textinput.Model
	searchActive       bool
	cancelled          bool
	err                error
	windowSize         int // Number of containers visible at once
	windowOffset       int // First visible container index
}

// NewSelectionModel creates a new selection model
func NewSelectionModel(containers []ContainerInfo) SelectionModel {
	intervalInput := textinput.New()
	intervalInput.Placeholder = "5"
	intervalInput.CharLimit = 4
	intervalInput.Width = 10

	nameInput := textinput.New()
	nameInput.Placeholder = "my-monitor"
	nameInput.CharLimit = 50
	nameInput.Width = 30

	searchInput := textinput.New()
	searchInput.Placeholder = "Search containers..."
	searchInput.CharLimit = 100
	searchInput.Width = 50

	return SelectionModel{
		containers:         containers,
		filteredContainers: containers, // Initially show all
		selected:           make(map[int]bool),
		intervalInput:      intervalInput,
		nameInput:          nameInput,
		searchInput:        searchInput,
		searchActive:       false,
		interval:           5,
		windowSize:         10, // Show 10 containers at a time (each takes 2 lines)
		windowOffset:       0,
	}
}

// NewEditModel creates a selection model pre-populated with existing config
func NewEditModel(containers []ContainerInfo, config Config) SelectionModel {
	m := NewSelectionModel(containers)
	m.configName = config.Name
	m.interval = config.Interval
	m.intervalInput.SetValue(strconv.Itoa(config.Interval))

	// Pre-select containers that are in the config
	for i, c := range containers {
		for _, selected := range config.Containers {
			if c.Name == selected || c.ID == selected {
				m.selected[i] = true
				break
			}
		}
	}

	return m
}

func (m SelectionModel) Init() tea.Cmd {
	return nil
}

// filterContainers filters containers based on search query
func (m *SelectionModel) filterContainers() {
	query := strings.ToLower(m.searchInput.Value())
	if query == "" {
		m.filteredContainers = m.containers
		return
	}

	filtered := make([]ContainerInfo, 0)
	for _, c := range m.containers {
		name := strings.ToLower(c.Name)
		id := strings.ToLower(c.ID)
		image := strings.ToLower(c.Image)

		if strings.Contains(name, query) || strings.Contains(id, query) || strings.Contains(image, query) {
			filtered = append(filtered, c)
		}
	}
	m.filteredContainers = filtered

	// Reset cursor and window if needed
	if m.cursor >= len(m.filteredContainers) {
		m.cursor = 0
		m.windowOffset = 0
	}
}

// getActualContainerIndex maps filtered index to original container index
func (m *SelectionModel) getActualContainerIndex(filteredIndex int) int {
	if filteredIndex < 0 || filteredIndex >= len(m.filteredContainers) {
		return -1
	}

	filteredContainer := m.filteredContainers[filteredIndex]
	for i, c := range m.containers {
		if c.ID == filteredContainer.ID {
			return i
		}
	}
	return -1
}

func (m SelectionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle search mode input first
		if m.searchActive && m.phase == 0 {
			switch msg.String() {
			case "esc":
				// Exit search mode
				m.searchActive = false
				m.searchInput.Blur()
				m.searchInput.SetValue("")
				m.filterContainers()
				return m, nil
			case "enter":
				// Exit search mode and keep filter
				m.searchActive = false
				return m, nil
			default:
				// Update search input
				m.searchInput, cmd = m.searchInput.Update(msg)
				m.filterContainers()
				return m, cmd
			}
		}

		switch msg.String() {
		case "ctrl+c", "q":
			if m.phase == 0 && !m.searchActive {
				m.cancelled = true
				return m, tea.Quit
			}
		case "/":
			if m.phase == 0 && !m.searchActive {
				// Activate search mode
				m.searchActive = true
				m.searchInput.Focus()
				return m, textinput.Blink
			}
		case "esc":
			if m.phase > 0 {
				m.phase--
				return m, nil
			}
			if m.searchActive {
				// Exit search mode
				m.searchActive = false
				m.searchInput.SetValue("")
				m.filterContainers()
				return m, nil
			}
			m.cancelled = true
			return m, tea.Quit
		case "up", "k":
			if m.phase == 0 && !m.searchActive && m.cursor > 0 {
				m.cursor--
				// Adjust window if cursor moved above visible area
				if m.cursor < m.windowOffset {
					m.windowOffset = m.cursor
				}
			}
		case "down", "j":
			if m.phase == 0 && !m.searchActive && m.cursor < len(m.filteredContainers)-1 {
				m.cursor++
				// Adjust window if cursor moved below visible area
				if m.cursor >= m.windowOffset+m.windowSize {
					m.windowOffset = m.cursor - m.windowSize + 1
				}
			}
		case "pgup":
			if m.phase == 0 && !m.searchActive && m.cursor > 0 {
				// Jump up by window size
				m.cursor -= m.windowSize
				if m.cursor < 0 {
					m.cursor = 0
				}
				m.windowOffset = m.cursor
			}
		case "pgdown":
			if m.phase == 0 && !m.searchActive && m.cursor < len(m.filteredContainers)-1 {
				// Jump down by window size
				m.cursor += m.windowSize
				if m.cursor >= len(m.filteredContainers) {
					m.cursor = len(m.filteredContainers) - 1
				}
				// Adjust window
				if m.cursor >= m.windowOffset+m.windowSize {
					m.windowOffset = m.cursor - m.windowSize + 1
				}
			}
		case "home":
			if m.phase == 0 && !m.searchActive {
				m.cursor = 0
				m.windowOffset = 0
			}
		case "end":
			if m.phase == 0 && !m.searchActive {
				m.cursor = len(m.filteredContainers) - 1
				m.windowOffset = m.cursor - m.windowSize + 1
				if m.windowOffset < 0 {
					m.windowOffset = 0
				}
			}
		case " ":
			if m.phase == 0 && !m.searchActive && m.cursor < len(m.filteredContainers) {
				// Find the actual container index in the original list
				actualIndex := m.getActualContainerIndex(m.cursor)
				if actualIndex >= 0 {
					m.selected[actualIndex] = !m.selected[actualIndex]
				}
			}
		case "a":
			if m.phase == 0 && !m.searchActive {
				// Select all (in current filter)
				allFilteredSelected := true
				for i := range m.filteredContainers {
					actualIndex := m.getActualContainerIndex(i)
					if actualIndex >= 0 && !m.selected[actualIndex] {
						allFilteredSelected = false
						break
					}
				}

				// Toggle selection for all filtered containers
				for i := range m.filteredContainers {
					actualIndex := m.getActualContainerIndex(i)
					if actualIndex >= 0 {
						m.selected[actualIndex] = !allFilteredSelected
					}
				}
			}
		case "enter":
			switch m.phase {
			case 0:
				if len(m.selected) == 0 {
					m.err = fmt.Errorf("select at least one container")
					return m, nil
				}
				m.err = nil
				m.phase = 1
				m.intervalInput.Focus()
				return m, textinput.Blink
			case 1:
				val := m.intervalInput.Value()
				if val == "" {
					val = "5"
				}
				interval, err := strconv.Atoi(val)
				if err != nil || interval < 1 {
					m.err = fmt.Errorf("invalid interval: must be a positive number")
					return m, nil
				}
				m.interval = interval
				m.err = nil

				// Skip name input if editing
				if m.configName != "" {
					m.finalizeSelection()
					return m, tea.Quit
				}

				m.phase = 2
				m.intervalInput.Blur()
				m.nameInput.Focus()
				return m, textinput.Blink
			case 2:
				name := m.nameInput.Value()
				if name == "" {
					m.err = fmt.Errorf("configuration name is required")
					return m, nil
				}
				if strings.ContainsAny(name, "/\\:*?\"<>|") {
					m.err = fmt.Errorf("invalid characters in name")
					return m, nil
				}
				if ConfigExists(name) {
					m.err = fmt.Errorf("configuration '%s' already exists", name)
					return m, nil
				}
				m.configName = name
				m.err = nil
				m.finalizeSelection()
				return m, tea.Quit
			}
		}
	}

	// Update text inputs
	switch m.phase {
	case 1:
		m.intervalInput, cmd = m.intervalInput.Update(msg)
	case 2:
		m.nameInput, cmd = m.nameInput.Update(msg)
	}

	return m, cmd
}

func (m *SelectionModel) finalizeSelection() {
	m.selectedContainers = make([]string, 0)
	for i := range m.containers {
		if m.selected[i] {
			m.selectedContainers = append(m.selectedContainers, m.containers[i].Name)
		}
	}
}

func (m SelectionModel) View() string {
	var s strings.Builder

	switch m.phase {
	case 0:
		s.WriteString(titleStyle.Render("Select containers to monitor"))
		s.WriteString("\n")

		// Show search input
		if m.searchActive {
			s.WriteString("\n")
			s.WriteString("Search: ")
			s.WriteString(m.searchInput.View())
			s.WriteString(" ")
			s.WriteString(dimStyle.Render("(ESC to clear)"))
		} else if m.searchInput.Value() != "" {
			s.WriteString("\n")
			s.WriteString(dimStyle.Render(fmt.Sprintf("Filter: %s (/ to search, ESC to clear)", m.searchInput.Value())))
		}

		// Show scroll indicator if there are more items above
		if m.windowOffset > 0 {
			s.WriteString("\n")
			s.WriteString(dimStyle.Render(fmt.Sprintf("    ▲ %d more above...", m.windowOffset)))
		}
		s.WriteString("\n\n")

		// Calculate visible range
		start := m.windowOffset
		end := m.windowOffset + m.windowSize
		if end > len(m.filteredContainers) {
			end = len(m.filteredContainers)
		}

		// Render only visible containers
		for i := start; i < end; i++ {
			c := m.filteredContainers[i]
			actualIndex := m.getActualContainerIndex(i)

			cursor := "  "
			if m.cursor == i {
				cursor = cursorStyle.Render("> ")
			}

			checked := "[ ]"
			if actualIndex >= 0 && m.selected[actualIndex] {
				checked = selectedStyle.Render("[x]")
			}

			name := c.Name
			if c.Name == "" {
				name = c.ID
			}

			// Format status and uptime
			statusInfo := formatContainerStatus(c)

			line := fmt.Sprintf("%s%s %-30s %s", cursor, checked, name, statusInfo)
			if m.cursor == i {
				line = cursorStyle.Render(line)
			}
			s.WriteString(line)
			s.WriteString("\n")

			// Show image on second line with indent
			s.WriteString("     ")
			s.WriteString(dimStyle.Render(fmt.Sprintf("   %s", c.Image)))
			s.WriteString("\n")
		}

		// Show scroll indicator if there are more items below
		if end < len(m.filteredContainers) {
			s.WriteString(dimStyle.Render(fmt.Sprintf("    ▼ %d more below...", len(m.filteredContainers)-end)))
		}

		// Show no results message if filter is active but no matches
		if len(m.filteredContainers) == 0 && m.searchInput.Value() != "" {
			s.WriteString("\n")
			s.WriteString(warningStyle.Render("No containers match your search."))
		}

		s.WriteString("\n")
		selectedCount := len(m.selected)
		totalCount := len(m.containers)
		filteredCount := len(m.filteredContainers)

		if filteredCount != totalCount {
			s.WriteString(helpStyle.Render(fmt.Sprintf("Selected: %d | Showing: %d/%d | /: search | space: toggle | a: all | enter: continue | q: quit", selectedCount, filteredCount, totalCount)))
		} else {
			s.WriteString(helpStyle.Render(fmt.Sprintf("Selected: %d/%d | /: search | ↑↓: navigate | space: toggle | a: all | enter: continue | q: quit", selectedCount, totalCount)))
		}

	case 1:
		s.WriteString(titleStyle.Render("Set monitoring interval"))
		s.WriteString("\n\n")

		s.WriteString("Interval (seconds): ")
		s.WriteString(m.intervalInput.View())
		s.WriteString("\n\n")

		s.WriteString(helpStyle.Render("enter: continue | esc: back"))

	case 2:
		s.WriteString(titleStyle.Render("Name your configuration"))
		s.WriteString("\n\n")

		s.WriteString("Configuration name: ")
		s.WriteString(m.nameInput.View())
		s.WriteString("\n\n")

		s.WriteString(helpStyle.Render("enter: save | esc: back"))
	}

	if m.err != nil {
		s.WriteString("\n\n")
		s.WriteString(errorStyle.Render("Error: " + m.err.Error()))
	}

	return s.String()
}

// DashboardModel is the TUI model for live monitoring dashboard
type DashboardModel struct {
	config        Config
	docker        *DockerClient
	containerData map[string]*ContainerData
	prevStats     map[string]*StatsResult
	err           error
	paused        bool
	quitting      bool
	width         int
	height        int
	lastUpdate    time.Time
}

// NewDashboardModel creates a new dashboard model
func NewDashboardModel(config Config) DashboardModel {
	docker, _ := NewDockerClient()
	return DashboardModel{
		config:        config,
		docker:        docker,
		containerData: make(map[string]*ContainerData),
		prevStats:     make(map[string]*StatsResult),
	}
}

type tickMsg time.Time
type statsMsg struct {
	container string
	stats     *StatsResult
	err       error
}

func (m DashboardModel) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
		m.tick(),
	)
}

func (m DashboardModel) tick() tea.Cmd {
	return tea.Tick(time.Duration(m.config.Interval)*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m DashboardModel) collectStats(container string) tea.Cmd {
	return func() tea.Msg {
		if m.docker == nil {
			return statsMsg{container: container, err: fmt.Errorf("docker client not initialized")}
		}

		ctx := context.Background()
		stats, err := m.docker.CollectStats(ctx, container, m.prevStats[container])
		return statsMsg{
			container: container,
			stats:     stats,
			err:       err,
		}
	}
}

func (m DashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			if m.docker != nil {
				m.docker.Close()
			}
			return m, tea.Quit
		case "p", " ":
			m.paused = !m.paused
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tickMsg:
		if m.paused {
			return m, m.tick()
		}

		m.lastUpdate = time.Time(msg)

		// Collect stats for all containers
		var cmds []tea.Cmd
		for _, container := range m.config.Containers {
			cmds = append(cmds, m.collectStats(container))
		}
		cmds = append(cmds, m.tick())
		return m, tea.Batch(cmds...)

	case statsMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.prevStats[msg.container] = msg.stats

			// Update or create container data
			if m.containerData[msg.container] == nil {
				m.containerData[msg.container] = &ContainerData{
					ContainerName: msg.container,
					StartTime:     time.Now(),
					Samples:       make([]Sample, 0),
				}
			}
			m.containerData[msg.container].Samples = append(
				m.containerData[msg.container].Samples,
				msg.stats.Sample,
			)

			// Keep only last 100 samples in memory for dashboard
			if len(m.containerData[msg.container].Samples) > 100 {
				m.containerData[msg.container].Samples = m.containerData[msg.container].Samples[1:]
			}
		}
	}

	return m, nil
}

func (m DashboardModel) View() string {
	if m.quitting {
		return ""
	}

	var s strings.Builder

	// Header
	header := titleStyle.Render(fmt.Sprintf("mdok - %s", m.config.Name))
	if m.paused {
		header += warningStyle.Render(" [PAUSED]")
	}
	s.WriteString(header)
	s.WriteString("\n")
	s.WriteString(dimStyle.Render(fmt.Sprintf("Last update: %s | Interval: %ds",
		m.lastUpdate.Format("15:04:05"),
		m.config.Interval)))
	s.WriteString("\n\n")

	// Container stats
	for _, container := range m.config.Containers {
		data := m.containerData[container]
		if data == nil || len(data.Samples) == 0 {
			s.WriteString(fmt.Sprintf("%s: %s\n\n",
				selectedStyle.Render(container),
				dimStyle.Render("waiting for data...")))
			continue
		}

		latest := data.Samples[len(data.Samples)-1]

		s.WriteString(selectedStyle.Render(container))
		s.WriteString("\n")

		// CPU
		cpuBar := renderBar(latest.CPUPercent, 100, 30)
		s.WriteString(fmt.Sprintf("  CPU:    %s %5.1f%%\n", cpuBar, latest.CPUPercent))

		// Memory
		memBar := renderBar(latest.MemoryPercent, 100, 30)
		s.WriteString(fmt.Sprintf("  Memory: %s %5.1f%% (%s)\n",
			memBar,
			latest.MemoryPercent,
			formatBytes(latest.MemoryUsage)))

		// Network
		s.WriteString(fmt.Sprintf("  Network: rx=%s/s tx=%s/s (total: rx=%s tx=%s)\n",
			formatBytes(uint64(latest.NetRxRate)),
			formatBytes(uint64(latest.NetTxRate)),
			formatBytes(latest.NetRxBytes),
			formatBytes(latest.NetTxBytes)))

		// Block I/O
		s.WriteString(fmt.Sprintf("  Block:   read=%s/s write=%s/s\n",
			formatBytes(uint64(latest.BlockReadRate)),
			formatBytes(uint64(latest.BlockWriteRate))))

		// PIDs
		s.WriteString(fmt.Sprintf("  PIDs:    %d\n", latest.PidsCount))

		s.WriteString("\n")
	}

	// Error display
	if m.err != nil {
		s.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
		s.WriteString("\n")
	}

	// Help
	s.WriteString(helpStyle.Render("p: pause | q: quit"))

	return s.String()
}

func renderBar(value, max float64, width int) string {
	if max <= 0 {
		max = 100
	}
	percent := value / max
	if percent > 1 {
		percent = 1
	}
	if percent < 0 {
		percent = 0
	}

	filled := int(percent * float64(width))
	empty := width - filled

	bar := barStyle.Render(strings.Repeat("█", filled))
	bar += barBgStyle.Render(strings.Repeat("░", empty))

	return "[" + bar + "]"
}

// formatContainerStatus formats the status and uptime for display
func formatContainerStatus(c ContainerInfo) string {
	var status string

	// Parse status to determine if running
	statusLower := strings.ToLower(c.Status)
	isRunning := strings.Contains(statusLower, "up")

	if isRunning {
		// Calculate uptime
		uptime := time.Since(c.Created)
		uptimeStr := formatDuration(uptime)
		status = successStyle.Render("● running") + dimStyle.Render(fmt.Sprintf(" (%s)", uptimeStr))
	} else {
		status = dimStyle.Render("○ stopped")
	}

	return status
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	} else if d < 24*time.Hour {
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		if minutes > 0 {
			return fmt.Sprintf("%dh%dm", hours, minutes)
		}
		return fmt.Sprintf("%dh", hours)
	} else {
		days := int(d.Hours() / 24)
		hours := int(d.Hours()) % 24
		if hours > 0 {
			return fmt.Sprintf("%dd%dh", days, hours)
		}
		return fmt.Sprintf("%dd", days)
	}
}
