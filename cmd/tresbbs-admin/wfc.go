package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"golang.org/x/term"

	"github.com/jasondostal/tresbbs/adapter/ansi"
	"github.com/jasondostal/tresbbs/domain"
	nodemgr "github.com/jasondostal/tresbbs/internal/node"
	"github.com/jasondostal/tresbbs/internal/session"
)

// The Waiting-for-Caller (WFC) console — the home screen of the sysop console,
// styled after TriBBS's red-bordered operator screen: a grid of command buttons,
// a magenta "Local Node: Waiting for Caller" bar, and a yellow live-stats panel.
// Each button opens the same editor the left-nav does; two are console-only
// (Drop to Shell, Local Logon).

var (
	wfcRed     = tcell.NewRGBColor(200, 45, 45)  // border — "TresBBS (R)" frame
	wfcMagenta = tcell.NewRGBColor(190, 55, 190) // waiting-for-caller bar
	wfcYellow  = tcell.NewRGBColor(235, 210, 90) // stats + title
	wfcChip    = tcell.NewRGBColor(200, 200, 200) // button face
	wfcChipHi  = tcell.NewRGBColor(255, 235, 120) // focused button face
)

// showWFC installs the WFC console in the content pane.
func (a *adminApp) showWFC() {
	buttons := []struct {
		label string
		fn    func()
	}{
		{"Drop to Shell", a.dropToShell},
		{"Configure Node", a.showConfig},
		{"Edit Conferences", a.showConferences},
		{"Local Logon", a.localLogon},
		{"Edit Users", a.showUsers},
		{"Edit Events", a.showEvents},
		{"Configure System", a.showConfig},
		{"Edit File Areas", a.showFileAreas},
		{"View Callers Log", a.showCallerLog},
		{"Pack User File", a.packUserFile},
		{"Exit", func() { a.app.Stop() }},
		{"Pack Message Base", a.packMessageBase},
	}

	grid := tview.NewGrid().
		SetRows(3, 3, 3, 3).
		SetColumns(0, 0, 0).
		SetGap(0, 2)
	base := tcell.StyleDefault.Background(wfcChip).Foreground(tcell.ColorBlack)
	hi := tcell.StyleDefault.Background(wfcChipHi).Foreground(tcell.ColorBlack).Bold(true)
	for i, b := range buttons {
		button := tview.NewButton(b.label).SetSelectedFunc(b.fn)
		button.SetStyle(base).SetActivatedStyle(hi)
		grid.AddItem(button, i/3, i%3, 1, 1, 0, 0, i == 0)
	}

	bar := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText("[black::b]Local Node: Waiting for Caller")
	bar.SetBackgroundColor(wfcMagenta)

	stats := tview.NewTextView().SetDynamicColors(true)
	stats.SetText(a.wfcStatsText())
	stats.SetBackgroundColor(tcell.ColorBlack)

	content := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(grid, 0, 1, true).
		AddItem(bar, 1, 0, false).
		AddItem(stats, 9, 0, false)
	content.SetBorder(true).
		SetBorderColor(wfcRed).
		SetTitle(fmt.Sprintf(" TresBBS (R) %s ", domain.Version)).
		SetTitleColor(wfcYellow)

	a.setContent(content)
	a.setStatus("WFC Console — arrows/Tab move · Enter selects · Esc for the nav menu")
}

// wfcStatsText builds the yellow live-stats panel shown under the button grid.
func (a *adminApp) wfcStatsText() string {
	cfg, _ := a.storage.GetConfig()
	users, _ := a.storage.UserCount()
	confs, _ := a.storage.GetConferences()
	areas, _ := a.storage.GetFileAreas()
	var msgs int64
	for _, c := range confs {
		if n, err := a.storage.PostCount(c.Name); err == nil {
			msgs += n
		}
	}
	board, sysop, nodes := "TresBBS", "Sysop", 0
	if cfg != nil {
		board, sysop, nodes = cfg.BoardName, cfg.SysopName, cfg.MaxNodes
	}
	row := func(label, value string) string {
		return fmt.Sprintf("  [yellow::b]%-14s[-:-:-] %s\n", label, value)
	}
	return "\n" +
		row("Board", board) +
		row("Sysop", sysop) +
		row("Users", fmt.Sprintf("%d", users)) +
		row("Conferences", fmt.Sprintf("%d", len(confs))) +
		row("File Areas", fmt.Sprintf("%d", len(areas))) +
		row("Messages", fmt.Sprintf("%d", msgs)) +
		row("Nodes", fmt.Sprintf("%d", nodes))
}

// dropToShell suspends the TUI and runs an interactive shell, returning to the
// console on exit (the TresBBS-era "Drop to DOS", made portable).
func (a *adminApp) dropToShell() {
	a.app.Suspend(func() {
		sh := os.Getenv("SHELL")
		if sh == "" {
			sh = "/bin/sh"
		}
		fmt.Println("\nDropped to shell — type 'exit' to return to the console.")
		cmd := exec.Command(sh)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		_ = cmd.Run()
	})
}

// localLogon runs a full BBS session locally against the same board database —
// TriBBS's "Local Logon". It suspends the TUI, puts the terminal in raw mode so
// single-keystroke menus work, and drives a session over stdio (with local echo
// on). On logoff the terminal is restored and the console resumes.
func (a *adminApp) localLogon() {
	a.app.Suspend(func() {
		fd := int(os.Stdin.Fd())
		if old, err := term.MakeRaw(fd); err == nil {
			defer term.Restore(fd, old)
		}

		cfg, err := a.storage.GetConfig()
		if err != nil || cfg == nil {
			cfg = &domain.Config{BoardName: "TresBBS", MaxNodes: 1, MaxTimePerLogon: 60}
		}

		nodes := nodemgr.NewManager(cfg.MaxNodes)
		disp := ansi.New(os.Stdin, os.Stdout)
		disp.SetLocalEcho(true)
		disp.Clear()

		sess := session.New(disp, a.storage, nodes, cfg, 1, "Local", a.menuDir, "local")
		sess.Run()

		fmt.Print("\r\n[Local logon ended — press Enter to return to the console]\r\n")
		fmt.Fscanln(os.Stdin)
	})
}

// packUserFile removes deleted user records (TriBBS "Pack User File").
func (a *adminApp) packUserFile() {
	n, err := a.storage.PackUsers()
	if err != nil {
		a.errorModal("Pack User File failed: " + err.Error())
		return
	}
	a.infoModal("Pack User File", fmt.Sprintf("Removed %d deleted user record(s).", n))
	a.showWFC() // refresh stats
}

// packMessageBase routes to the maintenance view, which handles message-base
// compaction (TriBBS "Pack Message Base").
func (a *adminApp) packMessageBase() {
	a.showMaintenance()
}

// infoModal shows a dismissible informational message with a title.
func (a *adminApp) infoModal(title, msg string) {
	m := tview.NewModal().
		SetText(title + "\n\n" + msg).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(int, string) { a.closeModal("info") })
	a.pages.AddPage("info", m, true, true)
	a.app.SetFocus(m)
}
