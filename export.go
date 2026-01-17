package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Export exports monitoring data in the specified format
func Export(configName string, opts ExportOptions) error {
	// Load all container data
	allData, err := LoadAllContainerData(configName)
	if err != nil {
		return fmt.Errorf("failed to load data: %w", err)
	}

	if len(allData) == 0 {
		return fmt.Errorf("no monitoring data found for '%s'", configName)
	}

	// Filter samples by time if specified
	if !opts.All {
		allData = filterDataByTime(allData, opts)
	}

	// Generate output
	var output string
	var outputBytes []byte

	switch opts.Format {
	case "json":
		outputBytes, err = exportJSON(allData)
	case "csv":
		output, err = exportCSV(allData)
	case "markdown", "md":
		output, err = exportMarkdown(configName, allData)
	case "html":
		output, err = exportHTML(configName, allData)
	default:
		return fmt.Errorf("unsupported format: %s", opts.Format)
	}

	if err != nil {
		return err
	}

	// Write to file or stdout
	if opts.Output != "" {
		if outputBytes != nil {
			err = os.WriteFile(opts.Output, outputBytes, 0644)
		} else {
			err = os.WriteFile(opts.Output, []byte(output), 0644)
		}
		if err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		fmt.Printf("Exported to %s\n", opts.Output)
	} else {
		if outputBytes != nil {
			fmt.Print(string(outputBytes))
		} else {
			fmt.Print(output)
		}
	}

	return nil
}

// filterDataByTime filters samples based on time options
func filterDataByTime(allData []*ContainerData, opts ExportOptions) []*ContainerData {
	var from, to time.Time

	if opts.Last != "" {
		duration, err := parseDuration(opts.Last)
		if err == nil {
			from = time.Now().Add(-duration)
			to = time.Now()
		}
	} else {
		if !opts.From.IsZero() {
			from = opts.From
		}
		if !opts.To.IsZero() {
			to = opts.To
		}
	}

	if from.IsZero() && to.IsZero() {
		return allData
	}

	// Filter samples
	for _, data := range allData {
		var filtered []Sample
		for _, s := range data.Samples {
			if !from.IsZero() && s.Timestamp.Before(from) {
				continue
			}
			if !to.IsZero() && s.Timestamp.After(to) {
				continue
			}
			filtered = append(filtered, s)
		}
		data.Samples = filtered
	}

	return allData
}

// exportJSON exports data as JSON
func exportJSON(allData []*ContainerData) ([]byte, error) {
	return json.MarshalIndent(allData, "", "  ")
}

// exportCSV exports summary data as CSV
func exportCSV(allData []*ContainerData) (string, error) {
	var buf strings.Builder
	writer := csv.NewWriter(&buf)

	// Write header
	header := []string{
		"Container", "Samples", "Duration",
		"CPU Min%", "CPU Avg%", "CPU Max%", "CPU P95%",
		"Mem Min", "Mem Avg", "Mem Max", "Mem P95",
		"Net Rx Total", "Net Tx Total",
		"Block Read Total", "Block Write Total",
	}
	writer.Write(header)

	// Write data rows
	for _, data := range allData {
		if data.Summary == nil {
			continue
		}
		s := data.Summary

		row := []string{
			data.ContainerName,
			fmt.Sprintf("%d", s.SampleCount),
			s.Duration,
			fmt.Sprintf("%.2f", s.CPUPercent.Min),
			fmt.Sprintf("%.2f", s.CPUPercent.Avg),
			fmt.Sprintf("%.2f", s.CPUPercent.Max),
			fmt.Sprintf("%.2f", s.CPUPercent.P95),
			formatBytes(uint64(s.MemoryUsage.Min)),
			formatBytes(uint64(s.MemoryUsage.Avg)),
			formatBytes(uint64(s.MemoryUsage.Max)),
			formatBytes(uint64(s.MemoryUsage.P95)),
			formatBytes(s.NetRxTotal),
			formatBytes(s.NetTxTotal),
			formatBytes(s.BlockReadTotal),
			formatBytes(s.BlockWriteTotal),
		}
		writer.Write(row)
	}

	writer.Flush()
	return buf.String(), writer.Error()
}

