package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Directory structure:
// ~/.mdok/
//   configs/       - configuration files
//   data/<config>/ - monitoring data files
//   pids/          - PID files for running daemons
//   logs/          - log files

// EnsureDirs creates the required directory structure
func EnsureDirs() error {
	dirs := []string{
		filepath.Join(mdokDir, "configs"),
		filepath.Join(mdokDir, "data"),
		filepath.Join(mdokDir, "pids"),
		filepath.Join(mdokDir, "logs"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}

// GetConfigDir returns the configs directory path
func GetConfigDir() string {
	return filepath.Join(mdokDir, "configs")
}

// GetDataDir returns the data directory for a config
func GetDataDir(configName string) string {
	return filepath.Join(mdokDir, "data", configName)
}

// GetPidFile returns the PID file path for a config
func GetPidFile(configName string) string {
	return filepath.Join(mdokDir, "pids", configName+".pid")
}

// GetLogFile returns the log file path for a config
func GetLogFile(configName string) string {
	return filepath.Join(mdokDir, "logs", configName+".log")
}

// GetConfigFile returns the config file path
func GetConfigFile(configName string) string {
	return filepath.Join(GetConfigDir(), configName+".json")
}

// SaveConfig saves a configuration to disk
func SaveConfig(config Config) error {
	if err := EnsureDirs(); err != nil {
		return err
	}

	configFile := GetConfigFile(config.Name)
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// LoadConfig loads a configuration from disk
func LoadConfig(configName string) (Config, error) {
	configFile := GetConfigFile(configName)
	data, err := os.ReadFile(configFile)
	if err != nil {
		return Config{}, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return Config{}, fmt.Errorf("failed to parse config: %w", err)
	}

	return config, nil
}

// ConfigExists checks if a configuration exists
func ConfigExists(configName string) bool {
	_, err := os.Stat(GetConfigFile(configName))
	return err == nil
}

// ListConfigs returns all saved configurations
func ListConfigs() ([]Config, error) {
	configDir := GetConfigDir()
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		return nil, nil
	}

	files, err := filepath.Glob(filepath.Join(configDir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to list configs: %w", err)
	}

	var configs []Config
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		var config Config
		if err := json.Unmarshal(data, &config); err != nil {
			continue
		}
		configs = append(configs, config)
	}

	return configs, nil
}

// DeleteConfig removes a configuration and its data
func DeleteConfig(configName string) error {
	// Remove config file
	configFile := GetConfigFile(configName)
	if err := os.Remove(configFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove config file: %w", err)
	}

	// Remove data directory
	dataDir := GetDataDir(configName)
	if err := os.RemoveAll(dataDir); err != nil {
		return fmt.Errorf("failed to remove data directory: %w", err)
	}

	// Remove log file
	logFile := GetLogFile(configName)
	os.Remove(logFile) // Ignore error

	// Remove PID file
	pidFile := GetPidFile(configName)
	os.Remove(pidFile) // Ignore error

	return nil
}

// SaveContainerData saves container monitoring data to disk
func SaveContainerData(configName string, data *ContainerData) error {
	dataDir := GetDataDir(configName)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Use sanitized container name for filename
	filename := sanitizeFilename(data.ContainerName) + ".json"
	filepath := filepath.Join(dataDir, filename)

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	if err := os.WriteFile(filepath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write data file: %w", err)
	}

	return nil
}

// LoadContainerData loads container data from a file
func LoadContainerData(filepath string) (*ContainerData, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read data file: %w", err)
	}

	var containerData ContainerData
	if err := json.Unmarshal(data, &containerData); err != nil {
		return nil, fmt.Errorf("failed to parse data: %w", err)
	}

	return &containerData, nil
}

// LoadAllContainerData loads all container data for a config
func LoadAllContainerData(configName string) ([]*ContainerData, error) {
	dataDir := GetDataDir(configName)
	files, err := filepath.Glob(filepath.Join(dataDir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to list data files: %w", err)
	}

	var allData []*ContainerData
	for _, file := range files {
		data, err := LoadContainerData(file)
		if err != nil {
			continue
		}
		allData = append(allData, data)
	}

	return allData, nil
}

// WritePidFile writes the PID of a daemon process
func WritePidFile(configName string, pid int) error {
	if err := EnsureDirs(); err != nil {
		return err
	}

	pidFile := GetPidFile(configName)
	return os.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0644)
}

// ReadPidFile reads the PID from a PID file
func ReadPidFile(configName string) (int, error) {
	pidFile := GetPidFile(configName)
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, err
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid PID: %w", err)
	}

	return pid, nil
}

// RemovePidFile removes the PID file
func RemovePidFile(configName string) error {
	return os.Remove(GetPidFile(configName))
}

// IsRunning checks if a daemon is running for the given config
func IsRunning(configName string) bool {
	pid, err := ReadPidFile(configName)
	if err != nil {
		return false
	}

	// Check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds, so we need to send signal 0
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// sanitizeFilename removes or replaces characters that aren't safe for filenames
func sanitizeFilename(name string) string {
	// Replace common problematic characters
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
		" ", "_",
	)
	return replacer.Replace(name)
}

// TailLines reads the last n lines from a file
func TailLines(filepath string, n int) (string, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Read all lines and return last n
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	if len(lines) <= n {
		return strings.Join(lines, "\n") + "\n", nil
	}

	return strings.Join(lines[len(lines)-n:], "\n") + "\n", nil
}

// TailFollow follows a file like tail -f
func TailFollow(filepath string) {
	cmd := exec.Command("tail", "-f", filepath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

// AppendToLog appends a message to the log file
func AppendToLog(configName string, message string) error {
	if err := EnsureDirs(); err != nil {
		return err
	}

	logFile := GetLogFile(configName)
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(message + "\n")
	return err
}

// CreateLogWriter returns a writer that appends to the log file
func CreateLogWriter(configName string) (io.WriteCloser, error) {
	if err := EnsureDirs(); err != nil {
		return nil, err
	}

	logFile := GetLogFile(configName)
	return os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
}
