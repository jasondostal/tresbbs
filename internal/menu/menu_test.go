package menu

import (
	"path/filepath"
	"strings"
	"testing"
)

func loadTestMenu(t *testing.T, name string) *Menu {
	t.Helper()
	m, err := Load(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("Load(%s): %v", name, err)
	}
	return m
}

// TestParseMainHeader pins the MAIN.MNU color header to the real file's values,
// which are exactly the colors in the reference screenshot (brown border, blue
// panel, white text).
func TestParseMainHeader(t *testing.T) {
	m := loadTestMenu(t, "MAIN.MNU")
	if m.Name != "MAIN" || m.Title != "Main Menu" {
		t.Errorf("name/title = %q/%q, want MAIN/Main Menu", m.Name, m.Title)
	}
	if m.BorderBg != 6 || m.BorderFg != 0 || m.PanelBg != 1 || m.PanelFg != 15 {
		t.Errorf("colors = %d,%d,%d,%d; want 6,0,1,15",
			m.BorderBg, m.BorderFg, m.PanelBg, m.PanelFg)
	}
}

// TestParseItems checks the item field order: key,command,description,security.
func TestParseItems(t *testing.T) {
	m := loadTestMenu(t, "MAIN.MNU")
	if len(m.Items) != 18 {
		t.Fatalf("got %d items, want 18", len(m.Items))
	}
	first := m.Items[0]
	if first.Key != 'B' || first.Command != "B" || first.Description != "Bulletin Menu" || first.Security != 10 {
		t.Errorf("first item = %+v; want B/B/Bulletin Menu/10", first)
	}
	// The three items a sec-10 caller can't see, by their real securities.
	sec := map[byte]int{}
	for _, it := range m.Items {
		sec[it.Key] = it.Security
	}
	for k, want := range map[byte]int{'Q': 999, 'S': 100, 'R': 999, 'G': 0} {
		if sec[k] != want {
			t.Errorf("item %c security = %d, want %d", k, sec[k], want)
		}
	}
}

// TestPromptKeysMatchesScreenshot is the keystone parity test: a security-10
// caller's MAIN menu prompt must be exactly the keys from the reference
// screenshot — the full item list minus the three items above his level.
func TestPromptKeysMatchesScreenshot(t *testing.T) {
	m := loadTestMenu(t, "MAIN.MNU")
	got := m.PromptKeys(10)
	want := "B M F D T C A N Y I U W X P G"
	if got != want {
		t.Errorf("PromptKeys(10) = %q\n                 want %q", got, want)
	}
}

// TestPromptKeysSysopSeesAll confirms a high-security caller sees every item,
// including Q/S/R that the sec-10 caller could not.
func TestPromptKeysSysopSeesAll(t *testing.T) {
	m := loadTestMenu(t, "MAIN.MNU")
	got := m.PromptKeys(1000)
	want := "B M F D T Q S C A R N Y I U W X P G"
	if got != want {
		t.Errorf("PromptKeys(1000) = %q\n                   want %q", got, want)
	}
}

func TestVisibleItemsFilters(t *testing.T) {
	m := loadTestMenu(t, "MAIN.MNU")
	for _, it := range m.VisibleItems(10) {
		if it.Key == 'Q' || it.Key == 'S' || it.Key == 'R' {
			t.Errorf("sec-10 caller should not see item %c", it.Key)
		}
	}
}

func TestFind(t *testing.T) {
	m := loadTestMenu(t, "MAIN.MNU")
	if it, ok := m.Find('b', 10); !ok || it.Command != "B" {
		t.Errorf("Find(b,10) = %+v,%v; want Bulletin command B", it, ok)
	}
	if _, ok := m.Find('S', 10); ok {
		t.Error("Find(S,10) should fail — Sysop menu needs security 100")
	}
	if _, ok := m.Find('S', 100); !ok {
		t.Error("Find(S,100) should succeed")
	}
}

// TestOtherMenusHeaders pins the remaining real files' color headers.
func TestOtherMenusHeaders(t *testing.T) {
	cases := []struct {
		file                   string
		bBg, bFg, pBg, pFg int
	}{
		{"FILES.MNU", 7, 0, 5, 15},
		{"MESSAGE.MNU", 7, 0, 2, 15},
		{"SYSOP.MNU", 7, 0, 3, 15},
	}
	for _, c := range cases {
		m := loadTestMenu(t, c.file)
		if m.BorderBg != c.bBg || m.BorderFg != c.bFg || m.PanelBg != c.pBg || m.PanelFg != c.pFg {
			t.Errorf("%s colors = %d,%d,%d,%d; want %d,%d,%d,%d", c.file,
				m.BorderBg, m.BorderFg, m.PanelBg, m.PanelFg, c.bBg, c.bFg, c.pBg, c.pFg)
		}
	}
}

// TestColorConversion pins the DOS-attribute -> ANSI-SGR mapping that makes the
// on-screen colors match the screenshot.
func TestColorConversion(t *testing.T) {
	cases := []struct {
		name   string
		got    int
		want   int
	}{
		{"fg white(15)", fgSGR(15), 97},
		{"fg black(0)", fgSGR(0), 30},
		{"bg brown(6)", bgSGR(6), 43},
		{"bg blue(1)", bgSGR(1), 44},
		{"bg magenta(5)", bgSGR(5), 45},
		{"fg brown(6)", fgSGR(6), 33},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}

// TestRenderContainsFaithfulElements checks the rendered box carries the title,
// the visible items, the panel color, and hides items above the caller's level.
func TestRenderContainsFaithfulElements(t *testing.T) {
	m := loadTestMenu(t, "MAIN.MNU")
	out := m.Render(RenderContext{BoardName: "JasonAndOpus", Security: 10})

	if !strings.Contains(out, "JasonAndOpus Main Menu") {
		t.Error("render missing board+menu title")
	}
	if !strings.Contains(out, "<B>..Bulletin Menu") {
		t.Error("render missing <B>..Bulletin Menu item cell")
	}
	if !strings.Contains(out, attr(1, 15)) {
		t.Error("render missing panel color (blue bg / white fg)")
	}
	if strings.Contains(out, "Questionaire") || strings.Contains(out, "Sysop Menu") {
		t.Error("render should hide items above the sec-10 caller's level")
	}
}

// TestLoadOrDefaultFallback confirms the embedded stock menu is used when no
// on-disk file is present, and that a sysop's file wins when it is.
func TestLoadOrDefaultFallback(t *testing.T) {
	m, err := LoadOrDefault("", "MAIN.MNU")
	if err != nil {
		t.Fatalf("embedded MAIN.MNU: %v", err)
	}
	if m.PromptKeys(10) != "B M F D T C A N Y I U W X P G" {
		t.Errorf("embedded default prompt = %q", m.PromptKeys(10))
	}
	// dir with the real files present should load from disk (same content here).
	m2, err := LoadOrDefault("testdata", "FILES.MNU")
	if err != nil || m2.PanelBg != 5 {
		t.Errorf("disk load FILES.MNU = %+v, err %v", m2, err)
	}
}

func TestOnlineLine(t *testing.T) {
	if got := OnlineLine(0, 50); got != "You have been on 0 minutes with 50 remaining." {
		t.Errorf("OnlineLine(0,50) = %q", got)
	}
}
