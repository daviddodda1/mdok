# mdok Product Specification

## Overview

mdok is a command-line application for monitoring Docker container resource utilization. It's a Go binary that allows users to select containers, configure monitoring frequency, and persist metrics to JSON files (one per container). Designed for capacity planning, it captures host system info, container resource limits, and provides AWS cost estimates for data transfer.

---

## User Stories

### Setup & Configuration

1. **As a user**, I want to run `mdok` to interactively select containers and create a named monitoring configuration.

2. **As a user**, I want to set the data collection frequency when creating a config so I can balance granularity vs resource usage.

3. **As a user**, I want to save multiple configs (e.g., `prod-db`, `web-tier`) so I can monitor different container groups independently.

4. **As a user**, I want to edit an existing config to add/remove containers without starting from scratch.

### Running & Managing Instances

5. **As a user**, I want to start a monitoring instance with `mdok start <config-name>` so it runs in the background.

6. **As a user**, I want to run multiple instances simultaneously (one per config) so I can monitor different groups with different settings.

7. **As a user**, I want to see all running instances with `mdok ls` so I know what's currently being monitored.

8. **As a user**, I want to stop a specific instance with `mdok stop <config-name>` so I can end monitoring gracefully.

### Viewing Data

9. **As a user**, I want to view live stats or a summary for a running instance with `mdok view <config-name>`.

10. **As a user**, I want each instance to write data to its own folder (`~/.mdok/data/<config-name>/`) so data stays organized.

### Exporting Reports

11. **As a user**, I want to export data for a time period as JSON so I can process it with other tools.

12. **As a user**, I want to export data as a self-contained HTML file with graphs so I can share it with teammates or archive it.

13. **As a user**, I want to filter exports by time range (`--last 24h`, `--from/--to`) so I can focus on relevant periods.

---

## Metrics to Monitor

### CPU Metrics

| Metric | Description | Unit |
|--------|-------------|------|
| `cpu_percent` | Percentage of host CPU used by container | % |
| `cpu_count` | Number of CPU cores available | Count |
| `cpu_system_time` | CPU time spent in kernel mode | Nanoseconds |
| `cpu_user_time` | CPU time spent in user mode | Nanoseconds |
| `cpu_throttled_periods` | Number of throttling periods | Count |
| `cpu_throttled_time` | Total throttled time | Nanoseconds |

### Memory Metrics

| Metric | Description | Unit |
|--------|-------------|------|
| `memory_usage` | Current memory usage | Bytes |
| `memory_limit` | Memory limit assigned to container | Bytes |
| `memory_percent` | Percentage of limit used | % |
| `memory_cache` | Memory used for cache | Bytes |
| `memory_rss` | Resident set size (non-swap physical memory) | Bytes |
| `memory_swap` | Swap usage | Bytes |

### Network I/O Metrics

| Metric | Description | Unit |
|--------|-------------|------|
| `net_rx_bytes` | Total bytes received | Bytes |
| `net_tx_bytes` | Total bytes transmitted | Bytes |
| `net_rx_rate` | Current receive rate | Bytes/sec |
| `net_tx_rate` | Current transmit rate | Bytes/sec |
| `net_rx_packets` | Total packets received | Count |
| `net_tx_packets` | Total packets transmitted | Count |
| `net_rx_errors` | Receive errors | Count |
| `net_tx_errors` | Transmit errors | Count |
| `net_rx_dropped` | Dropped incoming packets | Count |
| `net_tx_dropped` | Dropped outgoing packets | Count |

### Block I/O Metrics

| Metric | Description | Unit |
|--------|-------------|------|
| `block_read_bytes` | Total bytes read from disk | Bytes |
| `block_write_bytes` | Total bytes written to disk | Bytes |
| `block_read_ops` | Number of read operations | Count |
| `block_write_ops` | Number of write operations | Count |

### Container State Metrics

| Metric | Description | Unit |
|--------|-------------|------|
| `pids_current` | Current number of processes | Count |
| `pids_limit` | Maximum allowed processes | Count |
| `status` | Container status (running, paused, etc.) | String |
| `health_status` | Health check status if configured | String |

---

## CPU Normalization & vCPU Mapping

### The Problem

Docker reports CPU as a percentage of total host capacity, where 100% = one full core. However, this doesn't map directly to AWS vCPUs because:

