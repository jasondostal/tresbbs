// tresbbs-admin is the sysop console for TresBBS — a keyboard-first, dark-themed
// terminal UI for offline board management. It is the modern replacement for
// TriBBS's DOS-era TRIMAN admin tool.
//
// It opens the same SQLite board database the server uses (via the shared
// adapter/sqlite storage layer) and lets a sysop edit users, conferences, file
// areas, doors, bulletins, events, the master config, view the callers log, and
// run maintenance.
//
// Usage:
//
//	tresbbs-admin [-db board.db]
//
// Navigation:
//
//	Up/Down    move within a list, table, or form
//	Tab        move between fields / buttons
//	Enter      activate the selected item
//	Esc        back out — close a dialog, or jump to the left nav menu
//	Ctrl-C     quit
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/jasondostal/tresbbs/adapter/sqlite"
)

// adminApp holds the shared UI state for the console.
type adminApp struct {
	app     *tview.Application
	storage *sqlite.Storage
	dbPath  string

	pages       *tview.Pages // top-level: "main" plus transient modal pages
	nav         *tview.List  // left navigation menu
	contentArea *tview.Flex  // right content pane (swapped per view)
	status      *tview.TextView
	title       *tview.TextView

	menuDir string // TriBBS .MNU directory used by Local Logon (a dropped-in NWORK/)
}

func main() {
	dbPath := flag.String("db", "board.db", "SQLite board database path")
	menuDir := flag.String("menus", "", "TriBBS .MNU directory (NWORK/) used by Local Logon")
	flag.Parse()

	storage, err := sqlite.New(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tresbbs-admin: cannot open database %q: %v\n", *dbPath, err)
		os.Exit(1)
	}
	defer storage.Close()

	a := &adminApp{
		app:     tview.NewApplication(),
		storage: storage,
		dbPath:  *dbPath,
		menuDir: *menuDir,
	}
	a.setupTheme()
	a.build()

	if err := a.app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "tresbbs-admin: %v\n", err)
		os.Exit(1)
	}
}

// setupTheme installs a dark colour scheme used across all widgets.
func (a *adminApp) setupTheme() {
	tview.Styles = tview.Theme{
		PrimitiveBackgroundColor:    tcell.ColorBlack,
		ContrastBackgroundColor:     tcell.NewRGBColor(0, 60, 90),
		MoreContrastBackgroundColor: tcell.NewRGBColor(0, 90, 130),
		BorderColor:                 tcell.NewRGBColor(90, 130, 160),
		TitleColor:                  tcell.NewRGBColor(120, 200, 255),
		GraphicsColor:               tcell.NewRGBColor(90, 130, 160),
		PrimaryTextColor:            tcell.NewRGBColor(220, 220, 220),
		SecondaryTextColor:          tcell.NewRGBColor(150, 200, 150),
		TertiaryTextColor:           tcell.NewRGBColor(200, 180, 120),
		InverseTextColor:            tcell.ColorBlack,
		ContrastSecondaryTextColor:  tcell.NewRGBColor(120, 200, 255),
	}
}

// build wires up the root layout: header, left nav + content, status bar.
func (a *adminApp) build() {
	a.pages = tview.NewPages()

	a.title = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText("[::b]TresBBS Sysop Console[::-]  —  " + a.dbPath)
	a.title.SetBackgroundColor(tcell.NewRGBColor(0, 60, 90))

	a.status = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	a.status.SetBackgroundColor(tcell.NewRGBColor(0, 40, 60))

	a.nav = tview.NewList().ShowSecondaryText(false)
	a.nav.SetBorder(true).SetTitle(" Menu ")

	type navItem struct {
		label    string
		shortcut rune
		action   func()
	}
	items := []navItem{
		{"Console (WFC)", 'w', a.showWFC},
		{"Users", 'u', a.showUsers},
		{"Conferences", 'c', a.showConferences},
		{"File Areas", 'f', a.showFileAreas},
		{"Doors", 'd', a.showDoors},
		{"Bulletins", 'b', a.showBulletins},
		{"Events", 'e', a.showEvents},
		{"Config", 'g', a.showConfig},
		{"Callers Log", 'l', a.showCallerLog},
		{"Maintenance", 'm', a.showMaintenance},
		{"Quit", 'q', func() { a.app.Stop() }},
	}
	for _, it := range items {
		act := it.action
		a.nav.AddItem(it.label, "", it.shortcut, func() {
			act()
			a.app.SetFocus(a.contentArea)
		})
	}

	a.contentArea = tview.NewFlex()
	a.contentArea.SetBorder(false)

	body := tview.NewFlex().
		AddItem(a.nav, 22, 0, true).
		AddItem(a.contentArea, 0, 1, false)

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.title, 1, 0, false).
		AddItem(body, 0, 1, true).
		AddItem(a.status, 1, 0, false)

	a.pages.AddPage("main", layout, true, true)
	a.app.SetRoot(a.pages, true).EnableMouse(true)

	a.app.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		switch ev.Key() {
		case tcell.KeyEsc:
			name, _ := a.pages.GetFrontPage()
			if name == "main" {
				a.app.SetFocus(a.nav)
			} else {
				a.pages.RemovePage(name)
				a.app.SetFocus(a.contentArea)
			}
			return nil
		}
		return ev
	})

	// Open on the WFC console — the operator's home screen.
	a.showWFC()
	a.app.SetFocus(a.nav)
}

// setContent replaces the right-hand content pane with p.
func (a *adminApp) setContent(p tview.Primitive) {
	a.contentArea.Clear()
	a.contentArea.AddItem(p, 0, 1, true)
	a.app.SetFocus(p)
}

// setStatus updates the bottom status/keybinding bar.
func (a *adminApp) setStatus(s string) {
	a.status.SetText(" " + s)
}

// ---- modal helpers -------------------------------------------------------

// showModal centres p over the main layout as a named overlay page.
func (a *adminApp) showModal(name string, p tview.Primitive, width, height int) {
	overlay := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, height, 0, true).
			AddItem(nil, 0, 1, false), width, 0, true).
		AddItem(nil, 0, 1, false)
	a.pages.AddPage(name, overlay, true, true)
	a.app.SetFocus(p)
}

func (a *adminApp) closeModal(name string) {
	a.pages.RemovePage(name)
	a.app.SetFocus(a.contentArea)
}

// errorModal shows a dismissible error message.
func (a *adminApp) errorModal(msg string) {
	m := tview.NewModal().
		SetText(msg).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(int, string) { a.closeModal("error") })
	a.pages.AddPage("error", m, true, true)
	a.app.SetFocus(m)
}

// confirm asks a yes/no question, invoking onYes when confirmed.
func (a *adminApp) confirm(msg string, onYes func()) {
	m := tview.NewModal().
		SetText(msg).
		AddButtons([]string{"Yes", "No"}).
		SetDoneFunc(func(_ int, label string) {
			a.closeModal("confirm")
			if label == "Yes" {
				onYes()
			}
		})
	a.pages.AddPage("confirm", m, true, true)
	a.app.SetFocus(m)
}

// prompt shows a single-field text entry dialog.
func (a *adminApp) prompt(title, label, initial string, onDone func(string)) {
	input := tview.NewInputField().SetLabel(label + " ").SetText(initial).SetFieldWidth(30)
	form := tview.NewForm().
		AddFormItem(input).
		AddButton("OK", func() {
			v := input.GetText()
			a.closeModal("prompt")
			onDone(v)
		}).
		AddButton("Cancel", func() { a.closeModal("prompt") })
	form.SetBorder(true).SetTitle(" " + title + " ")
	a.showModal("prompt", form, 56, 7)
}
