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
	"time"
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

// filterToCurrentSession filters container data to only include the most recent monitoring session
func filterToCurrentSession(data *ContainerData) *ContainerData {
	if data == nil || len(data.Samples) == 0 {
		return data
	}

	// Find the most recent session by looking for gaps in timestamps
	// A gap > 2x the interval indicates monitoring was stopped and restarted
	maxGap := time.Duration(data.Interval*2+5) * time.Second

	sessionStartIdx := 0
	for i := len(data.Samples) - 1; i > 0; i-- {
		gap := data.Samples[i].Timestamp.Sub(data.Samples[i-1].Timestamp)
		if gap > maxGap {
			// Found a gap, so the current session starts at index i
			sessionStartIdx = i
			break
		}
	}

	// If we found a session boundary, filter the samples
	if sessionStartIdx > 0 {
		filtered := &ContainerData{
			ContainerID:   data.ContainerID,
			ContainerName: data.ContainerName,
			ImageName:     data.ImageName,
			Host:          data.Host,
			Limits:        data.Limits,
			StartTime:     data.Samples[sessionStartIdx].Timestamp,
			EndTime:       data.EndTime,
			Interval:      data.Interval,
			Samples:       data.Samples[sessionStartIdx:],
		}

		// Recalculate summary for the current session only
		filtered.Summary = CalculateSummary(filtered.Samples)
		if filtered.Summary != nil {
			filtered.Summary.Warnings = DetectWarnings(filtered)
			if filtered.EndTime.IsZero() {
				filtered.EndTime = time.Now()
			}
			if !filtered.StartTime.IsZero() && !filtered.EndTime.IsZero() {
				filtered.Summary.Duration = filtered.EndTime.Sub(filtered.StartTime).Round(time.Second).String()
			}
		}

		// Recalculate network cost for current session
		if filtered.Summary != nil {
			filtered.NetworkCost = CalculateNetworkCost(filtered.Summary.NetTxTotal)
		}

		return filtered
	}

	return data
}

// GetAllSessions returns all unique sessions from a configuration's data
func GetAllSessions(configName string) ([]SessionInfo, error) {
	dataDir := GetDataDir(configName)
	files, err := filepath.Glob(filepath.Join(dataDir, "*.json"))
	if err != nil || len(files) == 0 {
		return nil, fmt.Errorf("no monitoring data found")
	}

	// Map to track unique sessions
	sessionsMap := make(map[string]*SessionInfo)

	for _, file := range files {
		data, err := LoadContainerData(file)
		if err != nil {
			continue
		}

		if len(data.Samples) == 0 {
			continue
		}

		// Group samples by session ID
		// If no session ID, use start time gaps to identify sessions
		if data.SessionID != "" {
			// Use explicit session ID
			if session, exists := sessionsMap[data.SessionID]; exists {
				// Update session info
				session.Containers = appendUnique(session.Containers, data.ContainerName)
				if data.StartTime.Before(session.StartTime) {
					session.StartTime = data.StartTime
				}
				if data.EndTime.After(session.EndTime) {
					session.EndTime = data.EndTime
				}
				session.SampleCount += len(data.Samples)
			} else {
				// New session
				sessionsMap[data.SessionID] = &SessionInfo{
					SessionID:   data.SessionID,
					ConfigName:  configName,
					StartTime:   data.StartTime,
					EndTime:     data.EndTime,
					SampleCount: len(data.Samples),
					Containers:  []string{data.ContainerName},
				}
			}
		} else {
			// Legacy data without session IDs - create sessions based on time gaps
			sessions := splitIntoSessions(data)
			for _, sess := range sessions {
				sessionID := fmt.Sprintf("%d", sess.StartTime.Unix())
				if existing, exists := sessionsMap[sessionID]; exists {
					existing.Containers = appendUnique(existing.Containers, data.ContainerName)
					existing.SampleCount += sess.SampleCount
				} else {
					sessionsMap[sessionID] = &SessionInfo{
						SessionID:   sessionID,
						ConfigName:  configName,
						StartTime:   sess.StartTime,
						EndTime:     sess.EndTime,
						SampleCount: sess.SampleCount,
						Containers:  []string{data.ContainerName},
					}
				}
			}
		}
	}

	// Convert map to slice and sort by start time (newest first)
	sessions := make([]SessionInfo, 0, len(sessionsMap))
	for _, session := range sessionsMap {
		sessions = append(sessions, *session)
	}

	// Sort by start time descending (newest first)
	for i := 0; i < len(sessions)-1; i++ {
		for j := i + 1; j < len(sessions); j++ {
			if sessions[i].StartTime.Before(sessions[j].StartTime) {
				sessions[i], sessions[j] = sessions[j], sessions[i]
			}
		}
	}

	return sessions, nil
}

