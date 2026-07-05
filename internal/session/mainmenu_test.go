package session

import (
	"strings"
	"testing"
	"time"

	"github.com/jasondostal/tresbbs/domain"
	"github.com/jasondostal/tresbbs/internal/template"
)

// captureDisplay is a DisplayPort that records everything written and replays a
// scripted sequence of keypresses. It lets us assert the exact bytes a caller
// would see for a rendered menu without a live socket.
type captureDisplay struct {
	buf       strings.Builder
	keys      []byte
	kpos      int
	lines     []string // scripted ReadLine results (then "" when exhausted)
	lpos      int
	passwords []string // scripted ReadPassword results (then "" when exhausted)
	ppos      int
}

func (d *captureDisplay) Clear()                    { d.buf.WriteString("<CLR>") }
func (d *captureDisplay) MoveTo(row, col int)       {}
func (d *captureDisplay) Write(text string)         { d.buf.WriteString(text) }
func (d *captureDisplay) WriteLine(text string)     { d.buf.WriteString(text + "\n") }
func (d *captureDisplay) SetColor(color byte)       {}
func (d *captureDisplay) ResetColor()               {}
func (d *captureDisplay) SetReverse(on bool) {}
func (d *captureDisplay) ReadLine() (string, error) {
	if d.lpos >= len(d.lines) {
		return "", nil
	}
	s := d.lines[d.lpos]
	d.lpos++
	return s, nil
}
func (d *captureDisplay) ReadKey() (byte, error) {
	if d.kpos >= len(d.keys) {
		return 'G', nil // default to Goodbye so loops terminate
	}
	b := d.keys[d.kpos]
	d.kpos++
	return b, nil
}
func (d *captureDisplay) ReadPassword() (string, error) {
	if d.ppos >= len(d.passwords) {
		return "", nil
	}
	s := d.passwords[d.ppos]
	d.ppos++
	return s, nil
}
func (d *captureDisplay) ShowCursor(show bool)          {}
func (d *captureDisplay) Flush()                        {}
func (d *captureDisplay) Width() int                    { return 80 }
func (d *captureDisplay) Height() int                   { return 24 }
func (d *captureDisplay) DrawBox(r, c, w, h int)        {}
func (d *captureDisplay) SetScrollRegion(t, b int)      {}

func (d *captureDisplay) out() string { return d.buf.String() }

func newTestSession(disp *captureDisplay, sec int) *Session {
	cfg := &domain.Config{BoardName: "JasonAndOpus", SysopName: "Jason", SysopSecurity: 100}
	return &Session{
		display: disp,
		config:  cfg,
		user: &domain.User{
			Name: "Jason Dostal", City: "Plover", State: "WI",
			SecurityLevel: sec, TotalCalls: 3, TimeLeftToday: 50, ANSIMode: 1,
		},
		nodeNum:        1,
		baudRate:       "Local",
		loginAt:        time.Now(),
		templateEngine: template.NewEngine(cfg, "templates"),
		templateData:   &template.TemplateData{Config: cfg},
	}
}

// TestShowMenuRendersMainFromMNU is the end-to-end parity check: a sec-10 caller
// on the main menu sees the board+menu title, the item cells, the exact
// "Enter Selection" prompt from the .MNU, and none of the items above his level.
func TestShowMenuRendersMainFromMNU(t *testing.T) {
	disp := &captureDisplay{keys: []byte{'M'}}
	s := newTestSession(disp, 10)

	key, m := s.showMenu("MAIN.MNU")
	if key != 'M' {
		t.Errorf("returned key = %q, want M", key)
	}
	if m == nil || m.Name != "MAIN" {
		t.Fatalf("loaded menu = %+v", m)
	}
	out := disp.out()

	for _, want := range []string{
		"JasonAndOpus Main Menu",
		"<B>..Bulletin Menu",
		"You have been on 0 minutes with 50 remaining.",
		"Enter Selection - [B M F D T C A N Y I U W X P G ?]? ",
		"Sec Level: 10", // status bar
	} {
		if !strings.Contains(out, want) {
			t.Errorf("main-menu render missing %q", want)
		}
	}
	if strings.Contains(out, "Questionaire") || strings.Contains(out, "Sysop Menu") {
		t.Error("sec-10 caller should not see security-gated items (Questionaire/Sysop)")
	}
}

// TestShowMenuSysopSeesGatedItems confirms a sysop-level caller's prompt gains
// the Q/S/R keys hidden from the sec-10 caller.
func TestShowMenuSysopSeesGatedItems(t *testing.T) {
	disp := &captureDisplay{keys: []byte{'S'}}
	s := newTestSession(disp, 1000)
	s.showMenu("MAIN.MNU")
	if !strings.Contains(disp.out(), "Enter Selection - [B M F D T Q S C A R N Y I U W X P G ?]? ") {
		t.Error("sysop prompt should include Q/S/R keys")
	}
}

// TestMenuLoopDispatch checks the shared sub-menu loop: a menu-specific key runs
// its handler and keeps looping, while M returns to the main menu (quit=false).
func TestMenuLoopDispatch(t *testing.T) {
	disp := &captureDisplay{keys: []byte{'C', 'M'}} // press C (handler), then M (back)
	s := newTestSession(disp, 10)
	calls := 0
	quit := s.menuLoop("MESSAGE.MNU", "Message Menu", map[byte]func(){
		'C': func() { calls++ },
	})
	if calls != 1 {
		t.Errorf("handler called %d times, want 1", calls)
	}
	if quit {
		t.Error("M should return to main menu (quit=false), not end the session")
	}
}

// TestExpertModeSuppressesBox verifies expert mode hides the menu box but still
// prints the command prompt.
func TestExpertModeSuppressesBox(t *testing.T) {
	disp := &captureDisplay{keys: []byte{'G'}}
	s := newTestSession(disp, 10)
	s.expert = true
	s.showMenu("MAIN.MNU")
	out := disp.out()
	if strings.Contains(out, "<B>..Bulletin Menu") {
		t.Error("expert mode should suppress the menu box")
	}
	if !strings.Contains(out, "Enter Selection - [") {
		t.Error("expert mode should still show the prompt")
	}
}
