package internal

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Result struct {
	IssueKey string
	Status   string
	PRUrl    string
	Error    string
}

func ProcessIssue(cfg *Config, issueKey string) *Result {
	result := &Result{IssueKey: issueKey, Status: "started"}

	fmt.Printf("\n%s\n", strings.Repeat("=", 50))
	fmt.Printf("Processing: %s\n", issueKey)
	fmt.Printf("%s\n\n", strings.Repeat("=", 50))

	// 1. Fetch issue
	fmt.Println("→ Fetching issue...")
	issue, err := GetIssue(cfg, issueKey)
	if err != nil {
		return fail(result, "fetch", err)
	}

	if !issue.IsValidType() {
		return fail(result, "validate", fmt.Errorf("invalid type: %s", issue.Type))
	}
	if issue.IsClosed() {
		return fail(result, "validate", fmt.Errorf("issue is closed: %s", issue.Status))
	}
	fmt.Printf("  Title: %s\n", issue.Title)

	// 2. Setup git
	fmt.Println("→ Setting up git...")
	git := NewGit(cfg)
	if err := git.Init(); err != nil {
		return fail(result, "git", err)
	}

	branchName, err := git.CreateBranch(issueKey, issue.Title)
	if err != nil {
		return fail(result, "branch", err)
	}
	fmt.Printf("  Branch: %s\n", branchName)

	// 3. Run Claude Code
	fmt.Println("→ Running Claude Code...")
	if err := runClaude(git.Path(), issue); err != nil {
		return fail(result, "claude", err)
	}

	// 4. Commit & Push
	if git.HasChanges() {
		fmt.Println("→ Committing changes...")
		msg := fmt.Sprintf("%s: %s\n\nImplemented via factory", issueKey, issue.Title)
		if err := git.CommitAndPush(branchName, msg); err != nil {
			return fail(result, "push", err)
		}

		// 5. Create PR
		fmt.Println("→ Creating PR...")
		prTitle := fmt.Sprintf("[%s] %s", issueKey, issue.Title)
		prBody := FormatPRBody(issue, cfg.Jira.BaseURL)
		prURL, err := CreatePR(cfg, prTitle, prBody, branchName, cfg.Repo.DefaultBranch)
		if err != nil {
			return fail(result, "pr", err)
		}
		result.PRUrl = prURL
		fmt.Printf("  PR: %s\n", prURL)

		// 6. Update Jira
		fmt.Println("→ Updating Jira...")
		AddComment(cfg, issueKey, fmt.Sprintf("PR raised: %s", prURL))
		if cfg.Poll.AutoTransition {
			Transition(cfg, issueKey, "In Progress")
		}
	} else {
		fmt.Println("  No changes detected")
	}

	result.Status = "completed"
	fmt.Printf("\n✓ Completed: %s\n", issueKey)
	return result
}

func fail(result *Result, stage string, err error) *Result {
	result.Status = "failed"
	result.Error = fmt.Sprintf("%s: %v", stage, err)
	fmt.Printf("\n✗ Failed at %s: %v\n", stage, err)
	return result
}

func formatComments(comments []Comment) string {
	if len(comments) == 0 {
		return "No comments"
	}
	var parts []string
	for _, c := range comments {
		parts = append(parts, fmt.Sprintf("**%s** (%s):\n%s", c.Author, c.Date, c.Body))
	}
	return strings.Join(parts, "\n\n---\n\n")
}

func runClaude(repoPath string, issue *Issue) error {
	prompt := fmt.Sprintf(`Implement the following Jira issue:

## %s: %s

**Type**: %s | **Priority**: %s

## Description
%s

## Acceptance Criteria
%s

## Comments (Additional Context/Instructions)
%s

## Instructions
1. Analyze the codebase
2. Review the comments above for additional context or specific instructions
3. Implement the required changes
4. Add/update tests if needed
5. Keep changes minimal and focused
6. Add TODO comments for ambiguous parts`,
		issue.Key, issue.Title,
		issue.Type, issue.Priority,
		issue.Description,
		issue.AcceptanceCriteria,
		formatComments(issue.Comments))

	cmd := exec.Command("claude",
		"-p", prompt,
		"--allowedTools", "Read,Glob,Grep,Edit,Write,Bash",
		"--dangerously-skip-permissions",
	)
	cmd.Dir = repoPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	done := make(chan error)
	go func() { done <- cmd.Run() }()

	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Minute):
		cmd.Process.Kill()
		return fmt.Errorf("timeout after 10 minutes")
	}
}
