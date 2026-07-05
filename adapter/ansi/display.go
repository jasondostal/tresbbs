// Package ansi implements the DisplayPort using ANSI escape sequences.
//
// This adapter works over any byte stream — SSH, Telnet, or a local terminal.
// It handles the TriBBS color system (@X__ codes → ANSI colors) and provides
// the terminal I/O primitives that the BBS session loop needs.
package ansi

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/jasondostal/tresbbs/internal/ui"

	"github.com/jasondostal/tresbbs/port"
)

// Ensure we implement the port interface.
var _ port.DisplayPort = (*Display)(nil)

// ANSI escape sequences
const (
	ClearScreen = "\033[2J\033[H"
	CursorHome  = "\033[H"
	HideCursor  = "\033[?25l"
	ShowCursor  = "\033[?25h"
	Reset       = "\033[0m"
	Bold        = "\033[1m"
	Reverse     = "\033[7m"
)

// TriBBS color code → ANSI escape mapping.
// These match the original TriBBS @X__ system exactly.
var colorMap = map[byte]string{
	'A': "\033[92m", // Light Green  @X0A — labels, headers
	'B': "\033[96m", // Light Cyan   @X0B — secondary text
	'C': "\033[91m", // Light Red    @X0C — warnings, errors
	'D': "\033[95m", // Light Magenta @X0D — special emphasis
	'E': "\033[93m", // Yellow       @X0E — values, active items
	'F': "\033[97m", // White        @X0F — default body
}

// Display implements DisplayPort for ANSI terminals.
type Display struct {
	reader *bufio.Reader
	writer io.Writer
	width  int
	height int
	ripRenderer *RIPRenderer

	// localEcho makes ReadLine echo typed characters. Off by default: over
	// telnet/SSH the client echoes. It is turned on for a local (raw-terminal)
	// session — e.g. the sysop console's Local Logon — where nothing else does.
	localEcho bool
}

// New creates a new ANSI display adapter.
func New(r io.Reader, w io.Writer) *Display {
	d := &Display{
		reader: bufio.NewReader(r),
		writer: w,
		width:  80,  // default, can be updated
		height: 24,
	}
	d.ripRenderer = NewRIPRenderer(func(s string) { d.write(s) })
	return d
}

// SetSize updates the terminal dimensions.
func (d *Display) SetSize(width, height int) {
	d.width = width
	d.height = height
}

// SetLocalEcho enables echoing of typed characters in ReadLine, for a local
// raw-mode session where the terminal itself does not echo.
func (d *Display) SetLocalEcho(on bool) {
	d.localEcho = on
}

// Clear implements DisplayPort.
func (d *Display) Clear() {
	d.write(ClearScreen)
}

// MoveTo implements DisplayPort.
func (d *Display) MoveTo(row, col int) {
	d.write(fmt.Sprintf("\033[%d;%dH", row+1, col+1))
}

// Write implements DisplayPort.
func (d *Display) Write(text string) {
	d.write(d.RenderOutput(text))
}

// WriteLine implements DisplayPort.
func (d *Display) WriteLine(text string) {
	d.write(d.RenderOutput(ui.NormalizeBox(text)) + "\r\n")
}

// SetColor implements DisplayPort.
func (d *Display) SetColor(color byte) {
	if ansi, ok := colorMap[color]; ok {
		d.write(ansi)
	}
}

// ResetColor implements DisplayPort.
func (d *Display) ResetColor() {
	d.write(Reset)
}

// SetReverse implements DisplayPort.
func (d *Display) SetReverse(on bool) {
	if on {
		d.write(Reverse)
	} else {
		d.write(Reset)
	}
}

// ReadLine implements DisplayPort.
func (d *Display) ReadLine() (string, error) {
	var line []byte
	for {
		b, err := d.reader.ReadByte()
		if err != nil {
			if len(line) > 0 {
				return string(line), nil
			}
			return "", err
		}

		// Skip telnet IAC sequences (255 followed by command)
		if b == 255 {
			// Read the next two bytes (command + option)
			d.reader.ReadByte()
			d.reader.ReadByte()
			continue
		}

		// Handle carriage return (telnet sends \r\n or \r\0)
		if b == '\r' {
			// Read the next byte - could be \n or \0 or nothing
			next, err := d.reader.ReadByte()
			if err == nil && next != '\n' && next != 0 {
				// Not a line ending sequence, put it back
				line = append(line, b)
				line = append(line, next)
				if d.localEcho {
					d.write(string([]byte{b, next}))
				}
				continue
			}
			// It's a line ending — return the line (empty string on a blank
			// line, so callers see the Enter instead of blocking for more input).
			if d.localEcho {
				d.write("\r\n")
			}
			return string(line), nil
		}

		// Handle bare newline
		if b == '\n' {
			if d.localEcho {
				d.write("\r\n")
			}
			return string(line), nil
		}

		// Handle backspace
		if b == 127 || b == 8 {
			if len(line) > 0 {
				line = line[:len(line)-1]
				d.write("\b \b")
			}
			continue
		}

		// Handle Ctrl-C (cancel)
		if b == 3 {
			return "", nil
		}

		// Normal character
		line = append(line, b)
		if d.localEcho {
			d.write(string(b))
		}
	}
}

