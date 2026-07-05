// Package ansi implements RIPscrip graphics support for the display adapter.
//
// RIPscrip (Remote Imaging Protocol) was used by TriBBS and other BBS software
// to provide graphical interfaces over terminal connections. RIPscrip sequences
// are embedded in ANSI output and rendered as graphical elements when the
// client supports RIP mode.
//
// RIP sequences use the format: !<command><data>~
// where <command> is a two-character hex code.
package ansi

import (
	"fmt"
	"strings"
)

// RIPCommand represents a RIPscrip command.
type RIPCommand struct {
	Command byte   // Two-character hex command code
	Params  string // Command parameters
}

// RIPRenderer handles rendering of RIPscrip graphics commands.
type RIPRenderer struct {
	enabled    bool
	fullScreen bool
	width      int
	height     int
	mouseRegions []MouseRegion
	writer     func(string)
}

// MouseRegion defines a clickable region on screen for RIP mode.
type MouseRegion struct {
	X1, Y1, X2, Y2 int
	RegionNum       int
	HotKey          byte
}

// NewRIPRenderer creates a new RIPscrip renderer.
func NewRIPRenderer(writer func(string)) *RIPRenderer {
	return &RIPRenderer{
		enabled:    false,
		fullScreen: false,
		width:      80,
		height:     25,
		mouseRegions: make([]MouseRegion, 0),
		writer:     writer,
	}
}

// Enable activates RIPscrip mode.
func (r *RIPRenderer) Enable() {
	r.enabled = true
	r.fullScreen = true
	// Send RIP initialization sequence
	r.writer("\x1b[!p") // RIP detection query
}

// Disable deactivates RIPscrip mode.
func (r *RIPRenderer) Disable() {
	r.enabled = false
	r.fullScreen = false
	r.mouseRegions = nil
}

// IsEnabled returns whether RIPscrip mode is active.
func (r *RIPRenderer) IsEnabled() bool {
	return r.enabled
}

// IsFullScreen returns whether full-screen RIP mode is active.
func (r *RIPRenderer) IsFullScreen() bool {
	return r.fullScreen
}

// SetFullScreen toggles full-screen RIP mode.
func (r *RIPRenderer) SetFullScreen(on bool) {
	r.fullScreen = on
	if on {
		// Switch to full-screen graphics mode
		r.writer("\033[=3h") // Set screen mode
	} else {
		// Return to text mode
		r.writer("\033[=7h")
	}
}

// Render parses and renders a RIPscrip sequence from output.
func (r *RIPRenderer) Render(sequence string) string {
	if !r.enabled {
		return sequence
	}

	var result strings.Builder
	src := []byte(sequence)
	i := 0

	for i < len(src) {
		// Look for RIP escape: ! (0x21)
		if src[i] == '!' && i+2 < len(src) {
			// Found potential RIP command
			cmd := src[i+1]
			// Find the terminator ~
			end := i + 2
			for end < len(src) && src[end] != '~' && end < i+256 {
				end++
			}
			if end < len(src) && src[end] == '~' {
				params := string(src[i+2 : end])
				r.executeCommand(cmd, params)
				i = end + 1
				continue
			}
		}
		result.WriteByte(src[i])
		i++
	}

	return result.String()
}

// executeCommand processes a single RIPscrip command.
func (r *RIPRenderer) executeCommand(cmd byte, params string) {
	switch cmd {
	case 'c': // Reset
		r.cmdReset(params)
	case 'C': // Text color
		r.cmdTextColor(params)
	case 'Q': // Define mouse region
		r.cmdDefineMouseRegion(params)
	case 'M': // Move cursor
		r.cmdMoveCursor(params)
	case 'T': // Text
		r.cmdText(params)
	case 'D': // Draw line
		r.cmdDrawLine(params)
	case 'R': // Draw rectangle
		r.cmdDrawRect(params)
	case 'B': // Draw box
		r.cmdDrawBox(params)
	case 'I': // Icon/Image
		r.cmdDrawIcon(params)
	case 'F': // Font selection
		r.cmdSetFont(params)
	case 'W': // Window size
		r.cmdSetWindow(params)
	case 'P': // Pixel
		r.cmdDrawPixel(params)
	case 'L': // Line drawing
		r.cmdLineDraw(params)
	}
}