| Platform | What is "1 CPU"? | Performance Characteristics |
|----------|------------------|----------------------------|
| Intel/AMD EC2 | 1 hyper-thread (half physical core) | Variable, shared with sibling thread |
| AWS Graviton (ARM) | 1 physical core | Consistent, no hyperthreading |
| Apple Silicon | 1 core (P or E) | High single-thread perf |
| Generic x86 laptop | 1 hyper-thread | Varies by model |

**Example:** 200% CPU on an M2 MacBook doesn't mean "need 2 EC2 vCPUs" — it could need 3-4 vCPUs depending on workload and instance type.

### What mdok Does

1. **Records host info** — Every JSON file includes complete host CPU details (model, architecture, core count, frequency) so you know the baseline.

2. **Records container limits** — Shows what Docker limits were set (cpu_quota, cpu_shares, memory_limit) so you can compare actual usage vs allocated.

3. **Shows raw percentages** — No magic conversion; you see exactly what Docker reported.

4. **Provides rough guidance** — AWS suggestions include caveats about architecture differences.

5. **Recommends load testing** — The tool helps you estimate, but real sizing requires testing on target infrastructure.

### Container Resource Limits

mdok captures the Docker resource constraints so you can see if containers are hitting their limits:

| Limit | Description | Sizing Insight |
|-------|-------------|----------------|
| `cpu_quota` / `cpu_period` | CPU time allocation | If throttled_periods > 0, container needs more CPU |
| `cpu_shares` | Relative CPU weight | Only matters under contention |
| `memory_limit` | Hard memory cap | If usage ≈ limit, container may be OOM-killed |
| `memory_reservation` | Soft memory target | Scheduling hint, not enforced |
| `pids_limit` | Max process count | If pids_current ≈ pids_limit, may need increase |

---

## Aggregated Statistics (for AWS Sizing)

Each container's JSON file should include a running `summary` object that's updated with each sample. This provides the data needed for capacity planning decisions.

### Summary Metrics

| Metric | Description | AWS Sizing Use |
|--------|-------------|----------------|
| `cpu_min` | Minimum CPU % observed | Baseline/idle cost |
| `cpu_max` | Maximum CPU % observed | Peak capacity needed |
| `cpu_avg` | Average CPU % | Right-sizing instance type |
| `cpu_p95` | 95th percentile CPU % | Burst capacity planning |
| `memory_min` | Minimum memory usage | Baseline allocation |
| `memory_max` | Maximum memory usage | **Instance memory sizing** |
| `memory_avg` | Average memory usage | Cost optimization |
| `net_rx_rate_min` | Minimum inbound rate | Baseline bandwidth |
| `net_rx_rate_max` | Maximum inbound rate | **Network capacity sizing** |
| `net_rx_rate_avg` | Average inbound rate | Data transfer cost estimate |
| `net_tx_rate_min` | Minimum outbound rate | Baseline bandwidth |
| `net_tx_rate_max` | Maximum outbound rate | **Network capacity sizing** |
| `net_tx_rate_avg` | Average outbound rate | Data transfer cost estimate |
| `total_net_rx` | Total bytes received | Data transfer cost |
| `total_net_tx` | Total bytes transmitted | Data transfer cost |

---

## Human-Readable Output

### Console Summary (on exit or with `--summary` flag)

When monitoring stops, display a capacity planning summary:

