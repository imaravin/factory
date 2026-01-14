package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type Git struct {
	repoPath string
	branch   string
	cloneURL string
}

func NewGit(cfg *Config) *Git {
	path := cfg.Repo.LocalPath
	if !filepath.IsAbs(path) {
		path = filepath.Join(GetConfigDir(), path)
	}
	return &Git{
		repoPath: path,
		branch:   cfg.Repo.DefaultBranch,
		cloneURL: cfg.Repo.CloneURL,
	}
}

func (g *Git) exec(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = g.repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

func (g *Git) Init() error {
	// Create directory
	if err := os.MkdirAll(g.repoPath, 0755); err != nil {
		return err
	}

	// Clone if not exists
	if _, err := os.Stat(filepath.Join(g.repoPath, ".git")); os.IsNotExist(err) {
		fmt.Println("Cloning repository...")
		cmd := exec.Command("git", "clone", g.cloneURL, g.repoPath)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("clone failed: %s", string(out))
		}
	}

	// Configure git
	g.exec("config", "user.email", "automation@jira-automation")
	g.exec("config", "user.name", "Jira Automation")

	return nil
}

func (g *Git) Pull() error {
	if _, err := g.exec("checkout", g.branch); err != nil {
		return err
	}
	_, err := g.exec("pull", "origin", g.branch)
	return err
}

func (g *Git) CreateBranch(issueKey, title string) (string, error) {
	if err := g.Pull(); err != nil {
		return "", err
	}

	// Create branch name
	re := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	slug := re.ReplaceAllString(strings.ToLower(title), "-")
	slug = strings.Trim(slug, "-")
	if len(slug) > 40 {
		slug = slug[:40]
	}
	branchName := fmt.Sprintf("feature/%s-%s", issueKey, slug)

	// Check if exists
	out, _ := g.exec("branch", "-a")
	if strings.Contains(out, branchName) {
		g.exec("checkout", branchName)
	} else {
		g.exec("checkout", "-b", branchName)
	}

	return branchName, nil
}

func (g *Git) HasChanges() bool {
	out, _ := g.exec("status", "--porcelain")
	return out != ""
}

func (g *Git) CommitAndPush(branch, message string) error {
	if _, err := g.exec("add", "-A"); err != nil {
		return err
	}
	if _, err := g.exec("commit", "-m", message); err != nil {
		return err
	}
	if _, err := g.exec("push", "-u", "origin", branch); err != nil {
		return err
	}
	return nil
}

func (g *Git) Path() string {
	return g.repoPath
}
