package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---- helpers ----

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func spinnerTick() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(t time.Time) tea.Msg { return spinnerTickMsg(t) })
}

func openURL(url string) tea.Cmd {
	return func() tea.Msg {
		var c *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			c = exec.Command("open", url)
		case "windows":
			c = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		default:
			c = exec.Command("xdg-open", url)
		}
		if err := c.Start(); err != nil {
			return openMsg{url: url, err: err}
		}
		return openMsg{url: url}
	}
}

func runClaude(repo, prompt string) tea.Cmd {
	c := exec.Command("claude", prompt)
	c.Dir = repo
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return claudeFinishedMsg{cmd: prompt, err: err}
	})
}

func formatRelative(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < 5*time.Second:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

// ---- model actions ----

// startAction kicks off either checkout or merge for `target`. If working tree
// is dirty, switches to confirm mode (y/n) instead of running immediately.
// Returns commands to be appended to the Update batch.
func (m *model) startAction(action, target string) []tea.Cmd {
	dirty := gitStatusDirty(m.repoPath)
	if dirty {
		m.mode = "confirm"
		m.pendingTarget = target
		m.pendingAction = action
		m.dialogTitle = "Working tree dirty"
		switch action {
		case "merge":
			m.dialogBody = "uncommitted changes present.\nmerge " + target + " into " + m.branchName + " anyway?\n\ny: merge (git may abort if conflicts)\nn: cancel"
		default:
			m.dialogBody = "uncommitted changes present.\ncheckout " + target + " anyway?\n\ny: checkout (git may abort if conflicts)\nn: cancel"
		}
		return nil
	}
	m.mode = "list"
	m.pendingTarget = ""
	m.pendingAction = ""
	if action == "merge" {
		return []tea.Cmd{runGitMerge(m.repoPath, target)}
	}
	return []tea.Cmd{runGitCheckout(m.repoPath, target)}
}

func (m *model) executeAction(opt actionOption) []tea.Cmd {
	switch opt.kind {
	case "checkout":
		m.pendingTarget = opt.payload
		return m.startAction("checkout", opt.payload)
	case "merge":
		m.pendingTarget = opt.payload
		return m.startAction("merge", opt.payload)
	case "rebase":
		m.mode = "list"
		m.actionOpts = nil
		return []tea.Cmd{runRebase(m.repoPath, opt.payload)}
	case "pr":
		m.mode = "list"
		m.actionOpts = nil
		return []tea.Cmd{runCreatePR(m.repoPath, opt.payload)}
	case "open":
		m.mode = "list"
		m.actionOpts = nil
		return []tea.Cmd{openURL(opt.payload)}
	case "push":
		m.mode = "list"
		m.actionOpts = nil
		return []tea.Cmd{runPush(m.repoPath, false)}
	case "push-lease":
		m.mode = "list"
		m.actionOpts = nil
		return []tea.Cmd{runPush(m.repoPath, true)}
	}
	return nil
}

func (m *model) refreshDisplay() {
	if len(m.commitRow) == 0 {
		m.content = m.contentBase
	} else {
		m.content = applyHighlight(m.contentBase, m.commitRow, m.cursor, m.viewport.Width)
	}
	if m.ready {
		m.viewport.SetContent(m.content)
	}
}

func (m *model) ensureCursorVisible() {
	if m.cursor < 0 || m.cursor >= len(m.commitRow) {
		return
	}
	row := m.commitRow[m.cursor]
	yOff := m.viewport.YOffset
	h := m.viewport.Height
	if h <= 0 {
		return
	}
	switch {
	case row < yOff:
		m.viewport.SetYOffset(row)
	case row >= yOff+h:
		m.viewport.SetYOffset(row - h + 1)
	}
}

// ---- bubble tea ----

func (m model) Init() tea.Cmd {
	return tea.Batch(
		runGitFetch(m.repoPath),
		runGitLog(m.repoPath, m.maxCommits),
		runGitBranch(m.repoPath),
		runGitAheadBehind(m.repoPath),
		runGitHubPipelines(m.repoPath),
		runDetectMainBranch(m.repoPath),
		runGitStatus(m.repoPath),
		tick(m.refreshEvery),
		spinnerTick(),
	)
}

func (m model) branchIcon() string {
	if m.useIcons {
		return iconStyle.Render("") + " "
	}
	return iconStyle.Render("❯") + " "
}

func (m model) headerView() string {
	left := titleStyle.Render("git tree") +
		dotStyle.Render(" · ") +
		m.branchIcon() +
		branchPillStyle.Render(m.branchName)
	if m.abValid {
		var parts []string
		if m.ahead > 0 {
			parts = append(parts, aheadStyle.Render(fmt.Sprintf("↑%d", m.ahead)))
		}
		if m.behind > 0 {
			parts = append(parts, behindStyle.Render(fmt.Sprintf("↓%d", m.behind)))
		}
		if m.ahead == 0 && m.behind == 0 {
			parts = append(parts, dotStyle.Render("✓"))
		}
		if len(parts) > 0 {
			left += " " + strings.Join(parts, " ")
		}
	}
	// fetch status, right side of header
	right := ""
	switch {
	case m.fetching:
		spin := spinnerFrames[((m.spinnerFrame%len(spinnerFrames))+len(spinnerFrames))%len(spinnerFrames)]
		right = lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24")).Bold(true).Render(spin) +
			dotStyle.Render(" fetching")
	case m.fetchErr != nil:
		right = lipgloss.NewStyle().Foreground(colorErr).Render("⚠ fetch failed")
	case !m.lastFetch.IsZero():
		right = dotStyle.Render("↻ " + formatRelative(m.lastFetch))
	}
	gap := max(1, m.width-lipgloss.Width(left)-lipgloss.Width(right))
	titleLine := left
	if right != "" {
		titleLine += strings.Repeat(" ", gap) + right
	}
	rule := ruleStyle.Render(strings.Repeat("─", max(0, m.width)))
	return titleLine + "\n" + rule
}

// contextHints surfaces what's actionable given the current git state and
// cursor position. Each hint is one composed string. Keep these short — they
// share a single line with the scroll percent.
func (m model) contextHints() []string {
	var hints []string

	switch {
	case m.conflicts:
		hints = append(hints,
			lipgloss.NewStyle().Foreground(colorErr).Bold(true).Render("⚠ conflicts")+
				dotStyle.Render(" → ")+hintStyle.Render("x")+dotStyle.Render(" resolve"))
	case m.dirty:
		hints = append(hints,
			behindStyle.Render("● dirty")+
				dotStyle.Render(" → ")+hintStyle.Render("c")+dotStyle.Render(" commit"))
	}

	if m.abValid {
		switch {
		case m.ahead > 0 && m.behind > 0:
			hints = append(hints,
				aheadStyle.Render(fmt.Sprintf("↑%d", m.ahead))+" "+
					behindStyle.Render(fmt.Sprintf("↓%d", m.behind))+
					dotStyle.Render(" diverged → ⏎ ")+hintStyle.Render("u")+
					dotStyle.Render(" force-push"))
		case m.ahead > 0:
			hints = append(hints,
				aheadStyle.Render(fmt.Sprintf("↑%d", m.ahead))+
					dotStyle.Render(" → ⏎ ")+hintStyle.Render("u")+dotStyle.Render(" push"))
		case m.behind > 0:
			hints = append(hints,
				behindStyle.Render(fmt.Sprintf("↓%d", m.behind))+
					dotStyle.Render(" → ⏎ ")+hintStyle.Render("b")+dotStyle.Render(" rebase"))
		case m.mainBranch != "" && m.branchName != m.mainBranch:
			hints = append(hints,
				dotStyle.Render("synced → ⏎ ")+hintStyle.Render("p")+
					dotStyle.Render(" PR to ")+branchPillStyle.Render(m.mainBranch))
		}
	}

	if m.cursor >= 0 && m.cursor < len(m.commits) {
		if p, ok := pipelineForCommit(m.commits[m.cursor], m.pipelines); ok && p.url != "" {
			switch {
			case isRunningState(p.state):
				hints = append(hints,
					behindStyle.Render("⏳ pipeline")+dotStyle.Render(" running"))
			case p.state == "failure", p.state == "timed_out", p.state == "startup_failure", p.state == "action_required":
				hints = append(hints,
					lipgloss.NewStyle().Foreground(colorErr).Bold(true).Render("✗ pipeline")+
						dotStyle.Render(" → ⏎ ")+hintStyle.Render("o")+dotStyle.Render(" open"))
			}
		}
	}

	return hints
}

func (m model) footerView() string {
	pct := pctStyle.Render(fmt.Sprintf("%3.f%%", m.viewport.ScrollPercent()*100))

	// dynamic checkout hint: show the branch that ⏎ would switch to
	checkoutHint := hintStyle.Render("⏎") + " "
	switch {
	case m.cursor < 0 || m.cursor >= len(m.commits):
		checkoutHint += dotStyle.Render("checkout")
	default:
		target := pickCheckoutTarget(m.commits[m.cursor], m.branchName)
		switch {
		case target == "":
			checkoutHint += dotStyle.Render("(no branch)")
		case target == m.branchName:
			checkoutHint += dotStyle.Render("on ") + branchPillStyle.Render(target)
		default:
			checkoutHint += dotStyle.Render("→ ") + branchPillStyle.Render(target)
		}
	}

	keys := []string{
		hintStyle.Render("↑↓") + dotStyle.Render(" select"),
		checkoutHint,
		hintStyle.Render("h") + dotStyle.Render(" help"),
		hintStyle.Render("q") + dotStyle.Render(" quit"),
	}
	left := strings.Join(keys, dotStyle.Render(" · "))

	// contextual hint line — kept on its own row so the static keys row stays
	// stable in width and the viewport doesn't resize as state changes.
	ctx := strings.Join(m.contextHints(), dotStyle.Render("  ·  "))
	if ctx == "" {
		ctx = dotStyle.Render("clean · nothing to do")
	}

	rule := ruleStyle.Render(strings.Repeat("─", max(0, m.width)))
	gap := max(1, m.width-lipgloss.Width(left)-lipgloss.Width(pct))
	return rule + "\n" + ctx + "\n" + left + strings.Repeat(" ", gap) + pct
}

func (m model) dialogView() string {
	var body string
	switch m.mode {
	case "help":
		rows := []struct{ k, d string }{
			{"↑/k", "select up"},
			{"↓/j", "select down"},
			{"g", "jump to top"},
			{"G", "jump to bottom"},
			{"⏎ enter", "branch action picker"},
			{"  c", "  · checkout"},
			{"  m", "  · merge into current"},
			{"  b", "  · rebase onto main (claude on conflict)"},
			{"  p", "  · open PR to main (gh pr create)"},
			{"  u", "  · push (force-with-lease if diverged)"},
			{"  o", "  · open pipeline in browser"},
			{"c", "claude /commit"},
			{"x", "claude /resolve-conflicts"},
			{"r", "refresh (fetch + rebuild)"},
			{"f", "fetch only"},
			{"h, ?", "toggle help"},
			{"esc", "close dialog"},
			{"q", "quit"},
		}
		var lines []string
		lines = append(lines, titleStyle.Render("Help"))
		lines = append(lines, "")
		keyCol := lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Width(10)
		descCol := lipgloss.NewStyle().Foreground(colorMuted)
		for _, r := range rows {
			lines = append(lines, keyCol.Render(r.k)+descCol.Render(r.d))
		}
		lines = append(lines, "")
		lines = append(lines, dotStyle.Render("press any key to close"))
		body = strings.Join(lines, "\n")
	case "action", "confirm", "result":
		var lines []string
		lines = append(lines, titleStyle.Render(m.dialogTitle))
		lines = append(lines, "")
		keyAccent := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
		selStyle := lipgloss.NewStyle().Bold(true).Foreground(colorAccent)

		switch m.mode {
		case "action":
			if m.pendingTarget != "" {
				lines = append(lines, dotStyle.Render("selected: ")+branchPillStyle.Render(m.pendingTarget))
			}
			lines = append(lines, dotStyle.Render("current:  ")+branchPillStyle.Render(m.branchName))
			lines = append(lines, "")
			for i, opt := range m.actionOpts {
				marker := "  "
				if i == m.actionCursor {
					marker = selStyle.Render("▸ ")
				}
				key := keyAccent.Render(opt.key)
				label := opt.label
				if i == m.actionCursor {
					label = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Render(label)
				} else {
					label = lipgloss.NewStyle().Foreground(colorMuted).Render(label)
				}
				lines = append(lines, marker+key+"  "+label)
			}
			lines = append(lines, "")
			lines = append(lines, dotStyle.Render("↑↓ select  ⏎ confirm  n cancel"))
		case "confirm":
			lines = append(lines, m.dialogBody)
			lines = append(lines, "")
			lines = append(lines,
				keyAccent.Render("y")+dotStyle.Render(" confirm  ")+
					keyAccent.Render("n")+dotStyle.Render(" cancel"))
		default:
			lines = append(lines, m.dialogBody)
			lines = append(lines, "")
			lines = append(lines, dotStyle.Render("press any key to close"))
		}
		body = strings.Join(lines, "\n")
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(1, 2).
		Render(body)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	keyHandled := false

	switch msg := msg.(type) {
	case tea.KeyMsg:
		k := msg.String()

		// modal modes consume keys
		switch m.mode {
		case "help", "result":
			if k == "q" || k == "ctrl+c" {
				return m, tea.Quit
			}
			m.mode = "list"
			keyHandled = true
		case "action":
			switch k {
			case "up", "k":
				if m.actionCursor > 0 {
					m.actionCursor--
				}
			case "down", "j":
				if m.actionCursor < len(m.actionOpts)-1 {
					m.actionCursor++
				}
			case "enter":
				if m.actionCursor >= 0 && m.actionCursor < len(m.actionOpts) {
					cmds = append(cmds, m.executeAction(m.actionOpts[m.actionCursor])...)
				}
			case "n", "N", "esc", "q":
				m.mode = "list"
				m.pendingTarget = ""
				m.pendingAction = ""
				m.actionOpts = nil
			default:
				// letter shortcut
				for _, opt := range m.actionOpts {
					if strings.EqualFold(opt.key, k) {
						cmds = append(cmds, m.executeAction(opt)...)
						break
					}
				}
			}
			keyHandled = true
		case "confirm":
			switch k {
			case "y", "Y":
				target := m.pendingTarget
				action := m.pendingAction
				m.mode = "list"
				m.pendingTarget = ""
				m.pendingAction = ""
				if action == "merge" {
					cmds = append(cmds, runGitMerge(m.repoPath, target))
				} else {
					cmds = append(cmds, runGitCheckout(m.repoPath, target))
				}
			case "n", "N", "esc", "q":
				m.mode = "list"
				m.pendingTarget = ""
				m.pendingAction = ""
			}
			keyHandled = true
		default: // "list"
			switch k {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "h", "?":
				m.mode = "help"
				keyHandled = true
			case "r":
				m.fetching = true
				cmds = append(cmds,
					runGitFetch(m.repoPath),
					runGitLog(m.repoPath, m.maxCommits),
					runGitBranch(m.repoPath),
					runGitAheadBehind(m.repoPath),
					runGitHubPipelines(m.repoPath),
					runGitStatus(m.repoPath))
				keyHandled = true
			case "f":
				m.fetching = true
				cmds = append(cmds, runGitFetch(m.repoPath))
				keyHandled = true
			case "c":
				cmds = append(cmds, runClaude(m.repoPath, "/commit"))
				keyHandled = true
			case "x":
				cmds = append(cmds, runClaude(m.repoPath, "/resolve-conflicts"))
				keyHandled = true
			case "g":
				m.cursor = 0
				m.refreshDisplay()
				m.viewport.GotoTop()
				keyHandled = true
			case "G":
				if len(m.commits) > 0 {
					m.cursor = len(m.commits) - 1
				}
				m.refreshDisplay()
				m.viewport.GotoBottom()
				keyHandled = true
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
					m.refreshDisplay()
					m.ensureCursorVisible()
				}
				keyHandled = true
			case "down", "j":
				if m.cursor < len(m.commits)-1 {
					m.cursor++
					m.refreshDisplay()
					m.ensureCursorVisible()
				}
				keyHandled = true
			case "enter":
				if m.cursor >= 0 && m.cursor < len(m.commits) {
					c := m.commits[m.cursor]
					target := pickCheckoutTarget(c, m.branchName)
					onCurrent := target != "" && target == m.branchName
					pipe, hasPipe := pipelineForCommit(c, m.pipelines)

					var opts []actionOption
					switch {
					case target != "" && !onCurrent:
						opts = append(opts, actionOption{
							key: "c", kind: "checkout", payload: target,
							label: "checkout " + target,
						})
						opts = append(opts, actionOption{
							key: "m", kind: "merge", payload: target,
							label: "merge " + target + " into " + m.branchName,
						})
					case !c.IsHead && c.Hash != "":
						short := c.Hash
						if len(short) > 7 {
							short = short[:7]
						}
						opts = append(opts, actionOption{
							key: "c", kind: "checkout", payload: short,
							label: "checkout " + short + " (detached)",
						})
					}
					if onCurrent && m.mainBranch != "" && m.mainBranch != m.branchName {
						opts = append(opts, actionOption{
							key: "b", kind: "rebase", payload: m.mainBranch,
							label: "rebase onto origin/" + m.mainBranch,
						})
						opts = append(opts, actionOption{
							key: "p", kind: "pr", payload: m.mainBranch,
							label: "open PR to " + m.mainBranch,
						})
					}
					if onCurrent && m.abValid {
						switch {
						case m.ahead > 0 && m.behind == 0:
							opts = append(opts, actionOption{
								key: "u", kind: "push", payload: "",
								label: fmt.Sprintf("push (%d ahead)", m.ahead),
							})
						case m.ahead > 0 && m.behind > 0:
							opts = append(opts, actionOption{
								key: "u", kind: "push-lease", payload: "",
								label: fmt.Sprintf("push --force-with-lease (↑%d ↓%d)", m.ahead, m.behind),
							})
						case m.ahead == 0 && m.behind > 0:
							opts = append(opts, actionOption{
								key: "u", kind: "push-lease", payload: "",
								label: fmt.Sprintf("force push --force-with-lease (overwrite origin, ↓%d)", m.behind),
							})
						}
					}
					if hasPipe && pipe.url != "" {
						opts = append(opts, actionOption{
							key: "o", kind: "open", payload: pipe.url,
							label: "open pipeline (" + pipe.state + ")",
						})
					}

					if len(opts) == 0 {
						m.mode = "result"
						m.dialogTitle = "Branch action"
						switch {
						case target == "":
							m.dialogBody = "no local branch on this commit"
						case onCurrent:
							m.dialogBody = "already on " + target + " · no actions available"
						default:
							m.dialogBody = "no actions available"
						}
					} else {
						m.mode = "action"
						m.pendingTarget = target
						m.actionOpts = opts
						m.actionCursor = 0
						m.dialogTitle = "Branch action"
					}
				}
				keyHandled = true
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerH := lipgloss.Height(m.headerView())
		footerH := lipgloss.Height(m.footerView())
		vpH := max(1, msg.Height-headerH-footerH)
		if !m.ready {
			m.viewport = viewport.New(msg.Width, vpH)
			m.viewport.SetContent(m.content)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = vpH
		}
		m.refreshDisplay()

	case logMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			m.commits = msg.commits
			m.commitRow = msg.commitRow
			m.headSha = msg.headSha
			if m.cursor >= len(m.commits) {
				m.cursor = len(m.commits) - 1
			}
			if m.cursor < 0 {
				m.cursor = 0
			}
			// rebuild with current pipelines (msg.content didn't see them)
			m.contentBase, m.commitRow = buildContent(m.commits, m.headSha, m.pipelines, m.spinnerFrame)
			m.refreshDisplay()
		}

	case pipelinesMsg:
		m.pipelines = msg.pipelines
		if len(m.commits) > 0 {
			m.contentBase, m.commitRow = buildContent(m.commits, m.headSha, m.pipelines, m.spinnerFrame)
			m.refreshDisplay()
		}

	case spinnerTickMsg:
		if anyRunning(m.pipelines) && len(m.commits) > 0 {
			m.spinnerFrame++
			m.contentBase, m.commitRow = buildContent(m.commits, m.headSha, m.pipelines, m.spinnerFrame)
			m.refreshDisplay()
		}
		cmds = append(cmds, spinnerTick())

	case openMsg:
		if msg.err != nil {
			m.mode = "result"
			m.dialogTitle = "Open"
			m.dialogBody = "failed:\n" + msg.err.Error()
		}

	case branchMsg:
		m.branchName = msg.name

	case aheadBehindMsg:
		m.ahead = msg.ahead
		m.behind = msg.behind
		m.abValid = msg.valid

	case checkoutMsg:
		m.mode = "result"
		m.dialogTitle = "Checkout"
		if msg.err != nil {
			m.dialogBody = "failed:\n" + msg.err.Error()
		} else {
			m.dialogBody = "switched to " + msg.target
			if msg.out != "" {
				m.dialogBody += "\n\n" + msg.out
			}
			cmds = append(cmds,
				runGitLog(m.repoPath, m.maxCommits),
				runGitBranch(m.repoPath),
				runGitAheadBehind(m.repoPath),
				runGitStatus(m.repoPath))
		}

	case claudeFinishedMsg:
		cmds = append(cmds,
			runGitLog(m.repoPath, m.maxCommits),
			runGitBranch(m.repoPath),
			runGitAheadBehind(m.repoPath),
			runGitStatus(m.repoPath))
		if msg.err != nil {
			m.mode = "result"
			m.dialogTitle = "claude " + msg.cmd
			m.dialogBody = "exited with error:\n" + msg.err.Error()
		}

	case mainBranchMsg:
		m.mainBranch = msg.name

	case rebaseMsg:
		if msg.err != nil {
			if hasMergeConflicts(m.repoPath) {
				// hand off to claude to resolve + finish rebase
				cmds = append(cmds, runClaude(m.repoPath,
					"/resolve-conflicts and fulfill the rebase for me"))
			} else {
				m.mode = "result"
				m.dialogTitle = "Rebase"
				m.dialogBody = "failed:\n" + msg.err.Error()
			}
		} else {
			m.mode = "result"
			m.dialogTitle = "Rebase"
			m.dialogBody = "rebased " + m.branchName + " onto " + msg.base
			if msg.out != "" {
				m.dialogBody += "\n\n" + msg.out
			}
			cmds = append(cmds,
				runGitLog(m.repoPath, m.maxCommits),
				runGitBranch(m.repoPath),
				runGitAheadBehind(m.repoPath),
				runGitHubPipelines(m.repoPath),
				runGitStatus(m.repoPath))
		}

	case pushMsg:
		m.mode = "result"
		m.dialogTitle = "Push"
		if msg.err != nil {
			m.dialogBody = "failed:\n" + msg.err.Error()
		} else {
			label := "pushed " + m.branchName
			if msg.withLease {
				label += " (--force-with-lease)"
			}
			m.dialogBody = label
			if msg.out != "" {
				m.dialogBody += "\n\n" + msg.out
			}
			cmds = append(cmds,
				runGitLog(m.repoPath, m.maxCommits),
				runGitAheadBehind(m.repoPath),
				runGitHubPipelines(m.repoPath))
		}

	case prMsg:
		m.mode = "result"
		m.dialogTitle = "Pull Request"
		if msg.err != nil {
			m.dialogBody = "failed:\n" + msg.err.Error()
		} else {
			m.dialogBody = "PR created"
			if msg.url != "" {
				m.dialogBody += ": " + msg.url
				cmds = append(cmds, openURL(msg.url))
			}
			if msg.out != "" {
				m.dialogBody += "\n\n" + msg.out
			}
		}

	case mergeMsg:
		m.mode = "result"
		m.dialogTitle = "Merge"
		if msg.err != nil {
			m.dialogBody = "failed:\n" + msg.err.Error()
		} else {
			m.dialogBody = "merged " + msg.target + " into " + m.branchName
			if msg.out != "" {
				m.dialogBody += "\n\n" + msg.out
			}
			cmds = append(cmds,
				runGitLog(m.repoPath, m.maxCommits),
				runGitBranch(m.repoPath),
				runGitAheadBehind(m.repoPath),
				runGitStatus(m.repoPath))
		}

	case tickMsg:
		m.fetching = true
		cmds = append(cmds,
			runGitFetch(m.repoPath),
			runGitLog(m.repoPath, m.maxCommits),
			runGitBranch(m.repoPath),
			runGitAheadBehind(m.repoPath),
			runGitHubPipelines(m.repoPath),
			runGitStatus(m.repoPath),
			tick(m.refreshEvery),
		)

	case fetchMsg:
		m.fetching = false
		m.lastFetch = msg.ts
		m.fetchErr = msg.err
		// re-pull ahead/behind now that remote-tracking refs are fresh
		cmds = append(cmds, runGitAheadBehind(m.repoPath))

	case statusMsg:
		m.dirty = msg.dirty
		m.conflicts = msg.conflicts
	}

	if !keyHandled {
		var vpCmd tea.Cmd
		m.viewport, vpCmd = m.viewport.Update(msg)
		cmds = append(cmds, vpCmd)
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if !m.ready {
		return "loading…"
	}
	if m.err != nil {
		return errorStyle.Render(fmt.Sprintf("error: %v\n\npress r to retry, q to quit", m.err))
	}
	if m.mode == "help" || m.mode == "confirm" || m.mode == "result" || m.mode == "action" {
		return m.dialogView()
	}
	return fmt.Sprintf("%s\n%s\n%s", m.headerView(), m.viewport.View(), m.footerView())
}
