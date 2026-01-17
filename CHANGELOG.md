# Changelog

All notable changes to mdok will be documented in this file.

## [Unreleased]

### Added
- Enhanced container selection UI showing:
  - Live status indicator (● green for running, ○ gray for stopped)
  - Uptime display for running containers (e.g., "3d12h", "45m", "30s")
  - Two-line display with container name and image on separate lines
  - Better visual spacing and readability
- Comprehensive documentation:
  - README.md with complete usage guide
  - CONTRIBUTING.md with developer guidelines
  - EXAMPLES.md with real-world usage examples
  - LICENSE (MIT)
  - .gitignore for clean repository

### Changed
- Container selection screen now shows status and uptime prominently
- Image names displayed on second line for cleaner layout
- Improved column alignment in selection interface

### Fixed
- PidsLimit pointer dereference bug in Docker API integration
- Compilation error preventing successful build

## [0.1.0] - Initial Implementation

### Added
- Interactive container selection with Bubbletea TUI
- Background daemon monitoring with PID management
- Comprehensive metrics collection:
  - CPU usage and throttling
  - Memory usage, limits, and cache
  - Network I/O with rate calculations
  - Block I/O with rate calculations
  - Process (PID) counts
- Statistical analysis (Min/Max/Avg/P95/P99)
- AWS cost estimation for data transfer
- Instance recommendations based on workload
- Warning detection system
- Multiple export formats:
  - JSON (raw data)
  - CSV (summary tables)
  - Markdown (documentation)
  - HTML (interactive charts with Chart.js)
- Live dashboard with progress bars
- Configuration management (create, edit, delete, list)
- Daemon control (start, stop, list, logs)
- Time-based filtering for exports
- Host system information capture
- Container resource limit tracking
- Graceful shutdown with summary generation
- Comprehensive CLI with Cobra
- Beautiful terminal UI with Lipgloss styling

### Technical
- Built with Go 1.21+
- Docker SDK integration
- Bubbletea TUI framework
- Concurrent stats collection
- JSON data persistence
- Signal handling (SIGTERM, SIGINT)
- File-based PID management
- Log rotation ready
