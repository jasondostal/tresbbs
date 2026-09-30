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

// Telnet command bytes (RFC 854/857/858). Used to negotiate character-at-a-time
// mode with server echo — the mode a BBS needs so single-key menus fire on the
// keypress instead of waiting for Enter.
const (
	telnetIAC  = 255 // Interpret As Command
	telnetSE   = 240 // End of subnegotiation
	telnetSB   = 250 // Begin subnegotiation
	telnetWILL = 251
	telnetWONT = 252
	telnetDO   = 253
	telnetDONT = 254
	telnetEcho = 1 // option: ECHO
	telnetSGA  = 3 // option: SUPPRESS-GO-AHEAD
)

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

	// localEcho makes ReadLine echo typed characters. Off by default. It is
	// turned on when the server owns echo: telnet after we negotiate WILL ECHO
	// (character-at-a-time mode), SSH (raw PTY, no client echo), and the local
	// sysop console's Local Logon.
	localEcho bool

	// eatNextLF swallows the LF/NUL that trails a CR, so a CR, CR LF, or CR NUL
	// line ending all look identical to callers regardless of client. Set when
	// a read consumes a CR; honored (once) by the very next input read.
	eatNextLF bool
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

// readByte reads one data byte from the client, transparently consuming any
// telnet IAC command sequence and swallowing the LF/NUL that trails a CR. This
// is the single input primitive every reader is built on, so CR / CR LF / CR
// NUL line endings — and stray negotiation traffic — look identical to callers
// no matter which client (macOS telnet, PuTTY, SyncTERM, netcat, SSH) is on the
// other end.
func (d *Display) readByte() (byte, error) {
	for {
		b, err := d.reader.ReadByte()
		if err != nil {
			return 0, err
		}
		if b == telnetIAC {
			d.skipTelnetCommand()
			continue
		}
		if d.eatNextLF {
			d.eatNextLF = false
			if b == '\n' || b == 0 {
				continue // trailing byte of a CR LF / CR NUL pair
			}
		}
		return b, nil
	}
}

// ReadLine implements DisplayPort.
func (d *Display) ReadLine() (string, error) {
	var line []byte
	for {
		b, err := d.readByte()
		if err != nil {
			if len(line) > 0 {
				return string(line), nil
			}
			return "", err
		}

		// Any CR or LF ends the line. On CR, swallow a following LF/NUL so a
		// CRLF (or CR NUL) client doesn't leave a spurious empty line behind.
		if b == '\r' || b == '\n' {
			d.eatNextLF = b == '\r'
			if d.localEcho {
				d.write("\r\n")
			}
			return string(line), nil
		}

		// Stray NUL (telnet filler) — ignore.
		if b == 0 {
			continue
		}

		// Backspace / delete.
		if b == 127 || b == 8 {
			if len(line) > 0 {
				line = line[:len(line)-1]
				d.write("\b \b")
			}
			continue
		}

		// Ctrl-C cancels.
		if b == 3 {
			return "", nil
		}

		// Normal character.
		line = append(line, b)
		if d.localEcho {
			d.write(string(b))
		}
	}
}

// ReadKey implements DisplayPort. It reads a single keystroke for instant menu
// dispatch, transparently swallowing telnet IAC sequences and CR-pair trailers
// (a client's negotiation replies or a stray LF must not read as a keypress).
//
// After the key, it drains any line terminator the client sent along with it.
// A line-mode client (raw netcat, or a telnet client that refused char mode)
// transmits a menu selection as "K\n" or "K\r\n" — without this, that trailing
// newline leaks into the NEXT prompt and, e.g., instantly aborts the comment
// editor with an empty first line. The drain is non-blocking (it only discards
// bytes already buffered), so a char-mode client that sends a bare "K" is
// unaffected.
func (d *Display) ReadKey() (byte, error) {
	b, err := d.readByte()
	if err != nil {
		return 0, err
	}
	if b == '\r' {
		d.eatNextLF = true // swallow a following LF/NUL on the next read
	}
	d.drainBufferedEOL()
	return b, nil
}

// drainBufferedEOL discards CR/LF/NUL bytes that are ALREADY buffered, without
// reading from the connection (so it never blocks waiting on a char-mode client
// that sent no terminator). Used after a single-key menu read to absorb a
// line-mode client's trailing newline.
func (d *Display) drainBufferedEOL() {
	for d.reader.Buffered() > 0 {
		p, err := d.reader.Peek(1)
		if err != nil || (p[0] != '\r' && p[0] != '\n' && p[0] != 0) {
			return
		}
		d.reader.ReadByte()
		d.eatNextLF = false
	}
}

// skipTelnetCommand consumes a telnet IAC command sequence after the leading
// IAC (255) byte has already been read. WILL/WONT/DO/DONT carry one option
// byte; SB…SE is variable-length; a doubled IAC is an escaped literal 255.
func (d *Display) skipTelnetCommand() {
	cmd, err := d.reader.ReadByte()
	if err != nil {
		return
	}
	switch cmd {
	case telnetWILL, telnetWONT, telnetDO, telnetDONT:
		d.reader.ReadByte() // consume the option byte
	case telnetSB:
		for { // read until IAC SE
			b, err := d.reader.ReadByte()
			if err != nil {
				return
			}
			if b == telnetIAC {
				se, err := d.reader.ReadByte()
				if err != nil || se == telnetSE {
					return
				}
			}
		}
	}
	// Any other command (NOP, doubled-IAC literal, etc.) is a bare 2-byte
	// sequence we've now fully consumed.
}

// NegotiateTelnet puts a telnet client into character-at-a-time mode with
// server-side echo — the mode a BBS needs for instant single-key menus. It
// sends WILL ECHO + WILL/DO SUPPRESS-GO-AHEAD and switches on local (server)
// echo so typed input still shows. Call once, right after the telnet
// connection is accepted and before any prompt. SSH is already char-mode and
// uses SetLocalEcho(true) directly instead.
func (d *Display) NegotiateTelnet() {
	d.write(string([]byte{
		telnetIAC, telnetWILL, telnetEcho,
		telnetIAC, telnetWILL, telnetSGA,
		telnetIAC, telnetDO, telnetSGA,
	}))
	d.localEcho = true
}

// ReadPassword implements DisplayPort — reads input with echo disabled.
func (d *Display) ReadPassword() (string, error) {
	d.write(HideCursor)
	defer d.write(ShowCursor)

	var password []byte
	for {
		b, err := d.readByte()
		if err != nil {
			return "", err
		}
		switch b {
		case '\r', '\n':
			d.eatNextLF = b == '\r'
			d.write("\r\n")
			return string(password), nil
		case 0: // stray NUL — ignore
			continue
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
