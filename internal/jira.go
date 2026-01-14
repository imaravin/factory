package internal

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
)

type Issue struct {
	Key                string
	Title              string
	Description        string
	Type               string
	Priority           string
	Status             string
	Labels             []string
	Components         []string
	AcceptanceCriteria string
}

func (i *Issue) IsValidType() bool {
	t := strings.ToLower(i.Type)
	return t == "bug" || t == "task" || t == "story" || t == "sub-task"
}

func (i *Issue) IsClosed() bool {
	s := strings.ToLower(i.Status)
	return s == "done" || s == "closed" || s == "resolved" || s == "cancelled"
}

// --- ACLI Implementation ---

func GetIssueACLI(issueKey string) (*Issue, error) {
	issue := &Issue{Key: issueKey}

	// Get fields using templates
	if out, err := execJira("view", issueKey, "-t", "{{.fields.summary}}"); err == nil {
		issue.Title = out
	}
	if out, err := execJira("view", issueKey, "-t", "{{.fields.description}}"); err == nil {
		issue.Description = out
		issue.AcceptanceCriteria = extractAC(out)
	}
	if out, err := execJira("view", issueKey, "-t", "{{.fields.issuetype.name}}"); err == nil {
		issue.Type = out
	}
	if out, err := execJira("view", issueKey, "-t", "{{.fields.priority.name}}"); err == nil {
		issue.Priority = out
	}
	if out, err := execJira("view", issueKey, "-t", "{{.fields.status.name}}"); err == nil {
		issue.Status = out
	}

	return issue, nil
}

func GetAssignedIssuesACLI() ([]Issue, error) {
	jql := `assignee = currentUser() AND status != Done AND status != Closed AND type in (Bug, Task, Story) ORDER BY updated DESC`

	out, err := execJira("list", "-q", jql)
	if err != nil {
		return nil, err
	}

	var issues []Issue
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) >= 1 {
			issue := Issue{Key: strings.TrimSpace(parts[0])}
			if len(parts) >= 2 {
				issue.Title = strings.TrimSpace(parts[1])
			}
			issues = append(issues, issue)
		}
	}
	return issues, nil
}

func AddCommentACLI(issueKey, comment string) error {
	_, err := execJira("comment", issueKey, "-m", comment)
	return err
}

func TransitionACLI(issueKey, status string) error {
	_, err := execJira("transition", status, issueKey)
	return err
}

func execJira(args ...string) (string, error) {
	cmd := exec.Command("jira", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// --- REST Implementation ---

func GetIssueREST(cfg *Config, issueKey string) (*Issue, error) {
	path := fmt.Sprintf("/rest/api/3/issue/%s?fields=summary,description,issuetype,priority,status,labels,components", issueKey)
	body, err := jiraRequest(cfg, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var data struct {
		Key    string `json:"key"`
		Fields struct {
			Summary     string `json:"summary"`
			Description struct {
				Content []struct {
					Content []struct {
						Text string `json:"text"`
					} `json:"content"`
				} `json:"content"`
			} `json:"description"`
			IssueType  struct{ Name string } `json:"issuetype"`
			Priority   struct{ Name string } `json:"priority"`
			Status     struct{ Name string } `json:"status"`
			Labels     []string              `json:"labels"`
			Components []struct{ Name string } `json:"components"`
		} `json:"fields"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	var desc []string
	for _, block := range data.Fields.Description.Content {
		for _, c := range block.Content {
			if c.Text != "" {
				desc = append(desc, c.Text)
			}
		}
	}
	description := strings.Join(desc, "\n")

	var comps []string
	for _, c := range data.Fields.Components {
		comps = append(comps, c.Name)
	}

	return &Issue{
		Key:                data.Key,
		Title:              data.Fields.Summary,
		Description:        description,
		Type:               data.Fields.IssueType.Name,
		Priority:           data.Fields.Priority.Name,
		Status:             data.Fields.Status.Name,
		Labels:             data.Fields.Labels,
		Components:         comps,
		AcceptanceCriteria: extractAC(description),
	}, nil
}

func GetAssignedIssuesREST(cfg *Config) ([]Issue, error) {
	jql := url.QueryEscape(`assignee = currentUser() AND status != Done AND status != Closed AND type in (Bug, Task, Story)`)
	path := fmt.Sprintf("/rest/api/3/search?jql=%s&fields=summary,issuetype,status&maxResults=20", jql)

	body, err := jiraRequest(cfg, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var data struct {
		Issues []struct {
			Key    string `json:"key"`
			Fields struct {
				Summary   string             `json:"summary"`
				IssueType struct{ Name string } `json:"issuetype"`
				Status    struct{ Name string } `json:"status"`
			} `json:"fields"`
		} `json:"issues"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	var issues []Issue
	for _, item := range data.Issues {
		issues = append(issues, Issue{
			Key:    item.Key,
			Title:  item.Fields.Summary,
			Type:   item.Fields.IssueType.Name,
			Status: item.Fields.Status.Name,
		})
	}
	return issues, nil
}

func AddCommentREST(cfg *Config, issueKey, comment string) error {
	path := fmt.Sprintf("/rest/api/3/issue/%s/comment", issueKey)
	body := map[string]interface{}{
		"body": map[string]interface{}{
			"type": "doc", "version": 1,
			"content": []map[string]interface{}{
				{"type": "paragraph", "content": []map[string]interface{}{
					{"type": "text", "text": comment},
				}},
			},
		},
	}
	_, err := jiraRequest(cfg, "POST", path, body)
	return err
}

func TransitionREST(cfg *Config, issueKey, status string) error {
	// Get transitions
	path := fmt.Sprintf("/rest/api/3/issue/%s/transitions", issueKey)
	body, err := jiraRequest(cfg, "GET", path, nil)
	if err != nil {
		return err
	}

	var transitions struct {
		Transitions []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"transitions"`
	}
	json.Unmarshal(body, &transitions)

	var transID string
	for _, t := range transitions.Transitions {
		if strings.EqualFold(t.Name, status) {
			transID = t.ID
			break
		}
	}

	if transID == "" {
		return nil // Transition not available
	}

	_, err = jiraRequest(cfg, "POST", path, map[string]interface{}{
		"transition": map[string]string{"id": transID},
	})
	return err
}

func jiraRequest(cfg *Config, method, path string, body interface{}) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, cfg.Jira.BaseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}

	auth := base64.StdEncoding.EncodeToString([]byte(cfg.Jira.Email + ":" + cfg.Jira.APIToken))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("jira API error %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

// --- Unified Interface ---

func GetIssue(cfg *Config, issueKey string) (*Issue, error) {
	if cfg.Jira.UseACLI {
		return GetIssueACLI(issueKey)
	}
	return GetIssueREST(cfg, issueKey)
}

func GetAssignedIssues(cfg *Config) ([]Issue, error) {
	if cfg.Jira.UseACLI {
		return GetAssignedIssuesACLI()
	}
	return GetAssignedIssuesREST(cfg)
}

func AddComment(cfg *Config, issueKey, comment string) error {
	if cfg.Jira.UseACLI {
		return AddCommentACLI(issueKey, comment)
	}
	return AddCommentREST(cfg, issueKey, comment)
}

func Transition(cfg *Config, issueKey, status string) error {
	if cfg.Jira.UseACLI {
		return TransitionACLI(issueKey, status)
	}
	return TransitionREST(cfg, issueKey, status)
}

func extractAC(desc string) string {
	re := regexp.MustCompile(`(?i)acceptance\s*criteria[:\s]*([\s\S]*?)(?:\n\n|$)`)
	if m := re.FindStringSubmatch(desc); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}