// exportMarkdown exports data as Markdown
func exportMarkdown(configName string, allData []*ContainerData) (string, error) {
	var buf strings.Builder

	buf.WriteString(fmt.Sprintf("# Monitoring Report: %s\n\n", configName))
	buf.WriteString(fmt.Sprintf("Generated: %s\n\n", time.Now().Format(time.RFC3339)))

	for _, data := range allData {
		buf.WriteString(fmt.Sprintf("## %s\n\n", data.ContainerName))
		buf.WriteString(fmt.Sprintf("- **Container ID:** %s\n", data.ContainerID[:12]))
		buf.WriteString(fmt.Sprintf("- **Image:** %s\n", data.ImageName))
		buf.WriteString(fmt.Sprintf("- **Host:** %s\n", data.Host.Hostname))
		buf.WriteString(fmt.Sprintf("- **Start:** %s\n", data.StartTime.Format(time.RFC3339)))
		buf.WriteString(fmt.Sprintf("- **End:** %s\n", data.EndTime.Format(time.RFC3339)))
		buf.WriteString("\n")

		if data.Summary != nil {
			s := data.Summary
			buf.WriteString("### Summary Statistics\n\n")
			buf.WriteString(fmt.Sprintf("- **Samples:** %d\n", s.SampleCount))
			buf.WriteString(fmt.Sprintf("- **Duration:** %s\n\n", s.Duration))

			buf.WriteString("| Metric | Min | Avg | Max | P95 | P99 |\n")
			buf.WriteString("|--------|-----|-----|-----|-----|-----|\n")
			buf.WriteString(fmt.Sprintf("| CPU %% | %.1f | %.1f | %.1f | %.1f | %.1f |\n",
				s.CPUPercent.Min, s.CPUPercent.Avg, s.CPUPercent.Max, s.CPUPercent.P95, s.CPUPercent.P99))
			buf.WriteString(fmt.Sprintf("| Memory | %s | %s | %s | %s | %s |\n",
				formatBytes(uint64(s.MemoryUsage.Min)),
				formatBytes(uint64(s.MemoryUsage.Avg)),
				formatBytes(uint64(s.MemoryUsage.Max)),
				formatBytes(uint64(s.MemoryUsage.P95)),
				formatBytes(uint64(s.MemoryUsage.P99))))
			buf.WriteString(fmt.Sprintf("| Memory %% | %.1f | %.1f | %.1f | %.1f | %.1f |\n",
				s.MemoryPercent.Min, s.MemoryPercent.Avg, s.MemoryPercent.Max, s.MemoryPercent.P95, s.MemoryPercent.P99))
			buf.WriteString("\n")

			buf.WriteString("### Network & I/O Totals\n\n")
			buf.WriteString(fmt.Sprintf("- **Network Rx:** %s\n", formatBytes(s.NetRxTotal)))
			buf.WriteString(fmt.Sprintf("- **Network Tx:** %s\n", formatBytes(s.NetTxTotal)))
			buf.WriteString(fmt.Sprintf("- **Block Read:** %s\n", formatBytes(s.BlockReadTotal)))
			buf.WriteString(fmt.Sprintf("- **Block Write:** %s\n", formatBytes(s.BlockWriteTotal)))
			buf.WriteString("\n")

			if len(s.Warnings) > 0 {
				buf.WriteString("### Warnings\n\n")
				for _, w := range s.Warnings {
					buf.WriteString(fmt.Sprintf("- ⚠️ %s\n", w))
				}
				buf.WriteString("\n")
			}
		}

		if data.NetworkCost != nil {
			buf.WriteString("### AWS Network Cost Estimate\n\n")
			buf.WriteString(fmt.Sprintf("- **Region:** %s\n", data.NetworkCost.Region))
			buf.WriteString(fmt.Sprintf("- **Egress:** %.2f GB\n", data.NetworkCost.EgressGB))
			buf.WriteString(fmt.Sprintf("- **Estimated Cost:** $%.2f\n", data.NetworkCost.EstimatedCostUSD))
			buf.WriteString("\n")
		}

		if data.Recommendation != nil {
			buf.WriteString("### Instance Recommendation\n\n")
			buf.WriteString(fmt.Sprintf("- **Type:** %s (%d vCPU, %.1f GB RAM)\n",
				data.Recommendation.InstanceType,
				data.Recommendation.VCPU,
				data.Recommendation.MemoryGB))
			buf.WriteString(fmt.Sprintf("- **Hourly Cost:** $%.4f\n", data.Recommendation.HourlyPrice))
			buf.WriteString(fmt.Sprintf("- **Reason:** %s\n", data.Recommendation.Reason))
			buf.WriteString("\n")
		}

		buf.WriteString("---\n\n")
	}

	return buf.String(), nil
}