```
╭─────────────────────────────────────────────────────────────────╮
│                    mdok Summary Report                     │
│                 Monitored: 2h 34m | Samples: 1,847              │
╰─────────────────────────────────────────────────────────────────╯

Host: dev-macbook.local
  CPU:  Apple M2 Pro (10 cores, ARM64)
  RAM:  16 GB
  ⚠️   ARM architecture — EC2 vCPU performance will differ

┌─────────────────────────────────────────────────────────────────┐
│ my-web-app                                                      │
│ Limits: 1 CPU / 512 MB memory                                   │
├─────────────────────────────────────────────────────────────────┤
│ CPU Usage                                                       │
│   Min: 1.2%    Avg: 12.4%    Max: 67.3%    P95: 45.2%          │
│   Throttled: 0 periods (not CPU constrained)                    │
│                                                                 │
│ Memory                                                          │
│   Min: 128 MB    Avg: 245 MB    Max: 512 MB    P95: 466 MB     │
│   ⚠️  Hit memory limit — may need more headroom                 │
│                                                                 │
│ Network                                                         │
│   Inbound:  Avg: 2.3 MB/s    Max: 18.7 MB/s    Total: 12.4 GB  │
│   Outbound: Avg: 1.1 MB/s    Max: 8.2 MB/s     Total: 5.8 GB   │
│                                                                 │
│ AWS Cost Estimate (us-east-1)                                   │
│   Data transfer:  $0.52 this session                            │
│   Monthly proj:   $147/mo outbound (at current rate)            │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│ postgres-db                                                     │
│ Limits: 4 CPU / 4 GB memory                                     │
├─────────────────────────────────────────────────────────────────┤
│ CPU Usage                                                       │
│   Min: 0.5%    Avg: 8.1%    Max: 234.5%    P95: 89.3%          │
│   Throttled: 847 periods (CPU constrained!)                     │
│                                                                 │
│ Memory                                                          │
│   Min: 1.2 GB    Avg: 2.8 GB    Max: 3.9 GB    P95: 3.6 GB     │
│   Using 97% of limit at peak                                    │
│                                                                 │
│ Network                                                         │
│   Inbound:  Avg: 45.2 MB/s    Max: 312.8 MB/s    Total: 892 GB │
│   Outbound: Avg: 12.3 MB/s    Max: 156.4 MB/s    Total: 245 GB │
│                                                                 │
│ AWS Cost Estimate (us-east-1)                                   │
│   Data transfer:  $22.05 this session                           │
│   Monthly proj:   $6,328/mo outbound (at current rate)          │
│   ⚠️  High egress — consider same-AZ placement or caching       │
└─────────────────────────────────────────────────────────────────┘

Network Cost Summary (all containers)
  Total Inbound:   904.4 GB   (free)
  Total Outbound:  250.8 GB   ($22.57 this session)
  Monthly Projection: $6,475/mo at current rate

AWS Instance Suggestions (rough guidance, verify with load testing):
  my-web-app  →  t3.small   (2 vCPU, 2 GB)   ~$15/mo
  postgres-db →  r6i.xlarge (4 vCPU, 32 GB)  ~$180/mo
```

### CSV Export (with `--export csv` flag)

For spreadsheet analysis:

```csv
container,cpu_min,cpu_avg,cpu_max,cpu_p95,mem_min_mb,mem_avg_mb,mem_max_mb,net_in_avg_mbps,net_in_max_mbps,net_out_avg_mbps,net_out_max_mbps
my-web-app,1.2,12.4,67.3,45.2,128,245,512,2.3,18.7,1.1,8.2
postgres-db,0.5,8.1,234.5,89.3,1228,2867,3993,45.2,312.8,12.3,156.4
```

### Markdown Report (with `--export markdown` flag)

For documentation/sharing:

```markdown
# mdok Capacity Report
Generated: 2025-01-16 14:32:00 | Duration: 2h 34m | Samples: 1,847

## my-web-app
| Metric | Min | Avg | Max | P95 |
|--------|-----|-----|-----|-----|
| CPU (%) | 1.2 | 12.4 | 67.3 | 45.2 |
| Memory (MB) | 128 | 245 | 512 | - |
| Net In (MB/s) | 0.1 | 2.3 | 18.7 | - |
| Net Out (MB/s) | 0.0 | 1.1 | 8.2 | - |

**Suggested Instance:** t3.small (2 vCPU, 2 GB)
```

---

## JSON Output Schema

Each container gets its own JSON file named `{container_name}.json` in the output directory.

