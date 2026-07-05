// Package menu implements TriBBS .MNU file parsing and rendering.
//
// TriBBS drove every command menu from a plain-text .MNU file in the board's
// NWORK/ directory (MAIN.MNU, FILES.MNU, MESSAGE.MNU, SYSOP.MNU, ...). The BBS
// auto-drew the on-screen menu box from that file — colors, items, and the
// command prompt all came from the .MNU plus the caller's security level.
//
// tresbbs reads the SAME files, unmodified, so a sysop can drop their existing
// TriBBS NWORK/ directory in and the menus render exactly as they did under
// the original TriBBS. This package is that reader/renderer.
//
// File format (verified against real 5.01 board files):
//
//	borderBg,borderFg,panelBg,panelFg      <- color header (line 1), DOS attrs
//	key,command,description,security       <- one item per line
//	...
//
// e.g. MAIN.MNU begins "6,0,1,15" (brown border, blue panel, white text) then
// "B,B,Bulletin Menu,10" (press B -> command B -> "Bulletin Menu", security 10).
package menu

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Item is one selectable command in a menu.
type Item struct {
	Key         byte   // key the caller presses (upper-cased)
	Command     string // action identifier (built-in command or door ref)
	Description string // label shown in the box
	Security    int    // minimum security level to see/use the item
}

// Menu is a parsed .MNU file: a color scheme plus an ordered item list.
type Menu struct {
	Name  string // logical name, e.g. "MAIN" (from the filename, sans .MNU)
	Title string // display title, e.g. "Main Menu"

	// Color header (DOS attribute indices straight from the file).
	BorderBg int
	BorderFg int
	PanelBg  int
	PanelFg  int

	Items []Item
}

// menuTitles maps a .MNU logical name to the title TriBBS shows in the box.
// The board name is prepended at render time ("JasonAndOpus Main Menu").
var menuTitles = map[string]string{
	"MAIN":    "Main Menu",
	"FILES":   "File Menu",
	"MESSAGE": "Message Menu",
	"SYSOP":   "Sysop Menu",
	"DOORS":   "Door Menu",
}

// TitleFor returns the display title for a .MNU logical name.
func TitleFor(name string) string {
	if t, ok := menuTitles[strings.ToUpper(name)]; ok {
		return t
	}
	return strings.Title(strings.ToLower(name)) + " Menu"
}

// Load reads and parses a .MNU file. The logical name is the file's base name
// with the .MNU extension removed (e.g. "MAIN.MNU" -> "MAIN").
func Load(path string) (*Menu, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	base := path
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	name := strings.TrimSuffix(strings.ToUpper(base), ".MNU")
	return Parse(name, f)
}

// Parse reads a .MNU stream. name is the logical menu name (e.g. "MAIN").
//
// The first non-blank line is the color header (4 comma-separated DOS attribute
// indices). Every following non-blank line is an item: key,command,desc,security.
// Trailing CRs (the files are DOS/CRLF) and surrounding whitespace are trimmed.
func Parse(name string, r io.Reader) (*Menu, error) {
	m := &Menu{Name: strings.ToUpper(name), Title: TitleFor(name)}
	sc := bufio.NewScanner(r)
	haveHeader := false

	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r\n")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !haveHeader {
			if err := m.parseHeader(line); err != nil {
				return nil, err
			}
			haveHeader = true
			continue
		}
		if it, ok := parseItem(line); ok {
			m.Items = append(m.Items, it)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if !haveHeader {
		return nil, fmt.Errorf("menu %s: empty or missing color header", name)
	}
	return m, nil
}

func (m *Menu) parseHeader(line string) error {
	f := strings.Split(line, ",")
	if len(f) < 4 {
		return fmt.Errorf("menu %s: bad color header %q", m.Name, line)
	}
	vals := make([]int, 4)
	for i := 0; i < 4; i++ {
		n, err := strconv.Atoi(strings.TrimSpace(f[i]))
		if err != nil {
			return fmt.Errorf("menu %s: bad color value %q: %w", m.Name, f[i], err)
		}
		vals[i] = n
	}
	m.BorderBg, m.BorderFg, m.PanelBg, m.PanelFg = vals[0], vals[1], vals[2], vals[3]
	return nil
}

// parseItem parses "key,command,description,security". Malformed lines are
// dropped (ok=false) rather than aborting the whole menu, matching TriBBS's
// tolerant loader.
func parseItem(line string) (Item, bool) {
	f := strings.SplitN(line, ",", 4)
	if len(f) < 4 {
		return Item{}, false
	}
	key := strings.TrimSpace(f[0])
	if key == "" {
		return Item{}, false
	}
	sec, err := strconv.Atoi(strings.TrimSpace(f[3]))
	if err != nil {
		return Item{}, false
	}
	return Item{
		Key:         upper(key[0]),
		Command:     strings.TrimSpace(f[1]),
		Description: strings.TrimSpace(f[2]),
		Security:    sec,
	}, true
}

// VisibleItems returns the items a caller at the given security level may see,
// in file order. This is what drives both the rendered box and the prompt.
func (m *Menu) VisibleItems(security int) []Item {
	var out []Item
	for _, it := range m.Items {
		if security >= it.Security {
			out = append(out, it)
		}
	}
	return out
}

// PromptKeys returns the space-separated key list for the command prompt,
// e.g. "B M F D T C A N Y I U W X P G" — the visible items' keys, in order.
func (m *Menu) PromptKeys(security int) string {
	var b strings.Builder
	for _, it := range m.VisibleItems(security) {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteByte(it.Key)
	}
	return b.String()
}

// Find returns the item matching a pressed key (case-insensitive), if the
// caller's security level permits it.
func (m *Menu) Find(key byte, security int) (Item, bool) {
	key = upper(key)
	for _, it := range m.Items {
		if it.Key == key && security >= it.Security {
			return it, true
		}
	}
	return Item{}, false
}

func upper(b byte) byte {
	if b >= 'a' && b <= 'z' {
		return b - 32
	}
	return b
}