// exportHTML exports data as HTML with Chart.js
func exportHTML(configName string, allData []*ContainerData) (string, error) {
	var buf strings.Builder

	buf.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>mdok Report: ` + configName + `</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            line-height: 1.6;
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
            background: #f5f5f5;
        }
        .container-section {
            background: white;
            border-radius: 8px;
            padding: 20px;
            margin-bottom: 20px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        h1, h2, h3 { color: #333; }
        .chart-container {
            position: relative;
            height: 300px;
            margin: 20px 0;
        }
        table {
            width: 100%;
            border-collapse: collapse;
            margin: 10px 0;
        }
        th, td {
            padding: 8px 12px;
            text-align: left;
            border-bottom: 1px solid #ddd;
        }
        th { background: #f8f9fa; }
        .warning {
            background: #fff3cd;
            border: 1px solid #ffc107;
            padding: 10px;
            border-radius: 4px;
            margin: 10px 0;
        }
        .metric-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 15px;
            margin: 15px 0;
        }
        .metric-card {
            background: #f8f9fa;
            padding: 15px;
            border-radius: 4px;
        }
        .metric-value {
            font-size: 24px;
            font-weight: bold;
            color: #205493;
        }
        .metric-label {
            color: #666;
            font-size: 14px;
        }
    </style>
</head>
<body>
    <h1>Monitoring Report: ` + configName + `</h1>
    <p>Generated: ` + time.Now().Format("2006-01-02 15:04:05") + `</p>
`)

	for i, data := range allData {
		chartID := fmt.Sprintf("chart%d", i)

		buf.WriteString(fmt.Sprintf(`
    <div class="container-section">
        <h2>%s</h2>
        <p><strong>Image:</strong> %s | <strong>Container ID:</strong> %s</p>
        <p><strong>Host:</strong> %s | <strong>Duration:</strong> %s</p>
`, data.ContainerName, data.ImageName, data.ContainerID[:12], data.Host.Hostname,
			data.EndTime.Sub(data.StartTime).Round(time.Second)))

		if data.Summary != nil {
			s := data.Summary

			// Metric cards
			buf.WriteString(`
        <div class="metric-grid">
            <div class="metric-card">
                <div class="metric-value">` + fmt.Sprintf("%.1f%%", s.CPUPercent.Avg) + `</div>
                <div class="metric-label">CPU Average</div>
            </div>
            <div class="metric-card">
                <div class="metric-value">` + formatBytes(uint64(s.MemoryUsage.Avg)) + `</div>
                <div class="metric-label">Memory Average</div>
            </div>
            <div class="metric-card">
                <div class="metric-value">` + formatBytes(s.NetTxTotal) + `</div>
                <div class="metric-label">Network Egress</div>
            </div>
            <div class="metric-card">
                <div class="metric-value">` + fmt.Sprintf("%d", s.SampleCount) + `</div>
                <div class="metric-label">Samples</div>
            </div>
        </div>
`)

			// Summary table
			buf.WriteString(`
        <h3>Statistics</h3>
        <table>
            <tr><th>Metric</th><th>Min</th><th>Avg</th><th>Max</th><th>P95</th><th>P99</th></tr>
            <tr>
                <td>CPU %</td>
                <td>` + fmt.Sprintf("%.1f", s.CPUPercent.Min) + `</td>
                <td>` + fmt.Sprintf("%.1f", s.CPUPercent.Avg) + `</td>
                <td>` + fmt.Sprintf("%.1f", s.CPUPercent.Max) + `</td>
                <td>` + fmt.Sprintf("%.1f", s.CPUPercent.P95) + `</td>
                <td>` + fmt.Sprintf("%.1f", s.CPUPercent.P99) + `</td>
            </tr>
            <tr>
                <td>Memory</td>
                <td>` + formatBytes(uint64(s.MemoryUsage.Min)) + `</td>
                <td>` + formatBytes(uint64(s.MemoryUsage.Avg)) + `</td>
                <td>` + formatBytes(uint64(s.MemoryUsage.Max)) + `</td>
                <td>` + formatBytes(uint64(s.MemoryUsage.P95)) + `</td>
                <td>` + formatBytes(uint64(s.MemoryUsage.P99)) + `</td>
            </tr>
        </table>
`)

			// Warnings
			if len(s.Warnings) > 0 {
				buf.WriteString(`        <h3>Warnings</h3>`)
				for _, w := range s.Warnings {
					buf.WriteString(fmt.Sprintf(`        <div class="warning">⚠️ %s</div>`, w))
				}
			}
		}

		// Chart
		if len(data.Samples) > 0 {
			buf.WriteString(fmt.Sprintf(`
        <h3>Resource Usage Over Time</h3>
        <div class="chart-container">
            <canvas id="%s"></canvas>
        </div>
        <script>
            new Chart(document.getElementById('%s'), {
                type: 'line',
                data: {
                    labels: [%s],
                    datasets: [{
                        label: 'CPU %%',
                        data: [%s],
                        borderColor: 'rgb(75, 192, 192)',
                        tension: 0.1,
                        yAxisID: 'y'
                    }, {
                        label: 'Memory %%',
                        data: [%s],
                        borderColor: 'rgb(255, 99, 132)',
                        tension: 0.1,
                        yAxisID: 'y'
                    }]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    scales: {
                        y: {
                            type: 'linear',
                            display: true,
                            position: 'left',
                            min: 0,
                            max: 100,
                            title: { display: true, text: 'Percentage' }
                        }
                    }
                }
            });
        </script>
`, chartID, chartID, generateChartLabels(data.Samples), generateChartData(data.Samples, "cpu"), generateChartData(data.Samples, "mem")))
		}

		buf.WriteString(`    </div>
`)
	}

	buf.WriteString(`</body>
</html>
`)

	return buf.String(), nil
}

// generateChartLabels generates JavaScript array of timestamps
func generateChartLabels(samples []Sample) string {
	var labels []string
	// Limit to 100 points for readability
	step := 1
	if len(samples) > 100 {
		step = len(samples) / 100
	}

	for i := 0; i < len(samples); i += step {
		labels = append(labels, fmt.Sprintf("'%s'", samples[i].Timestamp.Format("15:04:05")))
	}
	return strings.Join(labels, ",")
}

// generateChartData generates JavaScript array of values
func generateChartData(samples []Sample, metric string) string {
	var values []string
	step := 1
	if len(samples) > 100 {
		step = len(samples) / 100
	}

	for i := 0; i < len(samples); i += step {
		var val float64
		switch metric {
		case "cpu":
			val = samples[i].CPUPercent
		case "mem":
			val = samples[i].MemoryPercent
		}
		values = append(values, fmt.Sprintf("%.2f", val))
	}
	return strings.Join(values, ",")
}