```json
{
  "container_id": "abc123def456",
  "container_name": "my-web-app",
  "image": "nginx:latest",
  "monitoring_started": "2025-01-16T10:00:00Z",
  "monitoring_ended": "2025-01-16T12:34:00Z",
  "sample_count": 1847,
  "host": {
    "hostname": "dev-macbook.local",
    "os": "darwin",
    "arch": "arm64",
    "cpu_model": "Apple M2 Pro",
    "cpu_cores_physical": 10,
    "cpu_cores_logical": 10,
    "cpu_frequency_mhz": 3500,
    "memory_total_bytes": 17179869184,
    "memory_total_human": "16 GB",
    "docker_version": "24.0.7",
    "docker_api_version": "1.43",
    "notes": "ARM architecture, no hyperthreading. 1 core here ≠ 1 EC2 vCPU."
  },
  "container_limits": {
    "cpu_shares": 1024,
    "cpu_quota": 100000,
    "cpu_period": 100000,
    "cpu_limit_cores": 1.0,
    "cpu_limit_human": "1 CPU",
    "memory_limit_bytes": 536870912,
    "memory_limit_human": "512 MB",
    "memory_reservation_bytes": 268435456,
    "memory_reservation_human": "256 MB",
    "memory_swap_limit_bytes": 1073741824,
    "memory_swap_limit_human": "1 GB",
    "pids_limit": 100,
    "network_mode": "bridge",
    "has_cpu_limit": true,
    "has_memory_limit": true
  },
  "summary": {
    "duration_seconds": 9240,
    "duration_human": "2h 34m",
    "cpu": {
      "min_percent": 1.2,
      "max_percent": 67.3,
      "avg_percent": 12.4,
      "p95_percent": 45.2,
      "p99_percent": 58.1
    },
    "memory": {
      "min_bytes": 134217728,
      "max_bytes": 536870912,
      "avg_bytes": 256901120,
      "p95_bytes": 489123456,
      "min_human": "128 MB",
      "max_human": "512 MB",
      "avg_human": "245 MB",
      "p95_human": "466 MB"
    },
    "network": {
      "rx_rate_min_bytes_sec": 104857,
      "rx_rate_max_bytes_sec": 19608862,
      "rx_rate_avg_bytes_sec": 2411724,
      "rx_rate_p95_bytes_sec": 15728640,
      "rx_rate_min_human": "102 KB/s",
      "rx_rate_max_human": "18.7 MB/s",
      "rx_rate_avg_human": "2.3 MB/s",
      "rx_rate_p95_human": "15 MB/s",
      "tx_rate_min_bytes_sec": 0,
      "tx_rate_max_bytes_sec": 8598323,
      "tx_rate_avg_bytes_sec": 1153433,
      "tx_rate_p95_bytes_sec": 6291456,
      "tx_rate_min_human": "0 B/s",
      "tx_rate_max_human": "8.2 MB/s",
      "tx_rate_avg_human": "1.1 MB/s",
      "tx_rate_p95_human": "6 MB/s",
      "total_rx_bytes": 13314398208,
      "total_tx_bytes": 6227020800,
      "total_rx_human": "12.4 GB",
      "total_tx_human": "5.8 GB"
    },
    "network_cost_estimate": {
      "note": "Estimates based on AWS us-east-1 pricing. Inbound is free, outbound charged.",
      "region_assumed": "us-east-1",
      "inbound_cost_usd": 0.00,
      "outbound_cost_usd": 0.52,
      "total_cost_usd": 0.52,
      "monthly_projection": {
        "based_on_duration_hours": 2.57,
        "projected_outbound_gb": 1632.5,
        "projected_cost_usd": 146.93,
        "breakdown": "First 10 TB @ $0.09/GB"
      }
    },
    "aws_recommendation": {
      "instance_type": "t3.small",
      "vcpus": 2,
      "memory_gb": 2,
      "reasoning": "CPU peaks at 67% single-core, memory max 512MB, moderate network",
      "caveat": "Measured on Apple M2 Pro (ARM). EC2 vCPUs are hyperthreads with different perf characteristics. Recommend load testing on target instance."
    }
  },
  "samples": [
    {
      "timestamp": "2025-01-16T10:00:00Z",
      "cpu": {
        "percent": 2.5,
        "count": 4,
        "system_time": 1234567890,
        "user_time": 9876543210,
        "throttled_periods": 0,
        "throttled_time": 0
      },
      "memory": {
        "usage": 52428800,
        "limit": 536870912,
        "percent": 9.77,
        "cache": 10485760,
        "rss": 41943040,
        "swap": 0,
        "usage_human": "50 MB",
        "limit_human": "512 MB"
      },
      "network": {
        "rx_bytes": 1048576,
        "tx_bytes": 524288,
        "rx_rate": 2097152,
        "tx_rate": 1048576,
        "rx_rate_human": "2 MB/s",
        "tx_rate_human": "1 MB/s",
        "rx_packets": 1000,
        "tx_packets": 500,
        "rx_errors": 0,
        "tx_errors": 0,
        "rx_dropped": 0,
        "tx_dropped": 0
      },
      "block_io": {
        "read_bytes": 104857600,
        "write_bytes": 52428800,
        "read_ops": 1000,
        "write_ops": 500,
        "read_human": "100 MB",
        "write_human": "50 MB"
      },
      "pids": {
        "current": 10,
        "limit": 100
      },
      "status": "running",
      "health_status": "healthy"
    }
  ]
}
```

---

## Network Cost Estimation

mdok estimates AWS data transfer costs to help with budgeting.

### Pricing Model (default: us-east-1)

