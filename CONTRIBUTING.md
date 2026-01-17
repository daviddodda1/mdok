# Contributing to mdok

Thank you for considering contributing to mdok! This document provides guidelines and instructions for contributing.

## Code of Conduct

Be respectful, inclusive, and constructive. We're all here to make monitoring Docker containers better.

## How to Contribute

### Reporting Bugs

Before creating a bug report:
1. Check if the bug has already been reported
2. Ensure you're using the latest version
3. Verify Docker is running and accessible

When reporting bugs, include:
- mdok version (`mdok --version`)
- Docker version (`docker version`)
- Operating system and version
- Steps to reproduce
- Expected vs actual behavior
- Relevant logs or error messages

### Suggesting Features

Feature requests are welcome! Please:
1. Check if it's already been suggested
2. Explain the use case clearly
3. Describe the expected behavior
4. Consider backward compatibility

### Pull Requests

1. **Fork the repository** and create a branch from `main`
2. **Make your changes** with clear, focused commits
3. **Test your changes** thoroughly
4. **Update documentation** if needed
5. **Submit a pull request** with a clear description

#### Development Setup

```bash
# Clone your fork
git clone https://github.com/yourusername/mdok.git
cd mdok

# Install dependencies
go mod download

# Build
go build -o mdok .

# Test
go test ./...

# Run
./mdok
```

#### Code Style

- Follow standard Go conventions
- Run `go fmt` before committing
- Use meaningful variable and function names
- Add comments for complex logic
- Keep functions focused and small

#### Commit Messages

- Use present tense ("Add feature" not "Added feature")
- Use imperative mood ("Move cursor" not "Moves cursor")
- Keep first line under 72 characters
- Reference issues/PRs when applicable

Examples:
```
Fix memory leak in stats collection

Add support for remote Docker hosts

Update README with new export formats
```

## Project Structure

```
mdok/
├── main.go           # CLI entry point and command definitions
├── docker.go         # Docker client and stats collection
├── monitor.go        # Monitoring loop and data collection
├── daemon.go         # Background daemon management
├── output.go         # File I/O and data persistence
├── stats.go          # Statistical analysis and recommendations
├── ui.go             # TUI components (Bubbletea)
├── export.go         # Export functionality (JSON, CSV, etc.)
├── types.go          # Data structures
└── spec.md           # Product specification
```

## Areas for Contribution

### High Priority

- [ ] Test coverage (unit and integration tests)
- [ ] Remote Docker host support
- [ ] Prometheus metrics endpoint
- [ ] Alert system with configurable thresholds

### Medium Priority

- [ ] More export formats (Grafana JSON, InfluxDB line protocol)
- [ ] Container log capture integration
- [ ] Multi-host monitoring dashboard
- [ ] Historical data comparison

### Nice to Have

- [ ] Web UI for viewing reports
- [ ] Kubernetes pod monitoring
- [ ] Custom metric plugins
- [ ] Report scheduling and email delivery

## Testing

Currently, mdok lacks comprehensive test coverage. Contributions to testing are highly valuable!

### Manual Testing Checklist

- [ ] Create configuration with multiple containers
- [ ] Start daemon and verify PID file creation
- [ ] Monitor for extended period (1+ hour)
- [ ] Stop daemon gracefully
- [ ] Verify summary statistics are accurate
- [ ] Export to all formats
- [ ] Test with containers that stop during monitoring
- [ ] Test with high-load containers

### Adding Tests

```go
// Example test structure
package main

import "testing"

func TestCalculateStats(t *testing.T) {
    samples := []Sample{
        {CPUPercent: 10.0},
        {CPUPercent: 20.0},
        {CPUPercent: 30.0},
    }

    stats := calculateStats(extractCPU(samples))

    if stats.Avg != 20.0 {
        t.Errorf("Expected avg 20.0, got %.1f", stats.Avg)
    }
}
```

## Documentation

When adding features:
- Update README.md with usage examples
- Add inline code comments for complex logic
- Update spec.md if changing core functionality
- Add examples to CONTRIBUTING.md if relevant

## Questions?

Feel free to:
- Open an issue for discussion
- Ask questions in pull request comments
- Reach out to maintainers

## Recognition

Contributors will be:
- Listed in the README
- Mentioned in release notes
- Thanked profusely! 🎉

---

Thank you for helping make mdok better!
