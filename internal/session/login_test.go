package session

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jasondostal/tresbbs/adapter/sqlite"
	"github.com/jasondostal/tresbbs/domain"
	"github.com/jasondostal/tresbbs/internal/auth"
	nodemgr "github.com/jasondostal/tresbbs/internal/node"
)

// TestLoginToMainMenuEndToEnd drives the real session constructor through the
// full flow — connect, log in an existing user against actual SQLite storage,
// render the main menu from the embedded .MNU, and log off — with nothing but
// scripted keystrokes. It guards the whole wired path (New -> Run -> login ->
// showMenu -> dispatchMain) against regressions.
func TestLoginToMainMenuEndToEnd(t *testing.T) {
	dir := t.TempDir()
	st, err := sqlite.New(filepath.Join(dir, "board.db"))
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer st.Close()

	hash, err := auth.HashPassword("secret")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if _, err := st.AddUser(&domain.User{
		Name: "Jason Dostal", Alias: "JasonD", Password: hash,
		SecurityLevel: 1000, ANSIMode: 1, City: "Plover", State: "WI",
	}); err != nil {
		t.Fatalf("add user: %v", err)
	}

	cfg := &domain.Config{
		BoardName: "JasonAndOpus", SysopName: "Jason", SysopSecurity: 90,
		MaxNodes: 4, MaxTimePerLogon: 60, AllowNewUsers: true, Allow2400Baud: true,
		MaxBaud: 14400,
	}
	nodes := nodemgr.NewManager(cfg.MaxNodes)

	disp := &captureDisplay{
		lines:     []string{"Jason Dostal"}, // User Name prompt; later pauses read ""
		passwords: []string{"secret"},       // Password prompt
		keys:      []byte{'G'},              // Main menu -> Goodbye
	}

	sess := New(disp, st, nodes, cfg, 1, "Telnet", "", "test")
	sess.Run()

	out := disp.out()
	checks := map[string]string{
		"Welcome back, JasonD!":                                  "login did not succeed",
		"JasonAndOpus Main Menu":                                 "main menu not rendered from the .MNU",
		"Enter Selection - [B M F D T Q S C A R N Y I U W X P G ?]? ": "prompt not built from the .MNU (sysop keys expected)",
		"Goodbye":                                                "did not reach logoff",
	}
	for want, msg := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("%s — missing %q", msg, want)
		}
	}

	// The login should have been recorded in the callers log.
	if log, _ := st.GetCallerLog(50); !containsSubstr(log, "logged on") {
		t.Error("expected a 'logged on' entry in the callers log")
	}
}

func containsSubstr(lines []string, sub string) bool {
	for _, l := range lines {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}