| Traffic Type | Cost |
|--------------|------|
| Inbound (from internet) | Free |
| Outbound (first 10 TB/mo) | $0.09/GB |
| Outbound (next 40 TB/mo) | $0.085/GB |
| Outbound (next 100 TB/mo) | $0.07/GB |
| Outbound (over 150 TB/mo) | $0.05/GB |
| Same-AZ (private IP) | Free |
| Cross-AZ | $0.01/GB each direction |

### Limitations

- Estimates are approximate (actual AWS billing may vary)
- Doesn't account for NAT Gateway costs ($0.045/GB + hourly)
- Doesn't track inter-container traffic on bridge network (local, not charged)
- Assumes all outbound goes to internet (not same-region S3, etc.)
- Pricing may be outdated — verify current rates at aws.amazon.com/ec2/pricing
- Containers with image or name containing `traefik`, `nginx`, `caddy`, `haproxy`,
  or `envoy` are treated as egress proxies, so connections to them count as
  internet traffic. Proxy detection works across all networks (not just shared ones).
- You can force proxy classification by labeling a container with `mdok.proxy=true`,
  or exclude a container from proxy detection with `mdok.proxy=false`.

---

## Export Reports

mdok can generate portable reports for sharing and archiving.

### Export Formats

| Format | Use Case | Contents |
|--------|----------|----------|
| `json` | Programmatic processing, data pipelines | Raw data with all samples and summaries |
| `csv` | Spreadsheets, quick analysis | Flattened summary stats per container |
| `markdown` | Documentation, wikis, PRs | Formatted tables and recommendations |
| `html` | Sharing with teammates, archiving | Self-contained file with interactive graphs |

### Time Filters

| Flag | Example | Description |
|------|---------|-------------|
| `--last` | `--last 1h`, `--last 24h`, `--last 7d` | Rolling window from now |
| `--from` / `--to` | `--from "2025-01-15" --to "2025-01-16"` | Specific date/time range |
| `--all` | `--all` | All data ever collected |

If no time filter is specified, defaults to `--last 24h`.

### HTML Report Contents

The HTML export is a single self-contained file (no external dependencies) that includes:

