# Factory

**Jira → Code → PR**, automated with Claude Code.

Factory watches your Jira for assigned issues and automatically implements them using Claude Code, creating ready-to-review pull requests.

## How It Works

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Jira      │────▶│   Factory   │────▶│ Claude Code │────▶│  GitHub PR  │
│  Assigned   │     │   Daemon    │     │ Implements  │     │  Created    │
└─────────────┘     └─────────────┘     └─────────────┘     └─────────────┘
```

1. **Poll** - Factory checks Jira every 5 minutes for issues assigned to you
2. **Fetch** - Gets issue details (title, description, acceptance criteria)
3. **Branch** - Creates `feature/PROJ-123-short-description`
4. **Implement** - Claude Code analyzes the codebase and writes the code
5. **Commit** - Commits changes with `PROJ-123: Title` format
6. **PR** - Creates a pull request linked to the Jira issue
7. **Update** - Adds PR link as comment on Jira, transitions to "In Progress"

## Installation

```bash
go install github.com/anthropics/factory@latest
```

### Requirements

- Go 1.21+
- [Claude Code CLI](https://claude.ai/code) installed and authenticated
- Git
- Jira CLI (recommended) or Jira API token
- GitHub personal access token

## Quick Start

```bash
# 1. Configure (interactive)
factory configure

# 2. Start the daemon
factory start

# That's it! Factory now watches for assigned issues.
```

## Commands

| Command | Description |
|---------|-------------|
| `factory configure` | Interactive setup wizard |
| `factory start` | Start background daemon |
| `factory stop` | Stop the daemon |
| `factory status` | Show daemon status and processed issues |
| `factory trigger KEY` | Process a specific issue immediately |
| `factory clear [KEY]` | Clear processed issues (allows reprocessing) |
| `factory logs` | Tail daemon logs |
| `factory help` | Show help |

## Configuration

Run `factory configure` to set up interactively, or manually create `~/.factory/config.json`:

```json
{
  "jira": {
    "baseUrl": "https://company.atlassian.net",
    "email": "you@company.com",
    "apiToken": "your-jira-api-token",
    "useAcli": true
  },
  "github": {
    "token": "ghp_xxxxxxxxxxxx",
    "owner": "your-org",
    "repo": "your-repo"
  },
  "repo": {
    "cloneUrl": "https://github.com/your-org/your-repo.git",
    "localPath": "~/.factory/workspace",
    "defaultBranch": "main"
  },
  "poll": {
    "intervalMinutes": 5,
    "autoTransition": true
  }
}
```

### Jira Setup

**Option 1: Jira CLI (Recommended)**

```bash
# Install go-jira
go install github.com/go-jira/jira/cmd/jira@latest

# Configure (~/.jira.d/config.yml)
endpoint: https://company.atlassian.net
user: you@company.com
authentication-method: api-token

# Add credentials (~/.jira.d/.credentials)
you@company.com:your-api-token
```

Then set `useAcli: true` in factory config.

**Option 2: REST API**

Get an API token from: https://id.atlassian.com/manage-profile/security/api-tokens

Set `useAcli: false` and provide `baseUrl`, `email`, and `apiToken`.

### GitHub Setup

Create a personal access token with `repo` scope: https://github.com/settings/tokens

## File Locations

```
~/.factory/
├── config.json       # Your configuration
├── processed.json    # Tracks processed issues
├── workspace/        # Cloned repository
├── daemon.pid        # Daemon process ID
└── daemon.log        # Daemon logs
```

## Issue Processing

Factory processes issues that match:

- **Assigned** to you (or configured user)
- **Type** is Bug, Task, or Story
- **Status** is not Done/Closed

### What Gets Created

**Branch:** `feature/PROJ-123-short-description`

**Commit:**
```
PROJ-123: Issue title

Implemented via factory
```

**Pull Request:**
```markdown
## Summary
- **Issue**: [PROJ-123](https://jira.../PROJ-123)
- **Type**: Bug
- **Priority**: High

## Description
<from Jira>

## Acceptance Criteria
<from Jira>

## Validation
- [ ] Code builds successfully
- [ ] Tests pass
- [ ] Acceptance criteria verified

## Jira
Closes PROJ-123
```

**Jira Comment:** `PR raised: https://github.com/.../pull/42`

## Examples

### Process a Specific Issue

```bash
factory trigger PROJ-123
```

### Check Status

```bash
$ factory status

Daemon: Running (PID 12345)

Processed Issues (3):
Issue        Status     PR/Error                                 When
--------------------------------------------------------------------------------
PROJ-123     ✓          https://github.com/org/repo/pull/42     Jan 14 10:30
PROJ-124     ✓          https://github.com/org/repo/pull/43     Jan 14 11:15
PROJ-125     ✗          branch: failed to push                  Jan 14 12:00
```

### Reprocess a Failed Issue

```bash
factory clear PROJ-125
factory trigger PROJ-125
```

### View Logs

```bash
factory logs
```

## Troubleshooting

### Daemon won't start

```bash
# Check if already running
factory status

# Force stop and restart
factory stop
factory start
```

### Claude Code errors

Ensure Claude Code CLI is installed and authenticated:

```bash
claude --version
claude "hello"  # Test it works
```

### Jira connection issues

Test Jira CLI:

```bash
jira view PROJ-123
```

Or check API credentials:

```bash
curl -u email:token https://company.atlassian.net/rest/api/3/myself
```

### GitHub PR creation fails

Verify token has `repo` scope and can push to the repository.

## Security

- Config stored in `~/.factory/` with restricted permissions (0600)
- Tokens never logged
- Workspace is local only
- No data sent anywhere except Jira/GitHub APIs

## License

MIT
