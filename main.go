package main

import (
	"fmt"
	"os"

	"github.com/imaravin/factory/internal"
)

const version = "1.0.0"

func main() {
	if len(os.Args) < 2 {
		help()
		os.Exit(0)
	}

	cmd := os.Args[1]

	switch cmd {
	case "configure", "config":
		if err := internal.RunConfigure(); err != nil {
			fatal(err)
		}

	case "start":
		if !internal.ConfigExists() {
			fatal(fmt.Errorf("not configured. Run: factory configure"))
		}
		if err := internal.StartDaemon(); err != nil {
			fatal(err)
		}

	case "stop":
		if err := internal.StopDaemon(); err != nil {
			fatal(err)
		}

	case "status":
		internal.ShowStatus()

	case "trigger":
		if len(os.Args) < 3 {
			fatal(fmt.Errorf("usage: factory trigger <ISSUE-KEY>"))
		}
		if !internal.ConfigExists() {
			fatal(fmt.Errorf("not configured. Run: factory configure"))
		}
		cfg, err := internal.LoadConfig()
		if err != nil {
			fatal(err)
		}
		result := internal.ProcessIssue(cfg, os.Args[2])
		if result.Status != "completed" {
			os.Exit(1)
		}

	case "clear":
		key := ""
		if len(os.Args) >= 3 {
			key = os.Args[2]
		}
		internal.ClearProcessed(key)

	case "logs":
		internal.TailLogs(50)

	case "version", "-v", "--version":
		fmt.Printf("factory v%s\n", version)

	case "help", "-h", "--help":
		help()

	default:
		fmt.Printf("Unknown command: %s\n\n", cmd)
		help()
		os.Exit(1)
	}
}

func help() {
	fmt.Printf(`factory v%s

Jira to Code to PR - Automated with Claude Code

USAGE:
    factory <command>

COMMANDS:
    configure    Setup Jira, GitHub, and repository settings
    start        Start the background daemon
    stop         Stop the daemon
    status       Show daemon status and processed issues
    trigger KEY  Process a specific issue immediately
    clear [KEY]  Clear processed issues (reprocess)
    logs         Tail daemon logs
    help         Show this help

QUICK START:
    1. factory configure
    2. factory start

INSTALL:
    go install github.com/anthropics/factory@latest

`, version)
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}