```
┌─────────────────────────────────────────────────────────────────┐
│                    mdok Report: prod-api                   │
│            Generated: 2025-01-16 14:32 | Period: 24h            │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  HOST INFORMATION                                               │
│  ─────────────────                                              │
│  Hostname: prod-server-01                                       │
│  CPU: Intel Xeon E5-2686 v4 (8 cores, x86_64)                  │
│  RAM: 32 GB                                                     │
│  Docker: 24.0.7                                                 │
│                                                                 │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  SUMMARY TABLE                                                  │
│  ┌────────────┬────────┬────────┬────────┬────────┬──────────┐ │
│  │ Container  │CPU Min │CPU Avg │CPU Max │CPU P95 │ Throttle │ │
│  ├────────────┼────────┼────────┼────────┼────────┼──────────┤ │
│  │ my-web-app │  1.2%  │ 12.4%  │ 67.3%  │ 45.2%  │    0     │ │
│  │ nginx      │  0.5%  │  2.1%  │ 15.8%  │  8.3%  │    0     │ │
│  │ redis      │  0.1%  │  0.8%  │  5.2%  │  2.1%  │    0     │ │
│  └────────────┴────────┴────────┴────────┴────────┴──────────┘ │
│  (similar tables for Memory, Network)                          │
│                                                                 │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  INTERACTIVE GRAPHS (Chart.js embedded)                         │
│  ─────────────────────────────────────                          │
│                                                                 │
│  [CPU Usage Over Time - Line Chart]                             │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │     ╭─╮                                                  │   │
│  │  ╭──╯ ╰──╮    ╭───╮        ╭─╮                          │   │
│  │ ─╯       ╰────╯   ╰────────╯ ╰──────                    │   │
│  └─────────────────────────────────────────────────────────┘   │
│  □ my-web-app  □ nginx  □ redis    [Zoom] [Pan] [Reset]        │
│                                                                 │
│  [Memory Usage Over Time - Line Chart]                          │
│  [Network I/O Over Time - Area Chart]                           │
│  [Network Totals - Bar Chart]                                   │
│                                                                 │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  AWS COST ESTIMATES                                             │
│  ───────────────────                                            │
│  Region: us-east-1                                              │
│                                                                 │
│  Data Transfer (this period):                                   │
│    Inbound:   45.2 GB  (free)                                  │
│    Outbound:  12.8 GB  ($1.15)                                 │
│                                                                 │
│  Monthly Projection (at current rate):                          │
│    Outbound:  384 GB   ($34.56/mo)                             │
│                                                                 │
│  Instance Recommendations:                                      │
│  ┌────────────┬─────────────┬────────────────────────────────┐ │
│  │ Container  │ Suggested   │ Reasoning                      │ │
│  ├────────────┼─────────────┼────────────────────────────────┤ │
│  │ my-web-app │ t3.small    │ CPU P95 45%, Memory max 512MB  │ │
│  │ nginx      │ t3.micro    │ Low resource usage             │ │
│  │ redis      │ r6g.medium  │ Memory-bound workload          │ │
│  └────────────┴─────────────┴────────────────────────────────┘ │
│                                                                 │
│  ⚠️  Measured on Intel Xeon. Graviton vCPUs may differ.        │
│                                                                 │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  WARNINGS & ALERTS                                              │
│  ─────────────────                                              │
│  ⚠️  my-web-app: Hit memory limit 3 times                       │
│  ⚠️  redis: CPU throttled for 847 periods                       │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### HTML Technical Implementation

- **Self-contained**: All CSS, JS, and data embedded in single file
- **Charting library**: Chart.js (~60KB) embedded inline
- **Interactive features**: Hover tooltips, click to toggle series, zoom/pan on time axis
- **Responsive**: Works on desktop and mobile browsers
- **No server required**: Just open the `.html` file in any browser
- **Print-friendly**: CSS media queries for clean printed output

---

## TUI Interface

### Screen 1: Container Selection

```
┌─────────────────────────────────────────────────────┐
│              mdok - Container Selection        │
├─────────────────────────────────────────────────────┤
│                                                     │
│  Select containers to monitor:                      │
│                                                     │
│  [x] my-web-app          (running)    nginx:latest │
│  [ ] redis-cache         (running)    redis:7      │
│  [x] postgres-db         (running)    postgres:15  │
│  [ ] worker-1            (exited)     python:3.11  │
│                                                     │
│  ─────────────────────────────────────────────────  │
│  Collection interval: [5] seconds                   │
│  Output directory: [./mdok-data]                │
│                                                     │
│  [Space] Toggle  [Enter] Start  [q] Quit            │
└─────────────────────────────────────────────────────┘
```

### Screen 2: Monitoring Dashboard

```
┌─────────────────────────────────────────────────────┐
│          mdok - Live Monitoring                │
│          Collecting every 5s | Samples: 42          │
├─────────────────────────────────────────────────────┤
│                                                     │
│  my-web-app (running)                               │
│  CPU: ████████░░░░░░░░░░░░  25.3%                  │
│  MEM: ██████░░░░░░░░░░░░░░  156MB / 512MB (30.5%)  │
│  NET: ↓ 1.2 MB/s  ↑ 0.8 MB/s                       │
│  I/O: R 50 MB/s  W 10 MB/s                         │
│                                                     │
│  postgres-db (running)                              │
│  CPU: ██░░░░░░░░░░░░░░░░░░   8.1%                  │
│  MEM: ████████████░░░░░░░░  320MB / 1GB (31.3%)    │
│  NET: ↓ 0.5 MB/s  ↑ 0.3 MB/s                       │
│  I/O: R 120 MB/s  W 80 MB/s                        │
│                                                     │
│  [p] Pause  [r] Resume  [s] Selection  [q] Quit     │
└─────────────────────────────────────────────────────┘
```

---

## Technical Requirements

### Language & Dependencies

- **Language**: Go 1.21+
- **Docker SDK**: `github.com/docker/docker/client` — official Docker client library
- **CLI Framework**: `github.com/spf13/cobra` — CLI argument parsing
- **TUI Library**: `github.com/charmbracelet/bubbletea` + `github.com/charmbracelet/bubbles` — interactive selection and UI
- **Styling**: `github.com/charmbracelet/lipgloss` — terminal styling

### Implementation Notes

1. **Single binary**: Compile to one executable, no runtime dependencies
2. **Docker client**: Use `client.NewClientWithOpts(client.FromEnv)` to connect to local Docker daemon
3. **Stats API**: Use `client.ContainerStats()` with `stream=false` for point-in-time metrics
4. **File handling**: Read existing JSON, unmarshal, append sample, marshal with indentation, write back
5. **Error handling**: Gracefully handle containers that stop during monitoring (check for context cancellation, closed channels)
6. **Signal handling**: Use `os/signal` to catch SIGINT/SIGTERM for clean shutdown and final summary
7. **Concurrency**: Use goroutines to collect stats from multiple containers in parallel, aggregate via channels

### Daemon Mode Implementation

1. **PID files**: One per instance at `~/.mdok/pids/<config-name>.pid`
2. **Config files**: Stored in `~/.mdok/configs/<config-name>.json`
3. **Data folders**: Each instance writes to `~/.mdok/data/<config-name>/`
4. **Log files**: Per-instance logs at `~/.mdok/logs/<config-name>.log` (rotate at 10MB)
5. **Process management**: Each `mdok start` spawns an independent daemon process
6. **Graceful shutdown**: On `stop`, send SIGTERM, wait up to 10s for summary write, then SIGKILL

### File Locations

```
~/.mdok/
├── configs/                    # Saved monitoring configurations
│   ├── prod-api.json
│   ├── web-tier.json
│   └── databases.json
├── data/                       # Monitoring data (one folder per config)
│   ├── prod-api/
│   │   ├── my-web-app.json     # Container metrics
│   │   ├── nginx-proxy.json
│   │   └── redis-cache.json
│   ├── web-tier/
│   │   └── ...
│   └── databases/
│       └── ...
├── pids/                       # PID files for running instances
│   ├── prod-api.pid
│   └── web-tier.pid
└── logs/                       # Instance logs
    ├── prod-api.log
    └── web-tier.log