// splitIntoSessions splits container data into sessions based on time gaps
func splitIntoSessions(data *ContainerData) []SessionInfo {
	if len(data.Samples) == 0 {
		return nil
	}

	maxGap := time.Duration(data.Interval*2+5) * time.Second
	var sessions []SessionInfo
	sessionStart := 0

	for i := 1; i < len(data.Samples); i++ {
		gap := data.Samples[i].Timestamp.Sub(data.Samples[i-1].Timestamp)
		if gap > maxGap {
			// Found a gap - save previous session
			sessions = append(sessions, SessionInfo{
				SessionID:   fmt.Sprintf("%d", data.Samples[sessionStart].Timestamp.Unix()),
				ConfigName:  "",
				StartTime:   data.Samples[sessionStart].Timestamp,
				EndTime:     data.Samples[i-1].Timestamp,
				SampleCount: i - sessionStart,
			})
			sessionStart = i
		}
	}

	// Add final session
	sessions = append(sessions, SessionInfo{
		SessionID:   fmt.Sprintf("%d", data.Samples[sessionStart].Timestamp.Unix()),
		ConfigName:  "",
		StartTime:   data.Samples[sessionStart].Timestamp,
		EndTime:     data.Samples[len(data.Samples)-1].Timestamp,
		SampleCount: len(data.Samples) - sessionStart,
	})

	return sessions
}

// appendUnique appends a string to a slice if it's not already present
func appendUnique(slice []string, item string) []string {
	for _, existing := range slice {
		if existing == item {
			return slice
		}
	}
	return append(slice, item)
}

// filterToSession filters container data to only include samples from a specific session
func filterToSession(data *ContainerData, sessionID string) *ContainerData {
	if data == nil || len(data.Samples) == 0 {
		return data
	}

	// If data has session ID, simple comparison
	if data.SessionID != "" {
		if data.SessionID == sessionID {
			return data
		}
		// No match, return empty
		return &ContainerData{
			ContainerID:   data.ContainerID,
			ContainerName: data.ContainerName,
			ImageName:     data.ImageName,
			Host:          data.Host,
			Limits:        data.Limits,
			SessionID:     data.SessionID,
			Interval:      data.Interval,
			Samples:       []Sample{},
		}
	}

	// Legacy data - filter by timestamp
	sessions := splitIntoSessions(data)
	for _, sess := range sessions {
		if sess.SessionID == sessionID {
			// Find sample indices for this session
			var filtered []Sample
			for _, sample := range data.Samples {
				if !sample.Timestamp.Before(sess.StartTime) && !sample.Timestamp.After(sess.EndTime) {
					filtered = append(filtered, sample)
				}
			}

			result := &ContainerData{
				ContainerID:   data.ContainerID,
				ContainerName: data.ContainerName,
				ImageName:     data.ImageName,
				Host:          data.Host,
				Limits:        data.Limits,
				SessionID:     sessionID,
				StartTime:     sess.StartTime,
				EndTime:       sess.EndTime,
				Interval:      data.Interval,
				Samples:       filtered,
			}

			// Recalculate summary for the session
			if len(result.Samples) > 0 {
				result.Summary = CalculateSummary(result.Samples)
				if result.Summary != nil {
					result.Summary.Warnings = DetectWarnings(result)
					result.Summary.Duration = result.EndTime.Sub(result.StartTime).Round(time.Second).String()
				}

				// Recalculate network cost
				if result.Summary != nil {
					result.NetworkCost = CalculateNetworkCost(result.Summary.NetTxTotal)
				}
			}

			return result
		}
	}

	// Session not found
	return &ContainerData{
		ContainerID:   data.ContainerID,
		ContainerName: data.ContainerName,
		ImageName:     data.ImageName,
		Host:          data.Host,
		Limits:        data.Limits,
		Interval:      data.Interval,
		Samples:       []Sample{},
	}
}
