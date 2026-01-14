package internal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Jira   JiraConfig   `json:"jira"`
	GitHub GitHubConfig `json:"github"`
	Repo   RepoConfig   `json:"repo"`
	Poll   PollConfig   `json:"poll"`
}

type JiraConfig struct {
	BaseURL  string `json:"baseUrl"`
	Email    string `json:"email"`
	APIToken string `json:"apiToken"`
	UseACLI  bool   `json:"useAcli"`
}

type GitHubConfig struct {
	Token string `json:"token"`
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
}

type RepoConfig struct {
	CloneURL      string `json:"cloneUrl"`
	LocalPath     string `json:"localPath"`
	DefaultBranch string `json:"defaultBranch"`
}

type PollConfig struct {
	IntervalMinutes int  `json:"intervalMinutes"`
	AutoTransition  bool `json:"autoTransition"`
}

var cfg *Config

func GetConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".factory")
}

func GetConfigPath() string {
	return filepath.Join(GetConfigDir(), "config.json")
}

func GetProcessedPath() string {
	return filepath.Join(GetConfigDir(), "processed.json")
}

func GetPidPath() string {
	return filepath.Join(GetConfigDir(), "daemon.pid")
}

func GetLogPath() string {
	return filepath.Join(GetConfigDir(), "daemon.log")
}

func LoadConfig() (*Config, error) {
	if cfg != nil {
		return cfg, nil
	}

	data, err := os.ReadFile(GetConfigPath())
	if err != nil {
		return nil, fmt.Errorf("config not found. Run 'factory configure' first")
	}

	cfg = &Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

func SaveConfig(c *Config) error {
	if err := os.MkdirAll(GetConfigDir(), 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(GetConfigPath(), data, 0600)
}

func ConfigExists() bool {
	_, err := os.Stat(GetConfigPath())
	return err == nil
}

// RunConfigure runs interactive configuration
func RunConfigure() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println(`
╔════════════════════════════════════════════════════════════════╗
║                 FACTORY - CONFIGURATION                        ║
╚════════════════════════════════════════════════════════════════╝
`)

	// Load existing config if any
	existing := &Config{
		Repo: RepoConfig{
			LocalPath:     filepath.Join(GetConfigDir(), "workspace"),
			DefaultBranch: "main",
		},
		Poll: PollConfig{
			IntervalMinutes: 5,
			AutoTransition:  true,
		},
	}
	if ConfigExists() {
		existing, _ = LoadConfig()
	}

	// Jira Configuration
	fmt.Println("── JIRA Configuration ──")
	fmt.Println()

	useACLI := prompt(reader, "Use Jira CLI (acli)? [Y/n]", "y")
	existing.Jira.UseACLI = strings.ToLower(useACLI) != "n"

	if !existing.Jira.UseACLI {
		existing.Jira.BaseURL = prompt(reader, "Jira URL (e.g., https://company.atlassian.net)", existing.Jira.BaseURL)
		existing.Jira.Email = prompt(reader, "Jira Email", existing.Jira.Email)
		existing.Jira.APIToken = promptSecret(reader, "Jira API Token", existing.Jira.APIToken)
	} else {
		fmt.Println("Using Jira CLI - ensure 'jira' command is configured")
		fmt.Println("  Setup: https://github.com/go-jira/jira")
	}

	// GitHub Configuration
	fmt.Println()
	fmt.Println("── GitHub Configuration ──")
	fmt.Println()

	existing.GitHub.Owner = prompt(reader, "GitHub Owner (org or username)", existing.GitHub.Owner)
	existing.GitHub.Repo = prompt(reader, "GitHub Repository name", existing.GitHub.Repo)
	existing.GitHub.Token = promptSecret(reader, "GitHub Personal Access Token", existing.GitHub.Token)

	// Repository Configuration
	fmt.Println()
	fmt.Println("── Repository Configuration ──")
	fmt.Println()

	existing.Repo.CloneURL = prompt(reader, "Repository Clone URL", existing.Repo.CloneURL)
	existing.Repo.DefaultBranch = prompt(reader, "Default Branch", existing.Repo.DefaultBranch)

	// Poll Configuration
	fmt.Println()
	fmt.Println("── Polling Configuration ──")
	fmt.Println()

	intervalStr := prompt(reader, "Poll Interval (minutes)", fmt.Sprintf("%d", existing.Poll.IntervalMinutes))
	fmt.Sscanf(intervalStr, "%d", &existing.Poll.IntervalMinutes)
	if existing.Poll.IntervalMinutes < 1 {
		existing.Poll.IntervalMinutes = 5
	}

	autoTrans := prompt(reader, "Auto-transition to 'In Progress'? [Y/n]", "y")
	existing.Poll.AutoTransition = strings.ToLower(autoTrans) != "n"

	// Save
	if err := SaveConfig(existing); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf(`
╔════════════════════════════════════════════════════════════════╗
║                  CONFIGURATION SAVED                           ║
╠════════════════════════════════════════════════════════════════╣
║  Config: %-50s  ║
║                                                                ║
║  Commands:                                                     ║
║    factory start       Start the daemon                        ║
║    factory stop        Stop the daemon                         ║
║    factory status      View status                             ║
║    factory trigger     Process an issue                        ║
╚════════════════════════════════════════════════════════════════╝
`, GetConfigPath())

	return nil
}

func prompt(reader *bufio.Reader, label, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("%s: ", label)
	}

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultVal
	}
	return input
}

func promptSecret(reader *bufio.Reader, label, defaultVal string) string {
	masked := ""
	if defaultVal != "" {
		masked = "****" + defaultVal[max(0, len(defaultVal)-4):]
	}

	if masked != "" {
		fmt.Printf("%s [%s]: ", label, masked)
	} else {
		fmt.Printf("%s: ", label)
	}

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultVal
	}
	return input
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
