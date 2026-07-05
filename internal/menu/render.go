package menu

import (
	"fmt"
	"strings"
)

// innerWidth is the interior column count of a menu box (between the ║ borders).
// With a 2-column margin the box sits centered in an 80-column screen.
const innerWidth = 72

// RenderContext carries the per-caller state a menu needs to render: the board
// name (prepended to the title) and the caller's security level (which items
// are visible).
type RenderContext struct {
	BoardName string
	Security  int
}

// Render draws the menu box exactly as TriBBS auto-drew it from the .MNU: a
// double-line frame in the border colors, a blue (or configured) interior panel,
// the board+menu title centered in the top border, and the visible items laid
// out in two columns as "<K>..Description". The returned string is CRLF-joined
// and ends without a trailing newline; it contains embedded ANSI color codes.
func (m *Menu) Render(ctx RenderContext) string {
	border := attr(m.BorderBg, m.BorderFg)
	panel := attr(m.PanelBg, m.PanelFg)
	title := attr(m.BorderBg, 15) // bright white title on the border color

	var lines []string

	// Top border with the centered title embedded in the ═ run.
	titleText := " " + strings.TrimSpace(ctx.BoardName+" "+m.Title) + " "
	if len([]rune(titleText)) > innerWidth {
		titleText = titleText[:innerWidth]
	}
	dash := innerWidth - len([]rune(titleText))
	left := dash / 2
	right := dash - left
	top := border + "╔" + strings.Repeat("═", left) +
		title + titleText + border + strings.Repeat("═", right) + "╗" + ansiReset
	lines = append(lines, top)

	// Item rows, two columns.
	items := m.VisibleItems(ctx.Security)
	rows := twoColumnRows(items)
	for _, row := range rows {
		lines = append(lines, border+"║"+panel+padTo(row, innerWidth)+border+"║"+ansiReset)
	}

	// Bottom border.
	lines = append(lines, border+"╚"+strings.Repeat("═", innerWidth)+"╝"+ansiReset)

	// Indent the whole box by two columns to center it.
	for i, ln := range lines {
		lines[i] = "  " + ln
	}
	return strings.Join(lines, "\r\n")
}

// twoColumnRows lays items into a left/right column grid, filling the left
// column top-to-bottom first (so reading order is columnar, as TriBBS did).
func twoColumnRows(items []Item) []string {
	n := len(items)
	if n == 0 {
		return nil
	}
	half := (n + 1) / 2 // left column gets the extra item on an odd count
	colWidth := (innerWidth - 3) / 2

	var rows []string
	for i := 0; i < half; i++ {
		left := itemCell(items[i], colWidth)
		right := ""
		if j := i + half; j < n {
			right = itemCell(items[j], colWidth)
		}
		rows = append(rows, " "+left+"  "+right)
	}
	return rows
}

// itemCell formats one item as "<K>..Description", padded/truncated to width.
func itemCell(it Item, width int) string {
	cell := fmt.Sprintf("<%c>..%s", it.Key, it.Description)
	return padTo(cell, width)
}

// padTo pads s with spaces (or truncates) to exactly n visible runes. Menu cell
// text contains no embedded ANSI, so a plain rune count is the visible width.
func padTo(s string, n int) string {
	r := []rune(s)
	if len(r) >= n {
		return string(r[:n])
	}
	return s + strings.Repeat(" ", n-len(r))
}

// OnlineLine is the "You have been on N minutes with M remaining." line TriBBS
// prints under the menu box.
func OnlineLine(minutesOn, minutesLeft int) string {
	return fmt.Sprintf("You have been on %d minutes with %d remaining.", minutesOn, minutesLeft)
}

// StatusInfo is the data for the persistent bottom status bar.
type StatusInfo struct {
	Name      string
	Location  string // "City, ST"
	Node      int
	Connection string // "LOCAL", "TELNET", "SSH", or a baud rate
	Security  int
	Calls     int
	TimeUsed  int // minutes this call
	TimeLeft  int // minutes remaining today
}

// StatusBar renders TriBBS's two-line gray status bar shown at the bottom of the
// screen: identity on the first line, session stats on the second.
func StatusBar(info StatusInfo) string {
	bar := attr(7, 0) // black on gray
	line1 := fmt.Sprintf(" %s  %s  [%d](%s)", info.Name, info.Location, info.Node, info.Connection)
	line2 := fmt.Sprintf(" Sec Level: %d   Calls: %d   Time Used: %d   Time Left: %d",
		info.Security, info.Calls, info.TimeUsed, info.TimeLeft)
	return bar + padTo(line1, 80) + ansiReset + "\r\n" +
		bar + padTo(line2, 80) + ansiReset
}