// cmdReset resets the RIP environment.
func (r *RIPRenderer) cmdReset(params string) {
	// Clear mouse regions
	r.mouseRegions = make([]MouseRegion, 0)
	// Reset colors to default
	r.writer("\033[0m")
}

// cmdTextColor sets text color for RIP mode.
func (r *RIPRenderer) cmdTextColor(params string) {
	if len(params) >= 2 {
		fg := params[0]
		bg := params[1]
		// Map RIP colors to ANSI
		ansiFg := ripColorToANSI(fg)
		ansiBg := ripBgColorToANSI(bg)
		r.writer(fmt.Sprintf("\033[%d;%dm", ansiFg, ansiBg))
	}
}

// cmdDefineMouseRegion creates a clickable region.
func (r *RIPRenderer) cmdDefineMouseRegion(params string) {
	// Parse: region# x1 y1 x2 y2 hotkey
	var region MouseRegion
	fmt.Sscanf(params, "%d %d %d %d %d %c",
		&region.RegionNum, &region.X1, &region.Y1,
		&region.X2, &region.Y2, &region.HotKey)
	r.mouseRegions = append(r.mouseRegions, region)
}

// cmdMoveCursor positions the cursor.
func (r *RIPRenderer) cmdMoveCursor(params string) {
	var x, y int
	fmt.Sscanf(params, "%d %d", &x, &y)
	r.writer(fmt.Sprintf("\033[%d;%dH", y+1, x+1))
}

// cmdText outputs text at current position.
func (r *RIPRenderer) cmdText(params string) {
	r.writer(params)
}

// cmdDrawLine draws a line between two points.
func (r *RIPRenderer) cmdDrawLine(params string) {
	var x1, y1, x2, y2 int
	fmt.Sscanf(params, "%d %d %d %d", &x1, &y1, &x2, &y2)
	// Use ANSI line-drawing characters
	r.drawANSILine(x1, y1, x2, y2)
}

// cmdDrawRect draws a rectangle outline.
func (r *RIPRenderer) cmdDrawRect(params string) {
	var x1, y1, x2, y2 int
	fmt.Sscanf(params, "%d %d %d %d", &x1, &y1, &x2, &y2)
	r.drawANSIBox(x1, y1, x2, y2)
}

// cmdDrawBox draws a filled box.
func (r *RIPRenderer) cmdDrawBox(params string) {
	var x1, y1, x2, y2 int
	fmt.Sscanf(params, "%d %d %d %d", &x1, &y1, &x2, &y2)
	r.drawANSIBox(x1, y1, x2, y2)
}

// cmdDrawIcon draws a RIP icon (converted to ANSI approximation).
func (r *RIPRenderer) cmdDrawIcon(params string) {
	// RIP icons are specific graphical symbols
	// We approximate them with ASCII art
	var iconNum int
	fmt.Sscanf(params, "%d", &iconNum)
	// Output a placeholder - real RIP icons would be graphical
	r.writer("[Icon]")
}

// cmdSetFont selects a RIP font.
func (r *RIPRenderer) cmdSetFont(params string) {
	// RIP font commands - we send the escape sequence
	// In ANSI mode, we approximate with character set changes
	var fontNum int
	fmt.Sscanf(params, "%d", &fontNum)
	switch fontNum {
	case 0: // Standard font
		r.writer("\033(B") // ASCII character set
	case 1: // Line drawing
		r.writer("\033(0") // Line drawing character set
	}
}

// cmdSetWindow sets the RIP window dimensions.
func (r *RIPRenderer) cmdSetWindow(params string) {
	var x1, y1, x2, y2 int
	fmt.Sscanf(params, "%d %d %d %d", &x1, &y1, &x2, &y2)
	r.width = x2 - x1 + 1
	r.height = y2 - y1 + 1
}

// cmdDrawPixel draws a single pixel (approximated in ANSI).
func (r *RIPRenderer) cmdDrawPixel(params string) {
	var x, y int
	fmt.Sscanf(params, "%d %d", &x, &y)
	r.writer(fmt.Sprintf("\033[%d;%dH·", y+1, x+1))
}

