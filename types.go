package main

import (
	"time"

	"github.com/charmbracelet/bubbles/viewport"
)

const (
	recordSep = "\x1e"
	fieldSep  = "\x1f"
)

// ---- messages ----

type (
	tickMsg        time.Time
	spinnerTickMsg time.Time

	logMsg struct {
		content   string
		commits   []Commit
		commitRow []int
		headSha   string
		err       error
	}
	branchMsg      struct{ name string }
	mainBranchMsg  struct{ name string }
	aheadBehindMsg struct {
		ahead, behind int
		valid         bool
	}
	checkoutMsg struct {
		target string
		out    string
		err    error
	}
	mergeMsg struct {
		target string
		out    string
		err    error
	}
	rebaseMsg struct {
		base string
		out  string
		err  error
	}
	prMsg struct {
		base string
		out  string
		url  string
		err  error
	}
	pushMsg struct {
		withLease bool
		out       string
		err       error
	}
	fetchMsg struct {
		ts  time.Time
		err error
	}
	statusMsg struct {
		dirty, conflicts bool
	}
	claudeFinishedMsg struct {
		cmd string
		err error
	}
	pipelinesMsg struct {
		pipelines map[string]pipelineStatus
	}
	openMsg struct {
		url string
		err error
	}
)

// ---- domain types ----

type (
	pipelineStatus struct {
		state string // "in_progress", "queued", "success", "failure", "cancelled", "skipped", ...
		url   string
	}

	Commit struct {
		Hash           string
		Parents        []string
		Display        string
		LocalBranches  []string
		RemoteBranches []string
		Tags           []string
		IsHead         bool
	}

	lane struct {
		sha     string
		color   int
		current bool
	}

	rowSpec struct {
		cells []string // each cell is 2 visible cells wide ("X ")
		text  string   // commit display text (commit rows only)
	}

	actionOption struct {
		key     string // single-letter shortcut
		label   string
		kind    string // "checkout" | "merge" | "rebase" | "pr" | "open"
		payload string // target branch / URL
	}

	model struct {
		viewport     viewport.Model
		content      string
		contentBase  string
		commits      []Commit
		commitRow    []int
		headSha      string
		pipelines    map[string]pipelineStatus
		spinnerFrame int
		mainBranch   string
		fetching     bool
		lastFetch    time.Time
		fetchErr     error
		err          error
		width        int
		height       int
		ready        bool
		repoPath     string
		refreshEvery time.Duration
		maxCommits   int
		branchName   string
		useIcons     bool
		ahead        int
		behind       int
		abValid      bool
		dirty        bool
		conflicts    bool
		cursor       int
		mode         string // "list", "help", "confirm", "result"
		dialogTitle  string
		dialogBody   string
		pendingTarget string // branch name awaiting confirm
		pendingAction string // "checkout" or "merge"
		actionOpts    []actionOption
		actionCursor  int
	}
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
