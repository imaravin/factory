package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type ProcessedIssue struct {
	ProcessedAt string `json:"processedAt"`
	Status      string `json:"status"`
	PRUrl       string `json:"prUrl,omitempty"`
	Error       string `json:"error,omitempty"`
}

var processed = make(map[string]ProcessedIssue)

func loadProcessed() {
	data, err := os.ReadFile(GetProcessedPath())
	if err == nil {
		json.Unmarshal(data, &processed)
	}
}

func saveProcessed() {
	data, _ := json.MarshalIndent(processed, "", "  ")
	os.WriteFile(GetProcessedPath(), data, 0644)
}

// StartDaemon starts the background daemon
func StartDaemon() error {
	// Check if already running
	if pid := GetDaemonPid(); pid > 0 {
		if isRunning(pid) {
			return fmt.Errorf("daemon already running (PID %d)", pid)
		}
	}

	// Fork into background
	if os.Getenv("FACTORY_DAEMON") != "1" {
		// Start new process in background
		cmd := exec.Command(os.Args[0], "start")
		cmd.Env = append(os.Environ(), "FACTORY_DAEMON=1")

		// Redirect output to log file
		logFile, err := os.OpenFile(GetLogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return err
		}
		cmd.Stdout = logFile
		cmd.Stderr = logFile

		if err := cmd.Start(); err != nil {
			return err
		}

		// Save PID
		os.WriteFile(GetPidPath(), []byte(strconv.Itoa(cmd.Process.Pid)), 0644)

		fmt.Printf("Daemon started (PID %d)\n", cmd.Process.Pid)
		fmt.Printf("Logs: %s\n", GetLogPath())
		return nil
	}

	// We're in the daemon process
	return runDaemon()
}

func runDaemon() error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}

	loadProcessed()

	mode := "ACLI"
	if !cfg.Jira.UseACLI {
		mode = "REST"
	}

	fmt.Printf(`
════════════════════════════════════════════════
  FACTORY DAEMON
  Mode: %s | Interval: %dm
════════════════════════════════════════════════

`, mode, cfg.Poll.IntervalMinutes)

	// Run immediately
	poll(cfg)

	// Then on interval
	ticker := time.NewTicker(time.Duration(cfg.Poll.IntervalMinutes) * time.Minute)
	for range ticker.C {
		poll(cfg)
	}

	return nil
}

func poll(cfg *Config) {
	fmt.Printf("[%s] Polling...\n", time.Now().Format("15:04:05"))

	issues, err := GetAssignedIssues(cfg)
	if err != nil {
		fmt.Printf("Error fetching issues: %v\n", err)
		return
	}

	fmt.Printf("Found %d assigned issue(s)\n", len(issues))

	// Filter new issues
	var newIssues []Issue
	for _, issue := range issues {
		if _, exists := processed[issue.Key]; !exists {
			newIssues = append(newIssues, issue)
		}
	}

	if len(newIssues) == 0 {
		fmt.Println("No new issues")
		return
	}

	keys := make([]string, len(newIssues))
	for i, issue := range newIssues {
		keys[i] = issue.Key
	}
	fmt.Printf("New: %s\n", strings.Join(keys, ", "))

	// Process each
	for _, issue := range newIssues {
		result := ProcessIssue(cfg, issue.Key)
		processed[issue.Key] = ProcessedIssue{
			ProcessedAt: time.Now().Format(time.RFC3339),
			Status:      result.Status,
			PRUrl:       result.PRUrl,
			Error:       result.Error,
		}
		saveProcessed()
	}
}

// StopDaemon stops the background daemon
func StopDaemon() error {
	pid := GetDaemonPid()
	if pid == 0 {
		return fmt.Errorf("daemon not running")
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		// Try force kill on Windows
		process.Kill()
	}

	os.Remove(GetPidPath())
	fmt.Println("Daemon stopped")
	return nil
}

// GetDaemonPid returns the daemon PID or 0 if not running
func GetDaemonPid() int {
	data, err := os.ReadFile(GetPidPath())
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return pid
}

func isRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds, so we need to check differently
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// ShowStatus shows daemon status and processed issues
func ShowStatus() {
	pid := GetDaemonPid()
	if pid > 0 && isRunning(pid) {
		fmt.Printf("Daemon: Running (PID %d)\n", pid)
	} else {
		fmt.Println("Daemon: Stopped")
	}

	loadProcessed()
	if len(processed) == 0 {
		fmt.Println("\nNo processed issues")
		return
	}

	fmt.Printf("\nProcessed Issues (%d):\n", len(processed))
	fmt.Printf("%-12s %-10s %-40s %s\n", "Issue", "Status", "PR/Error", "When")
	fmt.Println(strings.Repeat("-", 80))

	for key, info := range processed {
		status := "✓"
		if info.Status != "completed" {
			status = "✗"
		}
		detail := info.PRUrl
		if detail == "" {
			detail = info.Error
		}
		if len(detail) > 38 {
			detail = detail[:38] + "..."
		}
		t, _ := time.Parse(time.RFC3339, info.ProcessedAt)
		fmt.Printf("%-12s %-10s %-40s %s\n", key, status, detail, t.Format("Jan 02 15:04"))
	}
}

// ClearProcessed clears processed issues
func ClearProcessed(issueKey string) {
	loadProcessed()
	if issueKey == "" {
		processed = make(map[string]ProcessedIssue)
		fmt.Println("Cleared all")
	} else {
		delete(processed, issueKey)
		fmt.Printf("Cleared: %s\n", issueKey)
	}
	saveProcessed()
}

// TailLogs shows recent daemon logs
func TailLogs(lines int) error {
	cmd := exec.Command("tail", "-n", strconv.Itoa(lines), "-f", GetLogPath())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