// cmdLineDraw draws lines using RIP line-drawing commands.
func (r *RIPRenderer) cmdLineDraw(params string) {
	// RIP line drawing uses specific codes for line segments
	// Convert to ANSI line-drawing characters
	var direction, length int
	fmt.Sscanf(params, "%d %d", &direction, &length)
	for i := 0; i < length; i++ {
		switch direction {
		case 0: // Right
			r.writer("q")
		case 1: // Down
			r.writer("x")
		case 2: // Left
			r.writer("q")
		case 3: // Up
			r.writer("x")
		}
	}
}

// drawANSILine draws a line using ANSI line-drawing characters.
func (r *RIPRenderer) drawANSILine(x1, y1, x2, y2 int) {
	r.writer("\033(0") // Enable line drawing
	defer r.writer("\033(B") // Reset to ASCII

	if y1 == y2 {
		// Horizontal line
		r.writer(fmt.Sprintf("\033[%d;%dH", y1+1, x1+1))
		for i := x1; i <= x2; i++ {
			r.writer("q") // ─
		}
	} else if x1 == x2 {
		// Vertical line
		for i := y1; i <= y2; i++ {
			r.writer(fmt.Sprintf("\033[%d;%dH", i+1, x1+1))
			r.writer("x") // │
		}
	} else {
		// Diagonal line - approximate with dots
		for i := 0; i <= max(abs(x2-x1), abs(y2-y1)); i++ {
			px := x1 + i*(x2-x1)/max(abs(x2-x1), abs(y2-y1))
			py := y1 + i*(y2-y1)/max(abs(x2-x1), abs(y2-y1))
			r.writer(fmt.Sprintf("\033[%d;%dH·", py+1, px+1))
		}
	}
}

// drawANSIBox draws a box using ANSI line-drawing characters.
func (r *RIPRenderer) drawANSIBox(x1, y1, x2, y2 int) {
	r.writer("\033(0") // Enable line drawing
	defer r.writer("\033(B") // Reset to ASCII

	// Top border
	r.writer(fmt.Sprintf("\033[%d;%dH", y1+1, x1+1))
	r.writer("l") // ┌
	for i := x1 + 1; i < x2; i++ {
		r.writer("q") // ─
	}
	r.writer("k") // ┐

	// Middle rows
	for row := y1 + 1; row < y2; row++ {
		r.writer(fmt.Sprintf("\033[%d;%dH", row+1, x1+1))
		r.writer("x") // │
		r.writer(fmt.Sprintf("\033[%d;%dH", row+1, x2+1))
		r.writer("x") // │
	}

	// Bottom border
	r.writer(fmt.Sprintf("\033[%d;%dH", y2+1, x1+1))
	r.writer("m") // └
	for i := x1 + 1; i < x2; i++ {
		r.writer("q") // ─
	}
	r.writer("j") // ┘
}

// ripColorToANSI converts a RIP color code to an ANSI color number.
func ripColorToANSI(c byte) int {
	colorMap := map[byte]int{
		'0': 30, '1': 34, '2': 32, '3': 36,
		'4': 31, '5': 35, '6': 33, '7': 37,
		'8': 90, '9': 94, 'A': 92, 'B': 96,
		'C': 91, 'D': 95, 'E': 93, 'F': 97,
	}
	if ansi, ok := colorMap[c]; ok {
		return ansi
	}
	return 37 // Default white
}

// ripBgColorToANSI converts a RIP background color to ANSI.
func ripBgColorToANSI(c byte) int {
	colorMap := map[byte]int{
		'0': 40, '1': 44, '2': 42, '3': 46,
		'4': 41, '5': 45, '6': 43, '7': 47,
		'8': 100, '9': 104, 'A': 102, 'B': 106,
		'C': 101, 'D': 105, 'E': 103, 'F': 107,
	}
	if ansi, ok := colorMap[c]; ok {
		return ansi
	}
	return 40 // Default black
}

// Helper functions
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// GetMouseRegions returns the current mouse regions.
func (r *RIPRenderer) GetMouseRegions() []MouseRegion {
	return r.mouseRegions
}

// ClearMouseRegions removes all defined mouse regions.
func (r *RIPRenderer) ClearMouseRegions() {
	r.mouseRegions = make([]MouseRegion, 0)
}

// SendFontCommand sends a RIP font command to the client.
func (r *RIPRenderer) SendFontCommand(fontNum int) {
	if !r.enabled {
		return
	}
	// RIP font commands are sent as escape sequences
	r.writer(fmt.Sprintf("!F%02d~", fontNum))
}
