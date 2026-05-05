package main

import (
	"encoding/json"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func runGitHubPipelines(repo string) tea.Cmd {
	return func() tea.Msg {
		c := exec.Command("gh", "run", "list",
			"--limit", "100",
			"--json", "headBranch,status,conclusion,url,createdAt")
		c.Dir = repo
		out, err := c.Output()
		if err != nil {
			return pipelinesMsg{}
		}
		var runs []struct {
			HeadBranch string `json:"headBranch"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			URL        string `json:"url"`
			CreatedAt  string `json:"createdAt"`
		}
		if err := json.Unmarshal(out, &runs); err != nil {
			return pipelinesMsg{}
		}
		// runs come back newest-first; first occurrence per branch wins
		m := make(map[string]pipelineStatus, len(runs))
		for _, r := range runs {
			if r.HeadBranch == "" {
				continue
			}
			if _, ok := m[r.HeadBranch]; ok {
				continue
			}
			state := r.Status
			if r.Status == "completed" && r.Conclusion != "" {
				state = r.Conclusion
			}
			m[r.HeadBranch] = pipelineStatus{state: state, url: r.URL}
		}
		return pipelinesMsg{pipelines: m}
	}
}

func isRunningState(state string) bool {
	switch state {
	case "in_progress", "queued", "requested", "waiting", "pending":
		return true
	}
	return false
}

func pipelineIcon(state string, frame int) string {
	if isRunningState(state) {
		f := spinnerFrames[((frame%len(spinnerFrames))+len(spinnerFrames))%len(spinnerFrames)]
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24")).Bold(true).Render(f)
	}
	switch state {
	case "success":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#34D399")).Bold(true).Render("✓")
	case "failure", "timed_out", "startup_failure", "action_required":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171")).Bold(true).Render("✗")
	case "cancelled":
		return lipgloss.NewStyle().Foreground(colorMuted).Render("⊘")
	case "skipped", "neutral":
		return lipgloss.NewStyle().Foreground(colorMuted).Render("⏭")
	default:
		return ""
	}
}

func pipelineForCommit(c Commit, pipelines map[string]pipelineStatus) (pipelineStatus, bool) {
	if pipelines == nil {
		return pipelineStatus{}, false
	}
	for _, b := range c.LocalBranches {
		if p, ok := pipelines[b]; ok {
			return p, true
		}
	}
	for _, b := range c.RemoteBranches {
		if _, short, ok := strings.Cut(b, "/"); ok {
			if p, ok := pipelines[short]; ok {
				return p, true
			}
		}
	}
	return pipelineStatus{}, false
}

func pipelineIconFor(c Commit, pipelines map[string]pipelineStatus, frame int) string {
	p, ok := pipelineForCommit(c, pipelines)
	if !ok {
		return ""
	}
	return pipelineIcon(p.state, frame)
}

func anyRunning(pipelines map[string]pipelineStatus) bool {
	for _, p := range pipelines {
		if isRunningState(p.state) {
			return true
		}
	}
	return false
}
