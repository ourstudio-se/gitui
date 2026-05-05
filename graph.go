package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func laneRender(idx int, current, underlined bool, s string) string {
	style := lipgloss.NewStyle().Foreground(laneColors[idx%len(laneColors)])
	if current {
		style = style.Bold(true)
	}
	if underlined {
		style = style.Underline(true)
	}
	return style.Render(s)
}

// underlineDisplay wraps git's pre-colored Display text with underline ANSI,
// re-enabling underline after every embedded `\x1b[0m` / `\x1b[m` reset so
// nested color sequences don't drop the underline mid-line.
func underlineDisplay(s string) string {
	s = strings.ReplaceAll(s, "\x1b[0m", "\x1b[0m\x1b[4m")
	s = strings.ReplaceAll(s, "\x1b[m", "\x1b[m\x1b[4m")
	return "\x1b[4m" + s + "\x1b[0m"
}

func buildContent(commits []Commit, headSha string, pipelines map[string]pipelineStatus, spinnerFrame int) (string, []int) {
	var rows []rowSpec
	commitRow := make([]int, len(commits))
	var lanes []lane
	maxLaneCount := 0
	nextColor := 0

	for ci, c := range commits {
		// find slots referencing this commit
		var slots []int
		for i, l := range lanes {
			if l.sha == c.Hash {
				slots = append(slots, i)
			}
		}

		var primaryIdx, primaryColor int
		var primaryCurrent bool
		if len(slots) > 0 {
			primaryIdx = slots[0]
			primaryColor = lanes[primaryIdx].color
			primaryCurrent = lanes[primaryIdx].current
		} else {
			// new branch tip — find empty slot or append
			primaryIdx = -1
			for i, l := range lanes {
				if l.sha == "" {
					primaryIdx = i
					break
				}
			}
			if primaryIdx == -1 {
				primaryIdx = len(lanes)
				lanes = append(lanes, lane{})
			}
			primaryColor = nextColor
			nextColor++
			lanes[primaryIdx] = lane{sha: c.Hash, color: primaryColor}
		}
		isHead := c.Hash == headSha
		if isHead {
			primaryCurrent = true
			lanes[primaryIdx].current = true
		}

		// commit row
		commitCells := make([]string, len(lanes))
		for i, l := range lanes {
			switch {
			case i == primaryIdx:
				commitCells[i] = laneRender(primaryColor, primaryCurrent, isHead, "● ")
			case l.sha != "":
				commitCells[i] = laneRender(l.color, l.current, false, "│ ")
			default:
				commitCells[i] = "  "
			}
		}
		displayText := c.Display
		if isHead {
			displayText = underlineDisplay(displayText)
		}
		if icon := pipelineIconFor(c, pipelines, spinnerFrame); icon != "" {
			displayText = icon + " " + displayText
		}
		commitRow[ci] = len(rows)
		rows = append(rows, rowSpec{cells: commitCells, text: displayText})
		if len(commitCells) > maxLaneCount {
			maxLaneCount = len(commitCells)
		}

		// build new lane state
		newLanes := make([]lane, len(lanes))
		copy(newLanes, lanes)
		if len(c.Parents) > 0 {
			newLanes[primaryIdx] = lane{sha: c.Parents[0], color: primaryColor, current: primaryCurrent}
		} else {
			newLanes[primaryIdx] = lane{}
		}
		recentlyEmptied := map[int]bool{}
		for _, i := range slots {
			if i != primaryIdx {
				newLanes[i] = lane{}
				recentlyEmptied[i] = true
			}
		}
		// extra parents (merge): prefer pre-existing empty slots, else append
		for j := 1; j < len(c.Parents); j++ {
			placed := false
			for k := 0; k < len(newLanes); k++ {
				if newLanes[k].sha == "" && !recentlyEmptied[k] && k != primaryIdx {
					newLanes[k] = lane{sha: c.Parents[j], color: nextColor}
					nextColor++
					placed = true
					break
				}
			}
			if !placed {
				newLanes = append(newLanes, lane{sha: c.Parents[j], color: nextColor})
				nextColor++
			}
		}
		// trim trailing empty lanes
		for len(newLanes) > 0 && newLanes[len(newLanes)-1].sha == "" {
			newLanes = newLanes[:len(newLanes)-1]
		}

		// transition row when topology changed. Uses box-drawing corners
		// (╭ ╮ ╰ ╯), T-junctions (┬ ┴ ├ ┤), and crosses (┼) so glyphs tile
		// center-to-center. Each lane is 2 cells wide (cell0 = main glyph,
		// cell1 = spacer); spacer becomes `─` when horizontal stroke crosses
		// into the next lane.
		if !lanesEqual(lanes, newLanes) {
			width := max(len(lanes), len(newLanes))

			// classify per-column events
			closingCols := map[int]bool{}
			spawningCols := map[int]bool{}
			for i := 0; i < width; i++ {
				var oldSha, newSha string
				if i < len(lanes) {
					oldSha = lanes[i].sha
				}
				if i < len(newLanes) {
					newSha = newLanes[i].sha
				}
				switch {
				case oldSha != "" && newSha == "":
					closingCols[i] = true
				case oldSha == "" && newSha != "":
					spawningCols[i] = true
				}
			}
			// no closing/spawning = primary just carried forward parent[0];
			// no visual transition needed — skip the row.
			if len(closingCols) == 0 && len(spawningCols) == 0 {
				lanes = newLanes
				continue
			}
			// horizontal stroke span = primary..(furthest event)
			horizMin, horizMax := primaryIdx, primaryIdx
			for c := range closingCols {
				if c < horizMin {
					horizMin = c
				}
				if c > horizMax {
					horizMax = c
				}
			}
			for c := range spawningCols {
				if c < horizMin {
					horizMin = c
				}
				if c > horizMax {
					horizMax = c
				}
			}
			eventsLeft := horizMin < primaryIdx
			eventsRight := horizMax > primaryIdx

			transCells := make([]string, width)
			for i := 0; i < width; i++ {
				var oldSha, newSha string
				var oldColor, newColor int
				if i < len(lanes) {
					oldSha = lanes[i].sha
					oldColor = lanes[i].color
				}
				if i < len(newLanes) {
					newSha = newLanes[i].sha
					newColor = newLanes[i].color
				}
				inHoriz := i >= horizMin && i <= horizMax

				var ch string
				var color int
				var current bool
				cellEmpty := false

				switch {
				case i == primaryIdx:
					color = primaryColor
					current = primaryCurrent
					switch {
					case eventsLeft && eventsRight:
						ch = "┼"
					case eventsRight:
						ch = "├"
					case eventsLeft:
						ch = "┤"
					default:
						ch = "│"
					}
				case closingCols[i]:
					color = oldColor
					current = lanes[i].current
					if i > primaryIdx {
						if i == horizMax {
							ch = "╯"
						} else {
							ch = "┴"
						}
					} else {
						if i == horizMin {
							ch = "╰"
						} else {
							ch = "┴"
						}
					}
				case spawningCols[i]:
					color = newColor
					current = newLanes[i].current
					if i > primaryIdx {
						if i == horizMax {
							ch = "╮"
						} else {
							ch = "┬"
						}
					} else {
						if i == horizMin {
							ch = "╭"
						} else {
							ch = "┬"
						}
					}
				case oldSha != "" && newSha != "":
					if oldSha != newSha {
						color = newColor
						current = newLanes[i].current
					} else {
						color = oldColor
						current = lanes[i].current
					}
					if inHoriz {
						ch = "┼"
					} else {
						ch = "│"
					}
				default:
					if inHoriz {
						ch = "─"
						color = primaryColor
						current = primaryCurrent
					} else {
						cellEmpty = true
					}
				}

				// spacer to right of cell0: `─` only if the horizontal stroke
				// crosses into the next lane (i.e., i is inside [horizMin, horizMax)).
				spacer := " "
				if i >= horizMin && i < horizMax {
					spacer = laneRender(primaryColor, primaryCurrent, false, "─")
				}

				if cellEmpty {
					transCells[i] = " " + spacer
				} else {
					transCells[i] = laneRender(color, current, false, ch) + spacer
				}
			}
			rows = append(rows, rowSpec{cells: transCells})
			if width > maxLaneCount {
				maxLaneCount = width
			}
		}

		lanes = newLanes
	}

	// no global right-pad: in narrow side-panes, padding to the all-rows
	// max-lane-count pushes commit text off-screen. Each row prints its own
	// lanes and commit text follows directly.
	_ = maxLaneCount
	var b strings.Builder
	for _, r := range rows {
		for _, cell := range r.cells {
			b.WriteString(cell)
		}
		if r.text != "" {
			b.WriteString(" ")
			b.WriteString(r.text)
		}
		b.WriteString("\n")
	}
	return b.String(), commitRow
}

