package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// StartDaemon starts the monitoring daemon in the background
func StartDaemon(config Config) error {
	// Get the path to the current executable
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Create log file
	logWriter, err := CreateLogWriter(config.Name)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}

	// Start the process in foreground mode but detached
	cmd := exec.Command(executable, "start", config.Name, "--foreground")

	// Redirect stdout/stderr to log file
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter

	// Set up the process to be independent
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // Create new session
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		logWriter.Close()
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	// Write PID file
	if err := WritePidFile(config.Name, cmd.Process.Pid); err != nil {
		cmd.Process.Kill()
		logWriter.Close()
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	// Don't wait for the process - let it run independently
	go func() {
		cmd.Wait()
		logWriter.Close()
		RemovePidFile(config.Name)
	}()

	return nil
}

// StopDaemon stops a running daemon
func StopDaemon(configName string) error {
	pid, err := ReadPidFile(configName)
	if err != nil {
		return fmt.Errorf("failed to read PID file: %w", err)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		RemovePidFile(configName)
		return fmt.Errorf("process not found: %w", err)
	}

	// Send SIGTERM for graceful shutdown
	if err := process.Signal(syscall.SIGTERM); err != nil {
		// Process might already be dead
		RemovePidFile(configName)
		return nil
	}

	// Wait for process to terminate (with timeout)
	done := make(chan error, 1)
	go func() {
		_, err := process.Wait()
		done <- err
	}()

	select {
	case <-done:
		// Process terminated
	case <-time.After(10 * time.Second):
		// Force kill after timeout
		process.Signal(syscall.SIGKILL)
	}

	RemovePidFile(configName)
	return nil
}

// ListDaemons returns status of all running daemons
func ListDaemons() ([]DaemonStatus, error) {
	pidDir := filepath.Join(mdokDir, "pids")
	if _, err := os.Stat(pidDir); os.IsNotExist(err) {
		return nil, nil
	}

	files, err := filepath.Glob(filepath.Join(pidDir, "*.pid"))
	if err != nil {
		return nil, err
	}

	var statuses []DaemonStatus
	for _, file := range files {
		configName := filepath.Base(file)
		configName = configName[:len(configName)-4] // Remove .pid extension

		pid, err := ReadPidFile(configName)
		if err != nil {
			continue
		}

		// Check if process is running
		process, err := os.FindProcess(pid)
		if err != nil {
			RemovePidFile(configName)
			continue
		}

		running := process.Signal(syscall.Signal(0)) == nil
		if !running {
			RemovePidFile(configName)
			continue
		}

		// Load config for additional info
		config, err := LoadConfig(configName)
		if err != nil {
			continue
		}

		// Get process start time from /proc (Linux)
		startTime := getProcessStartTime(pid)

		statuses = append(statuses, DaemonStatus{
			ConfigName: configName,
			PID:        pid,
			StartTime:  startTime,
			Running:    running,
			Containers: config.Containers,
		})
	}

	return statuses, nil
}

// getProcessStartTime attempts to get the process start time
func getProcessStartTime(pid int) time.Time {
	// Try to get from /proc on Linux
	statFile := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(statFile)
	if err != nil {
		return time.Time{}
	}

	// Parse stat file - field 22 is starttime in clock ticks
	// This is a simplified version; for accuracy you'd parse properly
	var dummy1 int
	var dummy2 string
	var dummy3 byte
	var fields [20]int64
	_, err = fmt.Sscanf(string(data), "%d %s %c", &dummy1, &dummy2, &dummy3)
	if err != nil {
		return time.Time{}
	}

	// Just use file modification time as approximation
	info, err := os.Stat(statFile)
	if err != nil {
		return time.Time{}
	}
	_ = fields // Avoid unused variable

	return info.ModTime()
}

// RunAsDaemon is called when started with --foreground flag by daemon mode
func RunAsDaemon(config Config) error {
	// Create logger that writes to both stdout (which goes to log file) and tracks internally
	logger := log.New(os.Stdout, "", log.LstdFlags)

	logger.Printf("Daemon started for config: %s\n", config.Name)
	logger.Printf("Monitoring containers: %v\n", config.Containers)
	logger.Printf("Interval: %d seconds\n", config.Interval)

	monitor, err := NewMonitor(config, logger)
	if err != nil {
		logger.Printf("Failed to create monitor: %v\n", err)
		return err
	}

	return monitor.Run()
}