```

### Config File Schema

Each config file (`~/.mdok/configs/<name>.json`) stores:

```json
{
  "name": "prod-api",
  "created": "2025-01-16T10:00:00Z",
  "updated": "2025-01-16T14:30:00Z",
  "containers": [
    "my-web-app",
    "nginx-proxy",
    "redis-cache"
  ],
  "interval_seconds": 5,
  "options": {
    "region": "us-east-1",
    "cost_estimate": true
  }
}
```

---

## Resource Overhead

mdok is designed to be lightweight. Here's what to expect:

### Memory Usage

| Scenario | RAM | Notes |
|----------|-----|-------|
| Idle (daemon running) | ~10-15 MB | Go runtime + Docker client |
| Active monitoring (5 containers) | ~20-30 MB | + sample buffers |
| Active monitoring (20 containers) | ~30-50 MB | Scales linearly |

### CPU Usage

| Activity | CPU | Notes |
|----------|-----|-------|
| Sleeping between samples | <0.1% | Just timers |
| Collecting stats (per interval) | 0.3-0.5% | Burst during collection |
| Writing JSON | <0.1% | Fast marshal + write |

### Disk I/O

| Metric | Value |
|--------|-------|
| Sample size | ~1-2 KB per container per sample |
| 1 hour @ 5s interval | ~1.5 MB per container |
| 24 hours @ 5s interval | ~35 MB per container |
| 24 hours @ 30s interval | ~6 MB per container |

### Docker API Impact

| Interval | API Calls (10 containers) | Impact |
|----------|---------------------------|--------|
| 1 second | 600/min | Negligible — Docker handles this easily |
| 5 seconds | 120/min | Trivial |
| 30 seconds | 20/min | Unnoticeable |

**Bottom line**: mdok will use less resources than a single Chrome tab. The Docker daemon itself consumes far more. You can safely run it 24/7.

### Project Structure

```
mdok/
├── main.go           # Entry point, CLI setup with cobra
├── docker.go         # Docker client, container listing, stats collection
├── monitor.go        # Monitoring loop, data aggregation
├── ui.go             # Bubbletea TUI components (selection, dashboard)
├── types.go          # Data structures (ContainerStats, Sample, etc.)
├── output.go         # JSON file read/write operations
├── go.mod
└── go.sum
```

### Command Line Interface

```bash
# ─────────────────────────────────────────────────────
# SETUP (Interactive)
# ─────────────────────────────────────────────────────

# Create new monitoring config (interactive container selection)
mdok
# → Shows container list with checkboxes
# → Prompts for interval (default: 5s)
# → Prompts for config name (e.g., "prod-api")
# → Saves to ~/.mdok/configs/prod-api.json

# Edit existing config
mdok edit prod-api

# List all saved configs
mdok configs

# Delete a config (stops instance first if running)
mdok delete prod-api

# ─────────────────────────────────────────────────────
# RUNNING INSTANCES
# ─────────────────────────────────────────────────────

# Start monitoring (runs as background daemon)
mdok start prod-api

# Start multiple instances
mdok start prod-api
mdok start web-tier
mdok start databases

# List running instances
mdok ls
# Output:
# NAME        STATUS    CONTAINERS  SAMPLES  UPTIME     
# prod-api    running   3           1,847    2h 34m     
# web-tier    running   5           892      1h 12m     
# databases   stopped   2           0        -          

