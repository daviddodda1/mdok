# mdok

A command-line tool for monitoring Docker container resource utilization with AWS cost estimation and capacity planning insights.

## Features

- **Interactive Container Selection** - Easy TUI for selecting containers with live status and uptime display
- **Background Monitoring** - Run as daemon with persistent data collection
- **Comprehensive Metrics** - CPU, memory, network, block I/O, and PIDs
- **Statistical Analysis** - Min/Max/Avg/P95/P99 calculations for capacity planning
- **AWS Cost Estimates** - Data transfer cost projections based on egress traffic
- **Instance Recommendations** - AWS EC2 instance type suggestions based on usage patterns
- **Warning Detection** - Automatic alerts for resource limits, throttling, OOM risks
- **Multiple Export Formats** - JSON, CSV, Markdown, HTML with interactive charts
- **Live Dashboard** - Real-time monitoring with visual progress bars

## Installation

### Prerequisites

- Go 1.21 or later
- Docker daemon running
- Docker API access (usually via `/var/run/docker.sock`)

### Build from Source

```bash
# Clone the repository
git clone https://github.com/yourusername/mdok.git
cd mdok

# Build the binary
go build -o mdok .

# Optional: Install to system path
sudo mv mdok /usr/local/bin/
```

### Cross-Compilation

```bash
# For Linux
GOOS=linux GOARCH=amd64 go build -o mdok-linux .

# For macOS ARM (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o mdok-macos-arm64 .

# For Windows
GOOS=windows GOARCH=amd64 go build -o mdok.exe .
```

## Quick Start

### 1. Create a Monitoring Configuration

Run mdok interactively to select containers and create a configuration:

```bash
mdok
```

This will:
1. Show a list of running containers with live status (● running / ○ stopped) and uptime
2. Let you select containers with `space`
3. Set monitoring interval (default: 5 seconds)
4. Name your configuration

See [EXAMPLES.md](EXAMPLES.md) for detailed usage examples and screenshots.

### 2. Start Monitoring

Start the monitoring daemon in the background:

```bash
mdok start my-config
```

### 3. View Live Dashboard

Watch real-time metrics while monitoring:

```bash
mdok view my-config
```

### 4. Stop and Get Summary

Stop monitoring and view the summary:

```bash
mdok stop my-config
```

## Usage

### Configuration Management

```bash
# List all saved configurations
mdok configs

# Edit an existing configuration
mdok edit my-config

# Delete a configuration
mdok delete my-config
mdok delete my-config --force  # Skip confirmation
```

### Running Instances

```bash
# Start monitoring (background daemon)
mdok start my-config

# Start in foreground (for debugging)
mdok start my-config --foreground

# List running instances
mdok ls

# Stop a specific instance
mdok stop my-config

# View logs
mdok logs my-config
mdok logs my-config --follow    # Follow log output
mdok logs my-config --lines 100 # Show last 100 lines
```

### Viewing Data

```bash
# View live dashboard (if running)
mdok view my-config

# View summary (works for stopped instances too)
mdok view my-config
```

### Exporting Reports

Export monitoring data in various formats:

```bash
# Export as JSON
mdok export my-config --format json --output report.json

# Export as CSV for spreadsheet analysis
mdok export my-config --format csv --output report.csv

# Export as Markdown for documentation
mdok export my-config --format markdown --output report.md

# Export as HTML with interactive charts
mdok export my-config --format html --output report.html
```

#### Time Filters

Control what data to export:

```bash
# Last hour
mdok export my-config --last 1h --format html

# Last 24 hours (default if no filter specified)
mdok export my-config --last 24h --format json

# Last 7 days
mdok export my-config --last 7d --format csv

# Specific date range (RFC3339 format)
mdok export my-config --from "2025-01-15T00:00:00Z" --to "2025-01-16T00:00:00Z" --format html

# All collected data
mdok export my-config --all --format json
```

## Metrics Collected

### CPU Metrics
- CPU percentage (% of host CPU)
- CPU count (available cores)
- System and user time
- Throttling information

### Memory Metrics
- Memory usage and limit
- Memory percentage
- Cache and RSS
- Swap usage

### Network I/O
- Bytes received/transmitted
- Current rates (bytes/sec)
- Packet counts
- Errors and dropped packets

### Block I/O
- Bytes read/written
- Read/write operation counts
- Current I/O rates

### Container State
- Process (PID) count and limits
- Container status
- Health check status

## Statistical Analysis

For each metric, mdok calculates:

- **Min** - Minimum observed value
- **Max** - Maximum observed value
- **Avg** - Average over the monitoring period
- **P95** - 95th percentile (useful for capacity planning)
- **P99** - 99th percentile (useful for SLA planning)

## AWS Cost Estimation

mdok provides AWS data transfer cost estimates based on:

- **Egress traffic** (outbound data from containers)
- **Regional pricing** (defaults to us-east-1)
- **Monthly projections** based on current usage rates

**Note**: These are estimates. Actual AWS costs may vary based on:
- NAT Gateway costs
- Inter-AZ traffic
- Same-region S3 transfers (often free)
- Reserved capacity discounts

## Instance Recommendations

Based on your container's resource usage (CPU P95, memory P95), mdok suggests appropriate AWS EC2 instance types with:

- vCPU count
- Memory allocation
- Hourly cost estimates
- Reasoning (CPU-bound vs memory-bound)

**Important**: Recommendations include caveats about architecture differences (ARM vs x86, hyperthreading) and always recommend load testing on target infrastructure.

## Warning Detection

mdok automatically detects and warns about:

- **Memory** - Usage approaching limits, OOM risk
- **CPU** - High sustained usage, throttling
- **Network** - High egress traffic (cost implications)
- **PIDs** - Process count approaching limits

## Data Storage

mdok stores all data in `~/.mdok/`:

```
~/.mdok/
├── configs/              # Saved configurations
│   ├── prod-api.json
│   └── web-tier.json
├── data/                 # Monitoring data
│   ├── prod-api/
│   │   ├── my-web-app.json
│   │   └── nginx-proxy.json
│   └── web-tier/
│       └── frontend.json
├── pids/                 # PID files for running daemons
│   └── prod-api.pid
└── logs/                 # Daemon logs
    └── prod-api.log
```

## Example Output

### Terminal Summary

```
=== Summary for 'prod-api' ===

Container: my-web-app (abc123def456)
Image: nginx:latest
Duration: 2h30m
Samples: 1800

  CPU:    min=1.2% avg=12.4% max=67.3% p95=45.2%
  Memory: min=128MB avg=245MB max=512MB p95=466MB
  Network: rx=12.4GB tx=5.8GB
  Block I/O: read=1.2GB write=450MB

  Warnings:
    - Memory usage reached 95%+ of limit - OOM risk

  AWS Network Cost Estimate:
    Egress: 5.80 GB @ $0.090/GB = $0.52

  Instance Recommendation: t3.small (2 vCPU, 2.0 GB RAM)
    Reason: CPU-bound workload (P95: 45.2%)
```

### HTML Report

The HTML export creates a self-contained file with:
- Interactive Chart.js graphs
- Responsive design
- Summary tables
- Cost estimates
- Warning highlights
- No external dependencies

## Resource Overhead

mdok is designed to be lightweight:

- **Memory**: ~10-30MB depending on container count
- **CPU**: <0.5% average, brief bursts during collection
- **Disk**: ~1-2KB per sample per container
- **Network**: None (uses local Docker socket)

Example disk usage:
- 1 hour @ 5s interval: ~1.5MB per container
- 24 hours @ 5s interval: ~35MB per container
- 24 hours @ 30s interval: ~6MB per container

## Limitations

- **Local Docker only** - No remote Docker host support (yet)
- **No alerting** - Warnings are only shown in reports
- **Single host** - Cannot monitor multiple Docker hosts
- **Linux-optimized** - Some features may not work on Windows/macOS

## Troubleshooting

### Permission Denied

If you get permission errors accessing Docker:

```bash
# Add your user to the docker group
sudo usermod -aG docker $USER

# Log out and back in, or use:
newgrp docker
```

### Container Not Found

If a container stops during monitoring, mdok will:
- Log the event
- Continue monitoring other containers
- Include partial data in reports

### High Memory Usage

If mdok uses too much memory:
- Increase monitoring interval
- Monitor fewer containers simultaneously
- The dashboard keeps only last 100 samples in memory

## Architecture

mdok is built with:

- **[Cobra](https://github.com/spf13/cobra)** - CLI framework
- **[Bubbletea](https://github.com/charmbracelet/bubbletea)** - TUI components
- **[Lipgloss](https://github.com/charmbracelet/lipgloss)** - Terminal styling
- **[Docker SDK](https://github.com/docker/docker)** - Docker API client
- **[Chart.js](https://www.chartjs.org/)** - HTML report charts (embedded)

## Contributing

Contributions are welcome! Areas for improvement:

- Remote Docker host support
- Prometheus/Grafana integration
- Alert system with notifications
- More export formats (InfluxDB, Elasticsearch)
- Container log capture
- Historical data comparison
- Multi-host monitoring

## License

MIT License - See LICENSE file for details

## Acknowledgments

Built following the comprehensive specification in `spec.md`. Special thanks to the Charm CLI tooling ecosystem for making beautiful terminal UIs possible.

## Support

For issues, questions, or feature requests, please open an issue on GitHub.

---

**Made with ❤️ for Docker capacity planning and cloud cost optimization**
