package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/guptarohit/asciigraph"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var (
	// Global flags
	mdokDir string
)

func main() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting home directory: %v\n", err)
		os.Exit(1)
	}
	mdokDir = filepath.Join(homeDir, ".mdok")

	rootCmd := &cobra.Command{
		Use:   "mdok",
		Short: "Monitor Docker container resource utilization",
		Long: `mdok is a CLI tool for monitoring Docker container resource utilization.
Run without arguments to interactively create a new monitoring configuration.`,
		Run: func(cmd *cobra.Command, args []string) {
			runInteractiveSetup()
		},
	}

	// start command
	startCmd := &cobra.Command{
		Use:   "start <config-name>",
		Short: "Start monitoring daemon for a configuration",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			foreground, _ := cmd.Flags().GetBool("foreground")
			runStart(args[0], foreground)
		},
	}
	startCmd.Flags().BoolP("foreground", "f", false, "Run in foreground instead of as daemon")

	// stop command
	stopCmd := &cobra.Command{
		Use:   "stop <config-name>",
		Short: "Stop monitoring daemon",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runStop(args[0])
		},
	}

	// ls command
	lsCmd := &cobra.Command{
		Use:   "ls",
		Short: "List running monitoring instances",
		Run: func(cmd *cobra.Command, args []string) {
			runList()
		},
	}

	// view command
	viewCmd := &cobra.Command{
		Use:   "view <config-name>",
		Short: "View live dashboard or summary for a configuration",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			history, _ := cmd.Flags().GetBool("history")
			runView(args[0], history)
		},
	}
	viewCmd.Flags().Bool("history", false, "View historical data instead of live dashboard")

	// export command
	exportCmd := &cobra.Command{
		Use:   "export <config-name>",
		Short: "Export monitoring data",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			format, _ := cmd.Flags().GetString("format")
			output, _ := cmd.Flags().GetString("output")
			last, _ := cmd.Flags().GetString("last")
			from, _ := cmd.Flags().GetString("from")
			to, _ := cmd.Flags().GetString("to")
			all, _ := cmd.Flags().GetBool("all")
			runExport(args[0], format, output, last, from, to, all)
		},
	}
	exportCmd.Flags().StringP("format", "F", "json", "Export format: json, csv, markdown, html")
	exportCmd.Flags().StringP("output", "o", "", "Output file path")
	exportCmd.Flags().String("last", "", "Export data from last duration (e.g., 1h, 30m)")
	exportCmd.Flags().String("from", "", "Start time (RFC3339 format)")
	exportCmd.Flags().String("to", "", "End time (RFC3339 format)")
	exportCmd.Flags().Bool("all", false, "Export all data")

	// configs command
	configsCmd := &cobra.Command{
		Use:   "configs",
		Short: "List saved configurations",
		Run: func(cmd *cobra.Command, args []string) {
			runConfigs()
		},
	}

	// edit command
	editCmd := &cobra.Command{
		Use:   "edit <config-name>",
		Short: "Edit a configuration",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runEdit(args[0])
		},
	}

	// delete command
	deleteCmd := &cobra.Command{
		Use:   "delete <config-name>",
		Short: "Delete a configuration and its data",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			force, _ := cmd.Flags().GetBool("force")
			runDelete(args[0], force)
		},
	}
	deleteCmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt")

	// logs command
	logsCmd := &cobra.Command{
		Use:   "logs <config-name>",
		Short: "View logs for a monitoring instance",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			follow, _ := cmd.Flags().GetBool("follow")
			lines, _ := cmd.Flags().GetInt("lines")
			runLogs(args[0], follow, lines)
		},
	}
	logsCmd.Flags().BoolP("follow", "f", false, "Follow log output")
	logsCmd.Flags().IntP("lines", "n", 50, "Number of lines to show")

	rootCmd.AddCommand(startCmd, stopCmd, lsCmd, viewCmd, exportCmd, configsCmd, editCmd, deleteCmd, logsCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runInteractiveSetup() {
	// Initialize Docker client
	docker, err := NewDockerClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to Docker: %v\n", err)
		os.Exit(1)
	}
	defer docker.Close()

	// Get list of running containers
	ctx := context.Background()
	containers, err := docker.ListContainers(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing containers: %v\n", err)
		os.Exit(1)
	}

	if len(containers) == 0 {
		fmt.Println("No running containers found.")
		os.Exit(0)
	}

	// Run TUI for container selection
	model := NewSelectionModel(containers)
	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}

	m := finalModel.(SelectionModel)
	if m.cancelled {
		fmt.Println("Cancelled.")
		os.Exit(0)
	}

	// Create and save configuration
	config := Config{
		Name:       m.configName,
		Containers: m.selectedContainers,
		Interval:   m.interval,
		CreatedAt:  time.Now().Format(time.RFC3339),
	}

	if err := SaveConfig(config); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving configuration: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nConfiguration '%s' saved.\n", config.Name)
	fmt.Printf("  Containers: %s\n", strings.Join(config.Containers, ", "))
	fmt.Printf("  Interval: %ds\n", config.Interval)
	fmt.Printf("\nTo start monitoring, run: mdok start %s\n", config.Name)
}

