package main

import "github.com/charmbracelet/lipgloss"

var (
	colorAccent = lipgloss.Color("#7DD3FC")
	colorBranch = lipgloss.Color("#C4B5FD")
	colorMuted  = lipgloss.Color("#475569")
	colorRule   = lipgloss.Color("#1E293B")
	colorErr    = lipgloss.Color("#F87171")
	colorIconFg = lipgloss.Color("#94A3B8")

	titleStyle      = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	branchPillStyle = lipgloss.NewStyle().Foreground(colorBranch).Bold(true)
	dotStyle        = lipgloss.NewStyle().Foreground(colorMuted)
	ruleStyle       = lipgloss.NewStyle().Foreground(colorRule)
	hintStyle       = lipgloss.NewStyle().Foreground(colorMuted)
	pctStyle        = lipgloss.NewStyle().Foreground(colorMuted).Bold(true)
	iconStyle       = lipgloss.NewStyle().Foreground(colorIconFg)
	errorStyle      = lipgloss.NewStyle().Foreground(colorErr).Padding(1, 2)

	aheadStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#34D399")).Bold(true)
	behindStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24")).Bold(true)

	laneColors = []lipgloss.Color{
		"#34D399", // green
		"#60A5FA", // blue
		"#A78BFA", // violet
		"#F472B6", // pink
		"#FBBF24", // amber
		"#22D3EE", // cyan
		"#F87171", // red
	}
)
