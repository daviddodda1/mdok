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
	cursor             int
	selected           map[int]bool
	selectedContainers []string
	configName         string
	interval           int
	phase              int // 0=selection, 1=interval, 2=name
	intervalInput      textinput.Model
	nameInput          textinput.Model
	cancelled          bool
	err                error
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

	return SelectionModel{
		containers:    containers,
		selected:      make(map[int]bool),
		intervalInput: intervalInput,
		nameInput:     nameInput,
		interval:      5,
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

func (m SelectionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.phase == 0 {
				m.cancelled = true
				return m, tea.Quit
			}
		case "esc":
			if m.phase > 0 {
				m.phase--
				return m, nil
			}
			m.cancelled = true
			return m, tea.Quit
		case "up", "k":
			if m.phase == 0 && m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.phase == 0 && m.cursor < len(m.containers)-1 {
				m.cursor++
			}
		case " ":
			if m.phase == 0 {
				m.selected[m.cursor] = !m.selected[m.cursor]
			}
		case "a":
			if m.phase == 0 {
				// Select all
				allSelected := len(m.selected) == len(m.containers)
				m.selected = make(map[int]bool)
				if !allSelected {
					for i := range m.containers {
						m.selected[i] = true
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
	var cmd tea.Cmd
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
		s.WriteString("\n\n")

		for i, c := range m.containers {
			cursor := "  "
			if m.cursor == i {
				cursor = cursorStyle.Render("> ")
			}

			checked := "[ ]"
			if m.selected[i] {
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

		s.WriteString("\n")
		s.WriteString(helpStyle.Render("space: toggle | a: select all | enter: continue | q: quit"))

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
