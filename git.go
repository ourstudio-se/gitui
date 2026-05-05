package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func runGitLog(repo string, maxCommits int) tea.Cmd {
	return func() tea.Msg {
		return customGitLog(repo, maxCommits)
	}
}

func customGitLog(repo string, maxCommits int) tea.Msg {
	display := "%C(yellow)%h%Creset%C(auto)%d%Creset %s %C(8)· %cr · %an%Creset"
	// %D = ref names without parens (plain text). 4 fields total.
	format := "%H" + fieldSep + "%P" + fieldSep + "%D" + fieldSep + display + recordSep
	args := []string{
		"-C", repo,
		"log", "--all", "--topo-order", "--decorate=short", "--color=always",
		fmt.Sprintf("--max-count=%d", maxCommits),
		"--pretty=format:" + format,
	}
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return logMsg{err: gitErr(err)}
	}
	headOut, _ := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	headSha := strings.TrimSpace(string(headOut))
	commits := parseCommits(string(out))
	// pipelines empty initially; rebuilt later when pipelinesMsg arrives.
	content, commitRow := buildContent(commits, headSha, nil, 0)
	return logMsg{content: content, commits: commits, commitRow: commitRow, headSha: headSha}
}

func gitErr(err error) error {
	if ee, ok := err.(*exec.ExitError); ok {
		return fmt.Errorf("git: %s", strings.TrimSpace(string(ee.Stderr)))
	}
	return err
}

func parseCommits(raw string) []Commit {
	records := strings.Split(raw, recordSep)
	commits := make([]Commit, 0, len(records))
	for _, r := range records {
		r = strings.TrimLeft(r, "\n\r")
		if r == "" {
			continue
		}
		f := strings.SplitN(r, fieldSep, 4)
		if len(f) < 4 {
			continue
		}
		var parents []string
		if pf := strings.TrimSpace(f[1]); pf != "" {
			parents = strings.Fields(pf)
		}
		var local, remote, tags []string
		isHead := false
		for p := range strings.SplitSeq(f[2], ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			switch {
			case strings.HasPrefix(p, "HEAD -> "):
				isHead = true
				local = append(local, strings.TrimPrefix(p, "HEAD -> "))
			case p == "HEAD":
				isHead = true
			case strings.HasPrefix(p, "tag: "):
				tags = append(tags, strings.TrimPrefix(p, "tag: "))
			case strings.Contains(p, "/"):
				remote = append(remote, p)
			default:
				local = append(local, p)
			}
		}
		commits = append(commits, Commit{
			Hash:           f[0],
			Parents:        parents,
			Display:        f[3],
			LocalBranches:  local,
			RemoteBranches: remote,
			Tags:           tags,
			IsHead:         isHead,
		})
	}
	return commits
}

func runGitBranch(repo string) tea.Cmd {
	return func() tea.Msg {
		out, err := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD").Output()
		if err != nil {
			return branchMsg{name: "?"}
		}
		return branchMsg{name: strings.TrimSpace(string(out))}
	}
}

func runGitStatus(repo string) tea.Cmd {
	return func() tea.Msg {
		return statusMsg{
			dirty:     gitStatusDirty(repo),
			conflicts: hasMergeConflicts(repo),
		}
	}
}

func gitStatusDirty(repo string) bool {
	out, err := exec.Command("git", "-C", repo, "status", "--porcelain").Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0
}

func runGitCheckout(repo, target string) tea.Cmd {
	return func() tea.Msg {
		out, err := exec.Command("git", "-C", repo, "checkout", target).CombinedOutput()
		text := strings.TrimSpace(string(out))
		if err != nil {
			if text == "" {
				text = err.Error()
			}
			return checkoutMsg{target: target, err: fmt.Errorf("%s", text)}
		}
		return checkoutMsg{target: target, out: text}
	}
}

func detectMainBranch(repo string) string {
	out, err := exec.Command("git", "-C", repo, "symbolic-ref", "refs/remotes/origin/HEAD").Output()
	if err == nil {
		s := strings.TrimSpace(string(out))
		if i := strings.LastIndex(s, "/"); i >= 0 {
			return s[i+1:]
		}
	}
	for _, name := range []string{"main", "master"} {
		if exec.Command("git", "-C", repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+name).Run() == nil {
			return name
		}
	}
	return ""
}

