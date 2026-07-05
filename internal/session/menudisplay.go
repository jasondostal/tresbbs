package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Custom menu display screens (TriBBS parity).
//
// TriBBS normally auto-draws a menu from its .MNU, but a sysop can override any
// menu with a custom art screen. For the Main menu those are MAINn.RIP/.ANS/.BBS
// (n = the caller's security level) with MAINALL.* as the fallback; the same
// pattern applies to the Message (MESS), File (FILE), Sysop (SYSOP), and Door
// (DOOR) menus. The extension is chosen by the caller's graphics mode. tresbbs
// looks for these files in the same directory as the .MNU files (the dropped-in
// NWORK/), so a sysop's existing custom menus render unchanged.

// menuDisplayPrefix maps a .MNU logical name to its custom display-file prefix.
var menuDisplayPrefix = map[string]string{
	"MAIN":    "MAIN",
	"MESSAGE": "MESS",
	"FILES":   "FILE",
	"SYSOP":   "SYSOP",
	"DOORS":   "DOOR",
}

// graphicsExts returns the display-file extensions to try, in priority order,
// for the caller's graphics mode: RIP callers fall back to ANSI then ASCII;
// ANSI callers fall back to ASCII; ASCII callers get only .BBS.
func graphicsExts(ansiMode int) []string {
	switch ansiMode {
	case 2: // RIPscrip
		return []string{"RIP", "ANS", "BBS"}
	case 1: // ANSI
		return []string{"ANS", "BBS"}
	default: // plain ASCII
		return []string{"BBS"}
	}
}

// findMenuDisplay returns the path to a custom display screen for the named menu
// and the current caller, following TriBBS's "<PREFIX><security>" then
// "<PREFIX>ALL" lookup across the caller's graphics extensions. Returns "" when
// no menu directory is configured or no matching file exists (auto-draw the box).
func (s *Session) findMenuDisplay(menuName string) string {
	if s.menuDir == "" {
		return ""
	}
	prefix, ok := menuDisplayPrefix[strings.ToUpper(menuName)]
	if !ok {
		return ""
	}
	mode := 0
	if s.user != nil {
		mode = s.user.ANSIMode
	}
	exts := graphicsExts(mode)
	bases := []string{fmt.Sprintf("%s%d", prefix, s.userSecurity()), prefix + "ALL"}
	for _, base := range bases {
		for _, ext := range exts {
			p := filepath.Join(s.menuDir, base+"."+ext)
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
				return p
			}
		}
	}
	return ""
}

// renderDisplayFile renders a custom menu/display screen: its @VARIABLE and @X
// color tokens are substituted through the template engine, then written with
// the same flow-control handling as any template. Returns false if the file
// can't be read.
func (s *Session) renderDisplayFile(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	if s.user != nil {
		s.templateData.User = s.user
	}
	var b strings.Builder
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		b.WriteString(s.templateEngine.RenderLine(line, s.templateData))
		b.WriteString("\r\n")
	}
	s.writeTemplate(b.String())
	return true
}