// ReadKey implements DisplayPort.
func (d *Display) ReadKey() (byte, error) {
	b, err := d.reader.ReadByte()
	if err != nil {
		return 0, err
	}
	return b, nil
}

// ReadPassword implements DisplayPort — reads input with echo disabled.
func (d *Display) ReadPassword() (string, error) {
	d.write(HideCursor)
	defer d.write(ShowCursor)

	var password []byte
	for {
		b, err := d.reader.ReadByte()
		if err != nil {
			return "", err
		}
		switch b {
		case '\r', '\n':
			d.write("\r\n")
			return string(password), nil
		case 127, 8: // backspace
			if len(password) > 0 {
				password = password[:len(password)-1]
				d.write("\b \b")
			}
		default:
			password = append(password, b)
			d.write("*")
		}
	}
}

// ShowCursor implements DisplayPort.
func (d *Display) ShowCursor(show bool) {
	if show {
		d.write(ShowCursor)
	} else {
		d.write(HideCursor)
	}
}

// Flush implements DisplayPort.
func (d *Display) Flush() {
	// bufio.Writer would handle this, but we're using raw io.Writer
	// In practice, the SSH/Telnet adapter handles flushing
}

// Width implements DisplayPort.
func (d *Display) Width() int {
	return d.width
}

// Height implements DisplayPort.
func (d *Display) Height() int {
	return d.height
}

// DrawBox implements DisplayPort — draws a box with line-drawing characters.
func (d *Display) DrawBox(row, col, width, height int) {
	// Use ANSI line-drawing characters
	// Top border
	// Enable line-drawing mode
	d.write("\033(0") // Switch to line-drawing character set
	defer d.write("\033(B") // Switch back to ASCII

	// Top-left corner
	// Top border
	// Top-right corner
	// Bottom-left corner
	// Bottom border
	// Bottom-right corner
	// Left border
	// Right border

	// Top row
	d.MoveTo(row, col)
		d.write("l") // ┌
	for i := 0; i < width-2; i++ {
		d.write("q") // ─
	}
	d.write("k") // ┐

	// Middle rows
	for r := row + 1; r < row+height-1; r++ {
		d.MoveTo(r, col)
		d.write("x") // │
		for i := 0; i < width-2; i++ {
			d.write(" ")
		}
		d.write("x") // │
	}

	// Bottom row
	d.MoveTo(row+height-1, col)
	d.write("m") // └
	for i := 0; i < width-2; i++ {
		d.write("q") // ─
	}
	d.write("j") // ┘
}

// SetScrollRegion implements DisplayPort.
func (d *Display) SetScrollRegion(top, bottom int) {
	d.write(fmt.Sprintf("\033[%d;%dr", top+1, bottom+1))
}

// SetRIPMode enables or disables RIPscrip rendering for this display.
func (d *Display) SetRIPMode(enabled bool) {
	if enabled {
		d.ripRenderer.Enable()
	} else {
		d.ripRenderer.Disable()
	}
}

// GetRIPRenderer returns the RIP renderer for this display.
func (d *Display) GetRIPRenderer() *RIPRenderer {
	return d.ripRenderer
}

// RenderOutput processes output text, handling RIP sequences if enabled.
// When user ANSI mode is 2 (RIPscrip), this renders RIP sequences.
func (d *Display) RenderOutput(text string) string {
	if d.ripRenderer.IsEnabled() {
		return d.ripRenderer.Render(text)
	}
	return text
}

// write is a helper that writes and ignores errors.
func (d *Display) write(s string) {
	d.writer.Write([]byte(s))
}

// Render processes a TriBBS template string, converting @X__ color codes to
// ANSI escape sequences. This is the display-layer counterpart to the
// domain template engine.
func (d *Display) Render(template string) string {
	var result strings.Builder
	src := []byte(template)
	i := 0

	for i < len(src) {
		if src[i] == '@' && i+2 < len(src) && src[i+1] == 'X' {
			// Color code: @X__
			colorCode := src[i+2]
			if ansi, ok := colorMap[colorCode]; ok {
				result.WriteString(ansi)
			}
			i += 3
		} else {
			result.WriteByte(src[i])
			i++
		}
	}

	return result.String()
}