func runDetectMainBranch(repo string) tea.Cmd {
	return func() tea.Msg {
		return mainBranchMsg{name: detectMainBranch(repo)}
	}
}

func runRebase(repo, base string) tea.Cmd {
	return func() tea.Msg {
		// refresh remote-tracking refs so origin/<base> reflects upstream HEAD
		_ = exec.Command("git", "-C", repo, "fetch", "origin").Run()
		target := "origin/" + base
		out, err := exec.Command("git", "-C", repo, "rebase", target).CombinedOutput()
		text := strings.TrimSpace(string(out))
		if err != nil {
			if text == "" {
				text = err.Error()
			}
			return rebaseMsg{base: target, out: text, err: fmt.Errorf("%s", text)}
		}
		return rebaseMsg{base: target, out: text}
	}
}

func hasMergeConflicts(repo string) bool {
	out, _ := exec.Command("git", "-C", repo, "diff", "--name-only", "--diff-filter=U").Output()
	return len(strings.TrimSpace(string(out))) > 0
}

func runGitFetch(repo string) tea.Cmd {
	return func() tea.Msg {
		err := exec.Command("git", "-C", repo, "fetch", "origin", "--prune").Run()
		return fetchMsg{ts: time.Now(), err: err}
	}
}

func runPush(repo string, withLease bool) tea.Cmd {
	return func() tea.Msg {
		args := []string{"-C", repo, "push"}
		if withLease {
			args = append(args, "--force-with-lease")
		}
		out, err := exec.Command("git", args...).CombinedOutput()
		text := strings.TrimSpace(string(out))
		if err != nil {
			if text == "" {
				text = err.Error()
			}
			return pushMsg{withLease: withLease, out: text, err: fmt.Errorf("%s", text)}
		}
		return pushMsg{withLease: withLease, out: text}
	}
}

func runCreatePR(repo, base string) tea.Cmd {
	return func() tea.Msg {
		c := exec.Command("gh", "pr", "create", "--base", base, "--fill")
		c.Dir = repo
		out, err := c.CombinedOutput()
		text := strings.TrimSpace(string(out))
		url := ""
		for line := range strings.SplitSeq(text, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "https://") {
				url = line
			}
		}
		if err != nil {
			if text == "" {
				text = err.Error()
			}
			return prMsg{base: base, out: text, err: fmt.Errorf("%s", text)}
		}
		return prMsg{base: base, out: text, url: url}
	}
}

func runGitMerge(repo, target string) tea.Cmd {
	return func() tea.Msg {
		out, err := exec.Command("git", "-C", repo, "merge", target).CombinedOutput()
		text := strings.TrimSpace(string(out))
		if err != nil {
			if text == "" {
				text = err.Error()
			}
			return mergeMsg{target: target, err: fmt.Errorf("%s", text)}
		}
		return mergeMsg{target: target, out: text}
	}
}

func pickCheckoutTarget(c Commit, currentBranch string) string {
	for _, b := range c.LocalBranches {
		if b != currentBranch {
			return b
		}
	}
	if len(c.LocalBranches) > 0 {
		return c.LocalBranches[0]
	}
	// fall back: short name from remote ref. `git checkout` auto-creates a
	// local tracking branch if exactly one remote matches.
	for _, b := range c.RemoteBranches {
		_, short, ok := strings.Cut(b, "/")
		if !ok {
			continue
		}
		if short == "" || short == "HEAD" || short == currentBranch {
			continue
		}
		return short
	}
	return ""
}

func runGitAheadBehind(repo string) tea.Cmd {
	return func() tea.Msg {
		out, err := exec.Command("git", "-C", repo,
			"rev-list", "--left-right", "--count", "HEAD...@{upstream}").Output()
		if err != nil {
			return aheadBehindMsg{}
		}
		parts := strings.Fields(strings.TrimSpace(string(out)))
		if len(parts) != 2 {
			return aheadBehindMsg{}
		}
		ahead, _ := strconv.Atoi(parts[0])
		behind, _ := strconv.Atoi(parts[1])
		return aheadBehindMsg{ahead: ahead, behind: behind, valid: true}
	}
}