func runStart(configName string, foreground bool) {
	config, err := LoadConfig(configName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	// Check if already running
	if IsRunning(configName) {
		existingPid, _ := ReadPidFile(configName)
		if existingPid != os.Getpid() {
		  fmt.Fprintf(os.Stderr, "Monitoring for '%s' is already running.\n", configName)
		  os.Exit(1)
	        }
	}

	if foreground {
		// Run in foreground
		if err := RunMonitor(config); err != nil {
			fmt.Fprintf(os.Stderr, "Error running monitor: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Start as daemon
		if err := StartDaemon(config); err != nil {
			fmt.Fprintf(os.Stderr, "Error starting daemon: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Started monitoring '%s' in background.\n", configName)
		fmt.Printf("View logs: mdok logs %s\n", configName)
		fmt.Printf("View dashboard: mdok view %s\n", configName)
	}
}

func runStop(configName string) {
	if !IsRunning(configName) {
		fmt.Fprintf(os.Stderr, "No running instance found for '%s'.\n", configName)
		os.Exit(1)
	}

	if err := StopDaemon(configName); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping daemon: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Stopped monitoring '%s'.\n", configName)

	// Display summary
	displaySummary(configName)
}

func runList() {
	statuses, err := ListDaemons()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing daemons: %v\n", err)
		os.Exit(1)
	}

	if len(statuses) == 0 {
		fmt.Println("No running monitoring instances.")
		return
	}

	fmt.Printf("%-20s %-10s %-25s %s\n", "CONFIG", "PID", "STARTED", "CONTAINERS")
	fmt.Println(strings.Repeat("-", 80))
	for _, s := range statuses {
		containers := strings.Join(s.Containers, ", ")
		if len(containers) > 30 {
			containers = containers[:27] + "..."
		}
		fmt.Printf("%-20s %-10d %-25s %s\n",
			s.ConfigName,
			s.PID,
			s.StartTime.Format("2006-01-02 15:04:05"),
			containers,
		)
	}
}

func runView(configName string, history bool) {
	config, err := LoadConfig(configName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	// If --history flag is set, show interactive TUI or static summary
	if history {
		// Check if TTY for interactive mode
		if isatty.IsTerminal(os.Stdout.Fd()) {
			model := NewHistoryTUIModel(configName)
			p := tea.NewProgram(model, tea.WithAltScreen())
			if _, err := p.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		} else {
			// Non-TTY: fall back to static output
			displaySummary(configName)
		}
		return
	}

	// Check if daemon is running for live view
	if IsRunning(configName) {
		// Run live dashboard
		model := NewDashboardModel(config)
		p := tea.NewProgram(model, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error running dashboard: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Show static summary
		displaySummary(configName)
	}
}

func displaySummary(configName string) {
	dataDir := GetDataDir(configName)
	files, err := filepath.Glob(filepath.Join(dataDir, "*.json"))
	if err != nil || len(files) == 0 {
		fmt.Println("No monitoring data found.")
		return
	}

	fmt.Printf("\n╭─────────────────────────────────────────────────────────────────────────╮\n")
	fmt.Printf("│                   Historical Data for '%s'%-25s│\n", configName, strings.Repeat(" ", 28-len(configName)))
	fmt.Printf("╰─────────────────────────────────────────────────────────────────────────╯\n\n")

	for i, file := range files {
		if i > 0 {
			fmt.Println(strings.Repeat("─", 77))
			fmt.Println()
		}

		data, err := LoadContainerData(file)
		if err != nil {
			continue
		}

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

		// Header
		fmt.Printf("┌─────────────────────────────────────────────────────────────────────────┐\n")
		fmt.Printf("│ Container: %-63s │\n", data.ContainerName)
		fmt.Printf("│ Image: %-67s │\n", data.ImageName)
		fmt.Printf("│ ID: %-70s │\n", data.ContainerID[:12])
		fmt.Printf("└─────────────────────────────────────────────────────────────────────────┘\n\n")

		// Host Information
		fmt.Printf("Host Information:\n")
		fmt.Printf("  Hostname: %s\n", data.Host.Hostname)
		fmt.Printf("  CPU: %s (%d cores, %s)\n", data.Host.CPUModel, data.Host.CPUCores, data.Host.Architecture)
		fmt.Printf("  Memory: %s\n", formatBytes(data.Host.MemoryTotal))
		fmt.Printf("  OS: %s (kernel %s)\n", data.Host.OS, data.Host.KernelVer)
		fmt.Printf("  Docker: %s\n", data.Host.DockerVer)

		// Architecture warning
		if strings.Contains(strings.ToLower(data.Host.Architecture), "arm") ||
		   strings.Contains(strings.ToLower(data.Host.Architecture), "aarch") {
			fmt.Printf("  ⚠️  ARM architecture - EC2 x86 vCPU performance will differ\n")
		}
		fmt.Println()

		// Container Limits
		fmt.Printf("Container Resource Limits:\n")
		if data.Limits.CPUQuota > 0 && data.Limits.CPUPeriod > 0 {
			cpuLimit := float64(data.Limits.CPUQuota) / float64(data.Limits.CPUPeriod)
			fmt.Printf("  CPU Limit: %.2f cores (quota: %d, period: %d)\n",
				cpuLimit, data.Limits.CPUQuota, data.Limits.CPUPeriod)
		} else {
			fmt.Printf("  CPU Limit: unlimited\n")
		}

		if data.Limits.MemLimit > 0 {
			fmt.Printf("  Memory Limit: %s\n", formatBytes(data.Limits.MemLimit))
		} else {
			fmt.Printf("  Memory Limit: unlimited\n")
		}

		if data.Limits.PidsLimit > 0 {
			fmt.Printf("  PIDs Limit: %d\n", data.Limits.PidsLimit)
		}
		fmt.Println()

		// Monitoring Duration
		duration := data.EndTime.Sub(data.StartTime)
		fmt.Printf("Monitoring Period:\n")
		fmt.Printf("  Started: %s\n", data.StartTime.Format("2006-01-02 15:04:05"))
		fmt.Printf("  Ended: %s\n", data.EndTime.Format("2006-01-02 15:04:05"))
		fmt.Printf("  Duration: %s\n", duration.Round(time.Second))
		fmt.Printf("  Samples: %d (interval: %ds)\n\n", len(data.Samples), data.Interval)

		// Graphs
		if len(data.Samples) > 0 && data.Summary != nil {
			fmt.Printf("Resource Usage Over Time:\n\n")

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
				netTxData = append(netTxData, data.Samples[i].NetTxRate/(1024*1024)) // MB/s
				netRxData = append(netRxData, data.Samples[i].NetRxRate/(1024*1024)) // MB/s
			}

			fmt.Printf("CPU Usage (%%)\n")
			cpuGraph := asciigraph.Plot(cpuData, asciigraph.Height(10), asciigraph.Width(70),
				asciigraph.Caption(fmt.Sprintf("Min: %.1f%% | Avg: %.1f%% | Max: %.1f%% | P95: %.1f%%",
					data.Summary.CPUPercent.Min, data.Summary.CPUPercent.Avg,
					data.Summary.CPUPercent.Max, data.Summary.CPUPercent.P95)))
			fmt.Println(cpuGraph)
			fmt.Println()

			fmt.Printf("Memory Usage (MB)\n")
			memGraph := asciigraph.Plot(memData, asciigraph.Height(10), asciigraph.Width(70),
				asciigraph.Caption(fmt.Sprintf("Min: %s | Avg: %s | Max: %s | P95: %s",
					formatBytes(uint64(data.Summary.MemoryUsage.Min)),
					formatBytes(uint64(data.Summary.MemoryUsage.Avg)),
					formatBytes(uint64(data.Summary.MemoryUsage.Max)),
					formatBytes(uint64(data.Summary.MemoryUsage.P95)))))
			fmt.Println(memGraph)
			fmt.Println()

			fmt.Printf("Network TX (MB/s)\n")
			netTxGraph := asciigraph.Plot(netTxData, asciigraph.Height(8), asciigraph.Width(70),
				asciigraph.Caption(fmt.Sprintf("Total Egress: %s", formatBytes(data.Summary.NetTxTotal))))
			fmt.Println(netTxGraph)
			fmt.Println()

			fmt.Printf("Network RX (MB/s)\n")
			netRxGraph := asciigraph.Plot(netRxData, asciigraph.Height(8), asciigraph.Width(70),
				asciigraph.Caption(fmt.Sprintf("Total Ingress: %s", formatBytes(data.Summary.NetRxTotal))))
			fmt.Println(netRxGraph)
			fmt.Println()
		}

		// Summary Statistics
		if data.Summary != nil {
			s := data.Summary
			fmt.Printf("Summary Statistics:\n")
			fmt.Printf("  CPU:      min=%.1f%% avg=%.1f%% max=%.1f%% p95=%.1f%% p99=%.1f%%\n",
				s.CPUPercent.Min, s.CPUPercent.Avg, s.CPUPercent.Max, s.CPUPercent.P95, s.CPUPercent.P99)
			fmt.Printf("  Memory:   min=%s avg=%s max=%s p95=%s\n",
				formatBytes(uint64(s.MemoryUsage.Min)),
				formatBytes(uint64(s.MemoryUsage.Avg)),
				formatBytes(uint64(s.MemoryUsage.Max)),
				formatBytes(uint64(s.MemoryUsage.P95)))
			fmt.Printf("  Net I/O:  rx=%s tx=%s\n",
				formatBytes(s.NetRxTotal),
				formatBytes(s.NetTxTotal))

			// Network breakdown (if available)
			if s.NetworkBreakdown != nil {
				fmt.Printf("  Traffic:  ")
				if s.NetworkBreakdown.InterContainerPct > 0 {
					fmt.Printf("inter-container=%.1f%% ", s.NetworkBreakdown.InterContainerPct)
				}
				if s.NetworkBreakdown.InternalPct > 0 {
					fmt.Printf("internal=%.1f%% ", s.NetworkBreakdown.InternalPct)
				}
				if s.NetworkBreakdown.InternetPct > 0 {
					fmt.Printf("internet=%.1f%%", s.NetworkBreakdown.InternetPct)
				}
				fmt.Println()
			}

			fmt.Printf("  Block I/O: read=%s write=%s\n",
				formatBytes(s.BlockReadTotal),
				formatBytes(s.BlockWriteTotal))
			fmt.Printf("  PIDs:     min=%.0f avg=%.0f max=%.0f\n\n",
				s.PidsCount.Min, s.PidsCount.Avg, s.PidsCount.Max)

			// Warnings
			if len(s.Warnings) > 0 {
				fmt.Printf("⚠️  Warnings:\n")
				for _, w := range s.Warnings {
					fmt.Printf("  • %s\n", w)
				}
				fmt.Println()
			}
		}

		// Network Cost with Monthly Projection
		if data.NetworkCost != nil {
			fmt.Printf("AWS Network Cost Estimate (%s):\n", data.NetworkCost.Region)
			fmt.Printf("  Egress (this session): %.2f GB @ $%.3f/GB = $%.2f\n",
				data.NetworkCost.EgressGB,
				data.NetworkCost.PricePerGB,
				data.NetworkCost.EstimatedCostUSD)

			// Calculate monthly projection
			if duration.Hours() > 0 {
				hoursInMonth := 720.0 // 30 days
				projectedGB := data.NetworkCost.EgressGB * (hoursInMonth / duration.Hours())
				projectedCost := projectedGB * data.NetworkCost.PricePerGB
				fmt.Printf("  Monthly projection:    %.2f GB/month = $%.2f/month (at current rate)\n",
					projectedGB, projectedCost)

				if projectedCost > 50 {
					fmt.Printf("  ⚠️  High egress costs - consider same-AZ placement or caching\n")
				}
			}
			fmt.Println()
		}

		// AWS Instance Recommendations (both x86 and ARM)
		if data.Summary != nil {
			x86Rec, armRec := RecommendBothArchitectures(data.Summary)

			fmt.Printf("AWS Instance Recommendations:\n\n")

			if x86Rec != nil {
				monthlyPrice := x86Rec.HourlyPrice * 730 // hours in month
				fmt.Printf("  x86_64 (Intel/AMD):\n")
				fmt.Printf("    Instance: %s (%d vCPU, %.0f GB RAM)\n",
					x86Rec.InstanceType, x86Rec.VCPU, x86Rec.MemoryGB)
				fmt.Printf("    Cost: $%.4f/hour (~$%.2f/month)\n", x86Rec.HourlyPrice, monthlyPrice)
				fmt.Printf("    Reason: %s\n\n", x86Rec.Reason)
			}

			if armRec != nil {
				monthlyPrice := armRec.HourlyPrice * 730
				savings := 0.0
				if x86Rec != nil {
					savings = ((x86Rec.HourlyPrice - armRec.HourlyPrice) / x86Rec.HourlyPrice) * 100
				}
				fmt.Printf("  ARM64 (Graviton):\n")
				fmt.Printf("    Instance: %s (%d vCPU, %.0f GB RAM)\n",
					armRec.InstanceType, armRec.VCPU, armRec.MemoryGB)
				fmt.Printf("    Cost: $%.4f/hour (~$%.2f/month)", armRec.HourlyPrice, monthlyPrice)
				if savings > 0 {
					fmt.Printf(" [%.0f%% cheaper than x86]", savings)
				}
				fmt.Printf("\n    Reason: %s\n\n", armRec.Reason)
			}

			// Architecture note
			hostArch := strings.ToLower(data.Host.Architecture)
			if strings.Contains(hostArch, "arm") || strings.Contains(hostArch, "aarch") {
				fmt.Printf("  ℹ️  Measured on ARM hardware. Performance may differ on x86 instances.\n")
			} else if strings.Contains(hostArch, "x86") || strings.Contains(hostArch, "amd64") {
				fmt.Printf("  ℹ️  Measured on x86 hardware. ARM instances may perform differently.\n")
			}
			fmt.Printf("  ℹ️  Recommendations are estimates. Test on target instance type before committing.\n")
		}

		fmt.Println()
	}
}

func runExport(configName, format, output, last, from, to string, all bool) {
	opts := ExportOptions{
		Format: format,
		Last:   last,
		All:    all,
		Output: output,
	}

	if from != "" {
		t, err := time.Parse(time.RFC3339, from)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid from time: %v\n", err)
			os.Exit(1)
		}
		opts.From = t
	}

	if to != "" {
		t, err := time.Parse(time.RFC3339, to)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid to time: %v\n", err)
			os.Exit(1)
		}
		opts.To = t
	}

	if err := Export(configName, opts); err != nil {
		fmt.Fprintf(os.Stderr, "Error exporting data: %v\n", err)
		os.Exit(1)
	}
}

func runConfigs() {
	configs, err := ListConfigs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing configurations: %v\n", err)
		os.Exit(1)
	}

	if len(configs) == 0 {
		fmt.Println("No configurations found.")
		fmt.Println("Run 'mdok' to create a new configuration.")
		return
	}

	fmt.Printf("%-20s %-12s %-25s %s\n", "NAME", "INTERVAL", "CREATED", "CONTAINERS")
	fmt.Println(strings.Repeat("-", 80))
	for _, c := range configs {
		containers := strings.Join(c.Containers, ", ")
		if len(containers) > 30 {
			containers = containers[:27] + "..."
		}
		created := c.CreatedAt
		if t, err := time.Parse(time.RFC3339, c.CreatedAt); err == nil {
			created = t.Format("2006-01-02 15:04:05")
		}
		fmt.Printf("%-20s %-12s %-25s %s\n", c.Name, fmt.Sprintf("%ds", c.Interval), created, containers)
	}
}

func runEdit(configName string) {
	config, err := LoadConfig(configName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	if IsRunning(configName) {
		fmt.Fprintf(os.Stderr, "Cannot edit while monitoring is running. Stop it first with: mdok stop %s\n", configName)
		os.Exit(1)
	}

	// Initialize Docker client
	docker, err := NewDockerClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to Docker: %v\n", err)
		os.Exit(1)
	}
	defer docker.Close()

	// Get list of running containers
	ctx := context.Background()
	containers, err := docker.ListContainers(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing containers: %v\n", err)
		os.Exit(1)
	}

	// Run TUI for editing
	model := NewEditModel(containers, config)
	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}

	m := finalModel.(SelectionModel)
	if m.cancelled {
		fmt.Println("Cancelled.")
		os.Exit(0)
	}

	// Update configuration
	config.Containers = m.selectedContainers
	config.Interval = m.interval

	if err := SaveConfig(config); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving configuration: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Configuration '%s' updated.\n", config.Name)
}