# Stop an instance (graceful, writes summary)
mdok stop prod-api

# Stop all instances
mdok stop --all

# Restart an instance
mdok restart prod-api

# ─────────────────────────────────────────────────────
# VIEWING DATA
# ─────────────────────────────────────────────────────

# View live dashboard for running instance
mdok view prod-api

# View summary/stats (works for running or stopped)
mdok view prod-api --summary

# View logs
mdok logs prod-api
mdok logs prod-api --follow

# ─────────────────────────────────────────────────────
# EXPORTING REPORTS
# ─────────────────────────────────────────────────────

# Export as JSON (for processing with other tools)
mdok export prod-api --format json > report.json

# Export as self-contained HTML (with interactive graphs)
mdok export prod-api --format html > report.html

# Export as CSV (for spreadsheets)
mdok export prod-api --format csv > report.csv

# Export as Markdown (for docs/wikis)
mdok export prod-api --format markdown > report.md

# Time filters
mdok export prod-api --last 24h --format html > last-day.html
mdok export prod-api --last 7d --format json > last-week.json
mdok export prod-api --from "2025-01-15 00:00" --to "2025-01-16 00:00" --format html > jan15.html
mdok export prod-api --all --format html > full-history.html

# ─────────────────────────────────────────────────────
# OPTIONS
# ─────────────────────────────────────────────────────

# AWS region for cost estimates (default: us-east-1)
mdok start prod-api --region eu-west-1

# Disable cost estimates
mdok start prod-api --no-cost-estimate

# Help
mdok --help
mdok start --help
mdok export --help
```

### Build & Distribution

```bash
# Build for current platform
go build -o mdok .

# Cross-compile for Linux (from macOS/Windows)
GOOS=linux GOARCH=amd64 go build -o mdok-linux .

# Cross-compile for macOS ARM
GOOS=darwin GOARCH=arm64 go build -o mdok-macos .

# Cross-compile for Windows
GOOS=windows GOARCH=amd64 go build -o mdok.exe .
```

---

## Out of Scope (Future Enhancements)

- Remote Docker host monitoring
- Alerting/thresholds
- Data export to other formats (CSV, InfluxDB)
- Historical data visualization
- Container log capture
- Multi-host support

---

## Acceptance Criteria

### Setup & Configuration
1. ✅ `mdok` launches interactive container selection UI
2. ✅ Prompts for config name and saves to `~/.mdok/configs/<name>.json`
3. ✅ `mdok configs` lists all saved configurations
4. ✅ `mdok edit <name>` allows modifying existing config
5. ✅ `mdok delete <name>` removes config (stops instance first if running)

### Running Instances
6. ✅ `mdok start <name>` starts background daemon for that config
7. ✅ Multiple instances can run simultaneously (one per config)
8. ✅ `mdok ls` shows all running instances with status, uptime, sample count
9. ✅ `mdok stop <name>` gracefully stops instance and writes summary
10. ✅ `mdok stop --all` stops all running instances
11. ✅ Instances survive terminal close and user logout
12. ✅ Each instance writes data to `~/.mdok/data/<name>/`

### Viewing Data
13. ✅ `mdok view <name>` shows live dashboard for running instance
14. ✅ `mdok view <name> --summary` shows capacity planning summary
15. ✅ `mdok logs <name>` shows instance logs

### Exporting Reports
16. ✅ `mdok export <name> --format json` exports raw data
17. ✅ `mdok export <name> --format html` creates self-contained HTML with graphs
18. ✅ `mdok export <name> --format csv` exports summary as CSV
19. ✅ `mdok export <name> --format markdown` exports formatted markdown
20. ✅ Time filters work: `--last 24h`, `--from/--to`, `--all`
21. ✅ HTML report includes interactive Chart.js graphs
22. ✅ HTML report is fully self-contained (no external dependencies)

### Data & Analysis
23. ✅ Records host system info (CPU model, arch, cores, memory) in every data file
24. ✅ Records container resource limits (cpu_quota, memory_limit, etc.)
25. ✅ Calculates min/max/avg/p95 for CPU, memory, and network
26. ✅ Estimates AWS data transfer costs with monthly projections
27. ✅ Warns when containers hit resource limits (throttling, memory cap)

### Build & Distribution
28. ✅ Compiles to a single static binary
29. ✅ Cross-compiles for Linux, macOS, and Windows
30. ✅ Resource usage <50 MB RAM, <1% CPU during normal operation
