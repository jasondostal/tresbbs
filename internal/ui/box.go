// Package ui holds shared terminal-rendering helpers.
package ui

import (
	"regexp"
	"strings"
)

// BoxInnerWidth is the number of columns between the ║ borders of every box the
// BBS draws. Screens are hand-drawn (in Go and in .tpl files) with slightly
// inconsistent widths; NormalizeBox rebuilds any box line to this width so the
// right border always lines up.
const BoxInnerWidth = 65

var (
	// any ANSI escape (color, cursor, clear, …), not just color
	ansiRe      = regexp.MustCompile("\x1b\\[[0-9;]*[A-Za-z]")
	leadAnsiRe  = regexp.MustCompile("^(?:\x1b\\[[0-9;]*[A-Za-z])+")
	trailAnsiRe = regexp.MustCompile("(?:\x1b\\[[0-9;]*[A-Za-z])+$")
	boxBorderRe = regexp.MustCompile(`^([╔╠╚])═+([╗╣╝])$`)
	boxRowRe    = regexp.MustCompile(`^║(.*)║$`)
)

// visWidth is the visible column count of s, ignoring ANSI escapes.
func visWidth(s string) int {
	return len([]rune(ansiRe.ReplaceAllString(s, "")))
}

// NormalizeBox rebuilds a box-drawing line (a ╔═╗/╠═╣/╚═╝ border, or a ║ … ║
// content row) to the standard inner width. ANSI escapes wrapping the line
// (a color set at the start, a clear-screen before the top border, a reset at
// the end) are preserved. Non-box lines pass through untouched. Over-long
// content is truncated so a box can never blow out past its border.
func NormalizeBox(line string) string {
	lead := leadAnsiRe.FindString(line)
	core := line[len(lead):]
	trail := trailAnsiRe.FindString(core)
	core = core[:len(core)-len(trail)]

	if m := boxBorderRe.FindStringSubmatch(core); m != nil {
		return lead + m[1] + strings.Repeat("═", BoxInnerWidth) + m[2] + trail
	}
	if m := boxRowRe.FindStringSubmatch(core); m != nil {
		inner := m[1]
		vis := visWidth(inner)
		if vis > BoxInnerWidth {
			inner = string([]rune(inner)[:BoxInnerWidth]) // rare; content is mostly plain
			vis = BoxInnerWidth
		}
		return lead + "║" + inner + strings.Repeat(" ", BoxInnerWidth-vis) + "║" + trail
	}
	return line
}

// NormalizeBoxLines applies NormalizeBox to every line of a multi-line string,
// preserving \r\n line endings.
func NormalizeBoxLines(text string) string {
	lines := strings.Split(text, "\n")
	for i, ln := range lines {
		cr := strings.HasSuffix(ln, "\r")
		out := NormalizeBox(strings.TrimSuffix(ln, "\r"))
		if cr {
			out += "\r"
		}
		lines[i] = out
	}
	return strings.Join(lines, "\n")
}
