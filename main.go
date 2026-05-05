package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// version is set at build time via -ldflags "-X main.version=…".
var version = "dev"

func main() {
	repo := flag.String("repo", ".", "path to git repository")
	maxCommits := flag.Int("n", 500, "max commits to show")
	refreshSec := flag.Int("refresh", 5, "refresh interval seconds")
	noIcons := flag.Bool("no-icons", false, "disable nerd-font glyphs")
	dump := flag.Bool("dump", false, "render to stdout and exit (no TUI)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	if _, err := exec.LookPath("git"); err != nil {
		fmt.Fprintln(os.Stderr, "git not found in PATH")
		os.Exit(1)
	}

	if err := exec.Command("git", "-C", *repo, "rev-parse", "--git-dir").Run(); err != nil {
		fmt.Fprintf(os.Stderr, "not a git repo: %s\n", *repo)
		os.Exit(1)
	}

	if *dump {
		lm := customGitLog(*repo, *maxCommits).(logMsg)
		if lm.err != nil {
			fmt.Fprintln(os.Stderr, lm.err)
			os.Exit(1)
		}
		fmt.Print(lm.content)
		return
	}

	m := model{
		repoPath:     *repo,
		maxCommits:   *maxCommits,
		refreshEvery: time.Duration(*refreshSec) * time.Second,
		branchName:   "…",
		useIcons:     !*noIcons,
		mode:         "list",
		fetching:     true,
	}

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