// applyHighlight wraps the cursor's row with a background highlight. The base
// content is held unmodified in the model; we re-apply this on cursor change
// (cheap — split, wrap one line, rejoin).
func applyHighlight(content string, commitRow []int, cursor, width int) string {
	if cursor < 0 || cursor >= len(commitRow) {
		return content
	}
	target := commitRow[cursor]
	lines := strings.Split(content, "\n")
	if target < 0 || target >= len(lines) {
		return content
	}
	lines[target] = highlightLine(lines[target], width)
	return strings.Join(lines, "\n")
}

func highlightLine(s string, width int) string {
	// amber-900 — clearly yellow but dark enough that colored fg text stays readable.
	const bg = "\x1b[48;2;120;53;15m"
	const off = "\x1b[49m"
	// re-enable bg after every embedded reset so nested fg colors don't drop bg.
	s = strings.ReplaceAll(s, "\x1b[0m", "\x1b[0m"+bg)
	s = strings.ReplaceAll(s, "\x1b[m", "\x1b[m"+bg)
	visible := lipgloss.Width(s)
	if width > visible {
		s += strings.Repeat(" ", width-visible)
	}
	return bg + s + off
}

func lanesEqual(a, b []lane) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].sha != b[i].sha || a[i].color != b[i].color {
			return false
		}
	}
	return true
}
