package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFindMenuDisplayResolution pins TriBBS's custom-menu lookup: an exact
// security-level file beats <PREFIX>ALL, the graphics mode picks the extension,
// and RIP callers fall back through ANS to BBS.
func TestFindMenuDisplayResolution(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("MAINALL.BBS")
	write("MAIN10.ANS")

	disp := &captureDisplay{}
	s := newTestSession(disp, 10)
	s.menuDir = dir
	s.user.ANSIMode = 1 // ANSI

	// Exact-level ANS beats the ALL fallback.
	if got := s.findMenuDisplay("MAIN"); filepath.Base(got) != "MAIN10.ANS" {
		t.Errorf("ANSI sec-10 resolved %q, want MAIN10.ANS", got)
	}

	// Remove the exact file; falls back to MAINALL.BBS.
	os.Remove(filepath.Join(dir, "MAIN10.ANS"))
	if got := s.findMenuDisplay("MAIN"); filepath.Base(got) != "MAINALL.BBS" {
		t.Errorf("fallback resolved %q, want MAINALL.BBS", got)
	}

	// A plain-ASCII caller never picks an .ANS file.
	write("MAIN10.ANS")
	s.user.ANSIMode = 0
	if got := s.findMenuDisplay("MAIN"); filepath.Base(got) != "MAINALL.BBS" {
		t.Errorf("ASCII caller resolved %q, want MAINALL.BBS", got)
	}

	// No configured menu dir => never any custom screen.
	s.menuDir = ""
	if got := s.findMenuDisplay("MAIN"); got != "" {
		t.Errorf("no menuDir should yield no display file, got %q", got)
	}
}

// TestShowMenuUsesCustomArt verifies a custom display screen overrides the
// auto-drawn box and has its @VARIABLES substituted.
func TestShowMenuUsesCustomArt(t *testing.T) {
	dir := t.TempDir()
	art := "@BOARDNAME Custom Main Screen\r\n@USER is online.\r\n"
	if err := os.WriteFile(filepath.Join(dir, "MAINALL.ANS"), []byte(art), 0644); err != nil {
		t.Fatal(err)
	}
	disp := &captureDisplay{keys: []byte{'G'}}
	s := newTestSession(disp, 10)
	s.menuDir = dir

	s.showMenu("MAIN.MNU")
	out := disp.out()
	if !strings.Contains(out, "JasonAndOpus Custom Main Screen") {
		t.Error("custom art should render with @BOARDNAME substituted")
	}
	if !strings.Contains(out, "Jason Dostal is online.") {
		t.Error("custom art should render with @USER substituted")
	}
	if strings.Contains(out, "<B>..Bulletin Menu") {
		t.Error("custom art should replace the auto-drawn menu box")
	}
	// The prompt is still built from the .MNU regardless of custom art.
	if !strings.Contains(out, "Enter Selection - [B M F D T C A N Y I U W X P G ?]? ") {
		t.Error("prompt should still come from the .MNU under custom art")
	}
}