func runDelete(configName string, force bool) {
	if !ConfigExists(configName) {
		fmt.Fprintf(os.Stderr, "Configuration '%s' not found.\n", configName)
		os.Exit(1)
	}

	if IsRunning(configName) {
		fmt.Fprintf(os.Stderr, "Cannot delete while monitoring is running. Stop it first with: mdok stop %s\n", configName)
		os.Exit(1)
	}

	if !force {
		fmt.Printf("Delete configuration '%s' and all its data? [y/N] ", configName)
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "y" {
			fmt.Println("Cancelled.")
			return
		}
	}

	if err := DeleteConfig(configName); err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting configuration: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Configuration '%s' deleted.\n", configName)
}

func runLogs(configName string, follow bool, lines int) {
	if !ConfigExists(configName) {
		fmt.Fprintf(os.Stderr, "Configuration '%s' not found.\n", configName)
		os.Exit(1)
	}

	logFile := GetLogFile(configName)
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		fmt.Println("No logs found.")
		return
	}

	if follow {
		// Use tail -f
		TailFollow(logFile)
	} else {
		// Show last N lines
		content, err := TailLines(logFile, lines)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading logs: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(content)
	}
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func parseDuration(s string) (time.Duration, error) {
	// Handle simple formats like "1h", "30m", "2d"
	if len(s) == 0 {
		return 0, fmt.Errorf("empty duration")
	}

	last := s[len(s)-1]
	numStr := s[:len(s)-1]
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return time.ParseDuration(s)
	}

	switch last {
	case 's':
		return time.Duration(num) * time.Second, nil
	case 'm':
		return time.Duration(num) * time.Minute, nil
	case 'h':
		return time.Duration(num) * time.Hour, nil
	case 'd':
		return time.Duration(num) * 24 * time.Hour, nil
	default:
		return time.ParseDuration(s)
	}
}
