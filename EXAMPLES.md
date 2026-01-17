# mdok Examples

## Container Selection Screen

When you run `mdok` to create a new configuration, you'll see an improved interface that shows:
- Container name
- Running status (green ● for running, gray ○ for stopped)
- Uptime for running containers
- Image name on a second line

### Example Screen

```
Select containers to monitor

  [ ] coolify-proxy                  ● running (3d)
      traefik:v3.6

  [ ] mss8csg04cowckgck8wcgwgg-11... ● running (3d)
      mss8csg04cowckgck8wcgwgg:1284d50d291fd63b4257bc8fb201366dd999a22e

  [x] coolify                        ● running (6d)
      ghcr.io/coollabsio/coolify:4.0.0-beta.460

  [x] coolify-db                     ● running (6d)
      postgres:15-alpine

  [ ] coolify-redis                  ● running (6d)
      redis:7-alpine

  [ ] old-container                  ○ stopped
      nginx:latest

space: toggle | a: select all | enter: continue | q: quit
```

### Uptime Format

The uptime is displayed in a human-readable format:
- **< 1 minute**: `30s` (seconds)
- **< 1 hour**: `45m` (minutes)
- **< 1 day**: `2h30m` (hours and minutes)
- **< 1 week**: `3d12h` (days and hours)
- **≥ 1 week**: `15d` (days only)

## Live Dashboard Example

```
mdok - prod-api
Last update: 18:23:45 | Interval: 5s

coolify
  CPU:    [████████░░░░░░░░░░░░░░░░░░░░]  25.3%
  Memory: [██████░░░░░░░░░░░░░░░░░░░░░░]  30.5% (245MB)
  Network: rx=1.2MB/s tx=800KB/s (total: rx=12.4GB tx=5.8GB)
  Block:   read=50MB/s write=10MB/s
  PIDs:    24

coolify-db
  CPU:    [██░░░░░░░░░░░░░░░░░░░░░░░░░░]   8.1%
  Memory: [████████████░░░░░░░░░░░░░░░░]  48.2% (1.9GB)
  Network: rx=500KB/s tx=300KB/s (total: rx=8.2GB tx=3.1GB)
  Block:   read=120MB/s write=80MB/s
  PIDs:    45

p: pause | q: quit
```

## Summary Report Example

After stopping monitoring with `mdok stop prod-api`, you'll see:

```
=== Summary for 'prod-api' ===

Container: coolify (abc123def456)
Image: ghcr.io/coollabsio/coolify:4.0.0-beta.460
Duration: 2h30m15s
Samples: 1803

  CPU:    min=5.2% avg=18.4% max=67.3% p95=45.2%
  Memory: min=180MB avg=245MB max=380MB p95=350MB
  Network: rx=12.4GB tx=5.8GB
  Block I/O: read=1.2GB write=450MB

  Warnings:
    - CPU usage P95 above 40% - consider more CPU resources

  AWS Network Cost Estimate:
    Egress: 5.80 GB @ $0.090/GB = $0.52

  Instance Recommendation: t3.small (2 vCPU, 2.0 GB RAM)
    Reason: CPU-bound workload (P95: 45.2%)

Container: coolify-db (def456789abc)
Image: postgres:15-alpine
Duration: 2h30m15s
Samples: 1803

  CPU:    min=2.1% avg=12.1% max=45.8% p95=32.5%
  Memory: min=1.2GB avg=1.9GB max=2.4GB p95=2.2GB
  Network: rx=8.2GB tx=3.1GB
  Block I/O: read=24.5GB write=18.2GB

  Warnings:
    - High block I/O activity detected

  AWS Network Cost Estimate:
    Egress: 3.10 GB @ $0.090/GB = $0.28

  Instance Recommendation: r6g.large (2 vCPU, 16.0 GB RAM)
    Reason: Memory-bound workload (P95: 2.2 GB)
```

## Export Examples

### JSON Export

```bash
mdok export prod-api --format json --output report.json
```

Creates a structured JSON file with all samples and summaries.

### CSV Export

```bash
mdok export prod-api --format csv --output report.csv
```

Creates a spreadsheet-friendly CSV:

```csv
Container,Samples,Duration,CPU Min%,CPU Avg%,CPU Max%,CPU P95%,Mem Min,Mem Avg,Mem Max,Mem P95
coolify,1803,2h30m15s,5.20,18.40,67.30,45.20,180MB,245MB,380MB,350MB
coolify-db,1803,2h30m15s,2.10,12.10,45.80,32.50,1.2GB,1.9GB,2.4GB,2.2GB
```

### Markdown Export

```bash
mdok export prod-api --format markdown --output report.md
```

Creates documentation-ready Markdown with tables.

### HTML Export

```bash
mdok export prod-api --format html --output report.html
```

Creates a self-contained HTML file with:
- Interactive Chart.js graphs
- Metric cards with key values
- Summary tables
- Warning highlights
- Cost estimates
- No external dependencies - works offline

## Real-World Workflows

### Capacity Planning

```bash
# Monitor production workload for 24 hours
mdok start prod-api

# Check in periodically
mdok view prod-api

# After 24 hours, stop and generate report
mdok stop prod-api
mdok export prod-api --last 24h --format html --output capacity-report.html
```

### Cost Estimation

```bash
# Monitor for a week to get accurate cost projections
mdok start web-tier
sleep $((7 * 24 * 3600))
mdok stop web-tier

# Export and review network costs
mdok export web-tier --all --format markdown
```

### Performance Debugging

```bash
# Start monitoring during load test
mdok start load-test

# Run your load test
./run-load-test.sh

# Stop and analyze results
mdok stop load-test
mdok view load-test  # Shows summary with warnings
```

### Multiple Configurations

```bash
# Monitor different groups independently
mdok start frontend-services   # Web tier
mdok start backend-services    # API services
mdok start data-services       # Databases

# List all running monitors
mdok ls

# View specific dashboard
mdok view backend-services
```

## Tips and Tricks

### Optimal Monitoring Intervals

- **Development**: 30s - 60s (low overhead)
- **Staging**: 10s - 30s (good balance)
- **Production**: 5s - 10s (detailed insights)
- **Load Testing**: 1s - 5s (catch spikes)

### Managing Data Growth

```bash
# Export and archive old data periodically
mdok export prod-api --from "2025-01-01" --to "2025-01-31" --output jan-2025.json

# Delete old configs after archiving
mdok delete prod-api --force
```

### Comparing Before/After

```bash
# Before optimization
mdok start app-before
sleep 3600
mdok stop app-before

# After optimization
mdok start app-after
sleep 3600
mdok stop app-after

# Compare exports
mdok export app-before --format csv
mdok export app-after --format csv
```

### Quick Health Check

```bash
# Create a config for critical services
mdok  # Select critical containers, name it "health-check"

# Run for 5 minutes
mdok start health-check
sleep 300
mdok stop health-check

# Review warnings
mdok view health-check
```
