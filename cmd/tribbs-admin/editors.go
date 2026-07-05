package main

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/jasondostal/tresbbs/domain"
)

// buildList renders a selectable list with edit/add/delete keybindings.
func (a *adminApp) buildList(title string, labels []string, onEdit func(int), onAdd func(), onDelete func(int)) {
	list := tview.NewList().ShowSecondaryText(false)
	list.SetBorder(true).SetTitle(fmt.Sprintf(" %s (%d) ", title, len(labels)))
	for i, l := range labels {
		idx := i
		list.AddItem(l, "", 0, func() { onEdit(idx) })
	}
	list.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		switch ev.Rune() {
		case 'a':
			if onAdd != nil {
				onAdd()
			}
			return nil
		case 'd':
			if onDelete != nil {
				cur := list.GetCurrentItem()
				if cur >= 0 && cur < len(labels) {
					onDelete(cur)
				}
			}
			return nil
		}
		return ev
	})
	a.setContent(list)
	a.setStatus("[::b]Enter[::-] edit  [::b]a[::-] add  [::b]d[::-] delete  [::b]Esc[::-] menu")
}

// ---- Conferences ---------------------------------------------------------

func (a *adminApp) showConferences() {
	confs, err := a.storage.GetConferences()
	if err != nil {
		a.errorModal(fmt.Sprintf("Get conferences failed: %v", err))
		return
	}
	labels := make([]string, len(confs))
	for i, c := range confs {
		flags := ""
		if c.PrivateConf {
			flags += " [private]"
		}
		if c.Echo {
			flags += " [echo]"
		}
		labels[i] = fmt.Sprintf("%-20s  sec:%-4d msgs:%d%s", c.Name, c.SecurityLevel, c.MessageCount, flags)
	}
	a.buildList("Conferences", labels,
		func(i int) { a.editConference(confs[i], false) },
		func() { a.editConference(domain.Conference{}, true) },
		func(i int) {
			name := confs[i].Name
			a.confirm(fmt.Sprintf("Delete conference %q?", name), func() {
				if err := a.storage.DeleteConference(name); err != nil {
					a.errorModal(fmt.Sprintf("Delete failed: %v", err))
					return
				}
				a.showConferences()
			})
		})
}

func (a *adminApp) editConference(c domain.Conference, isNew bool) {
	form := tview.NewForm()
	title := " Edit Conference "
	if isNew {
		title = " New Conference "
		form.AddInputField("Name", "", 32, nil, nil)
	}
	form.SetBorder(true).SetTitle(title + c.Name)
	form.AddInputField("Security Level", fmt.Sprintf("%d", c.SecurityLevel), 8, nil, nil)
	form.AddCheckbox("Private", c.PrivateConf, nil)
	form.AddCheckbox("Echo (networked)", c.Echo, nil)

	form.AddButton("Save", func() {
		if isNew {
			c.Name = formText(form, "Name")
			if c.Name == "" {
				a.errorModal("Name is required.")
				return
			}
		}
		c.SecurityLevel = atoiOr(formText(form, "Security Level"), c.SecurityLevel)
		c.PrivateConf = formChecked(form, "Private")
		c.Echo = formChecked(form, "Echo (networked)")
		var err error
		if isNew {
			err = a.storage.AddConference(&c)
		} else {
			err = a.storage.SaveConference(&c)
		}
		if err != nil {
			a.errorModal(fmt.Sprintf("Save failed: %v", err))
			return
		}
		a.showConferences()
	})
	form.AddButton("Back", a.showConferences)
	a.setContent(form)
	a.setStatus("[::b]Tab[::-] fields  [::b]Save/Back[::-]  [::b]Esc[::-] menu")
}

// ---- File Areas ----------------------------------------------------------

func (a *adminApp) showFileAreas() {
	areas, err := a.storage.GetFileAreas()
	if err != nil {
		a.errorModal(fmt.Sprintf("Get file areas failed: %v", err))
		return
	}
	labels := make([]string, len(areas))
	for i, ar := range areas {
		cd := ""
		if ar.CDROM {
			cd = " [cdrom]"
		}
		labels[i] = fmt.Sprintf("%-20s  sec:%-4d %s%s", ar.Name, ar.SecurityLevel, ar.Description, cd)
	}
	a.buildList("File Areas", labels,
		func(i int) { a.editFileArea(areas[i], false) },
		func() { a.editFileArea(domain.FileArea{SortType: "name"}, true) },
		func(i int) {
			name := areas[i].Name
			a.confirm(fmt.Sprintf("Delete file area %q?", name), func() {
				if err := a.storage.DeleteFileArea(name); err != nil {
					a.errorModal(fmt.Sprintf("Delete failed: %v", err))
					return
				}
				a.showFileAreas()
			})
		})
}

func (a *adminApp) editFileArea(ar domain.FileArea, isNew bool) {
	form := tview.NewForm()
	title := " Edit File Area "
	if isNew {
		title = " New File Area "
		form.AddInputField("Name", "", 32, nil, nil)
	}
	form.SetBorder(true).SetTitle(title + ar.Name)
	form.AddInputField("Description", ar.Description, 40, nil, nil)
	form.AddInputField("Path", ar.Path, 48, nil, nil)
	form.AddInputField("Security Level", fmt.Sprintf("%d", ar.SecurityLevel), 8, nil, nil)
	form.AddCheckbox("CD-ROM (read-only)", ar.CDROM, nil)
	sortOpts := []string{"name", "date", "size"}
	sortIdx := 0
	for i, s := range sortOpts {
		if s == ar.SortType {
			sortIdx = i
		}
	}
	form.AddDropDown("Sort Type", sortOpts, sortIdx, nil)

	form.AddButton("Save", func() {
		if isNew {
			ar.Name = formText(form, "Name")
			if ar.Name == "" {
				a.errorModal("Name is required.")
				return
			}
		}
		ar.Description = formText(form, "Description")
		ar.Path = formText(form, "Path")
		ar.SecurityLevel = atoiOr(formText(form, "Security Level"), ar.SecurityLevel)
		ar.CDROM = formChecked(form, "CD-ROM (read-only)")
		if _, s := formDropdown(form, "Sort Type"); s != "" {
			ar.SortType = s
		}
		var err error
		if isNew {
			err = a.storage.AddFileArea(&ar)
		} else {
			err = a.storage.SaveFileArea(&ar)
		}
		if err != nil {
			a.errorModal(fmt.Sprintf("Save failed: %v", err))
			return
		}
		a.showFileAreas()
	})
	form.AddButton("Back", a.showFileAreas)
	a.setContent(form)
	a.setStatus("[::b]Tab[::-] fields  [::b]Save/Back[::-]  [::b]Esc[::-] menu")
}

// ---- Doors ---------------------------------------------------------------

func (a *adminApp) showDoors() {
	doors, err := a.storage.GetDoors()
	if err != nil {
		a.errorModal(fmt.Sprintf("Get doors failed: %v", err))
		return
	}
	labels := make([]string, len(doors))
	for i, d := range doors {
		hk := ""
		if d.HotKey != 0 {
			hk = fmt.Sprintf("[%c] ", d.HotKey)
		}
		labels[i] = fmt.Sprintf("%s%-20s sec:%-4d %s", hk, d.Name, d.Security, d.Description)
	}
	a.buildList("Doors", labels,
		func(i int) { a.editDoor(doors[i], false) },
		func() { a.editDoor(domain.Door{DropFormat: "DOORSYS", TimeLimit: 30}, true) },
		func(i int) {
			name := doors[i].Name
			a.confirm(fmt.Sprintf("Delete door %q?", name), func() {
				if err := a.storage.DeleteDoor(name); err != nil {
					a.errorModal(fmt.Sprintf("Delete failed: %v", err))
					return
				}
				a.showDoors()
			})
		})
}

func (a *adminApp) editDoor(d domain.Door, isNew bool) {
	form := tview.NewForm()
	title := " Edit Door "
	if isNew {
		title = " New Door "
		form.AddInputField("Name", "", 32, nil, nil)
	}
	form.SetBorder(true).SetTitle(title + d.Name)
	hotkey := ""
	if d.HotKey != 0 {
		hotkey = string(d.HotKey)
	}
	form.AddInputField("Hot Key", hotkey, 4, nil, nil)
	form.AddInputField("Description", d.Description, 40, nil, nil)
	form.AddInputField("Command", d.Command, 48, nil, nil)
	dropOpts := []string{"DOORSYS", "DORINFO"}
	dropIdx := 0
	for i, o := range dropOpts {
		if o == d.DropFormat {
			dropIdx = i
		}
	}
	form.AddDropDown("Drop Format", dropOpts, dropIdx, nil)
	form.AddInputField("Security", fmt.Sprintf("%d", d.Security), 8, nil, nil)
	form.AddInputField("Time Limit (min)", fmt.Sprintf("%d", d.TimeLimit), 8, nil, nil)

	form.AddButton("Save", func() {
		if isNew {
			d.Name = formText(form, "Name")
			if d.Name == "" {
				a.errorModal("Name is required.")
				return
			}
		}
		if hk := formText(form, "Hot Key"); hk != "" {
			d.HotKey = hk[0]
		} else {
			d.HotKey = 0
		}
		d.Description = formText(form, "Description")
		d.Command = formText(form, "Command")
		if _, s := formDropdown(form, "Drop Format"); s != "" {
			d.DropFormat = s
		}
		d.Security = atoiOr(formText(form, "Security"), d.Security)
		d.TimeLimit = atoiOr(formText(form, "Time Limit (min)"), d.TimeLimit)
		var err error
		if isNew {
			err = a.storage.AddDoor(&d)
		} else {
			err = a.storage.SaveDoor(&d)
		}
		if err != nil {
			a.errorModal(fmt.Sprintf("Save failed: %v", err))
			return
		}
		a.showDoors()
	})
	form.AddButton("Back", a.showDoors)
	a.setContent(form)
	a.setStatus("[::b]Tab[::-] fields  [::b]Save/Back[::-]  [::b]Esc[::-] menu")
}

// ---- Bulletins -----------------------------------------------------------

func (a *adminApp) showBulletins() {
	bulls, err := a.storage.GetBulletins()
	if err != nil {
		a.errorModal(fmt.Sprintf("Get bulletins failed: %v", err))
		return
	}
	labels := make([]string, len(bulls))
	for i, b := range bulls {
		labels[i] = fmt.Sprintf("#%-4d %-24s sec:%d", b.ID, b.Name, b.Security)
	}
	a.buildList("Bulletins", labels,
		func(i int) { a.editBulletin(bulls[i], false) },
		func() { a.editBulletin(domain.Bulletin{}, true) },
		func(i int) {
			b := bulls[i]
			a.confirm(fmt.Sprintf("Delete bulletin %q (#%d)?", b.Name, b.ID), func() {
				if err := a.storage.DeleteBulletin(b.ID); err != nil {
					a.errorModal(fmt.Sprintf("Delete failed: %v", err))
					return
				}
				a.showBulletins()
			})
		})
}

func (a *adminApp) editBulletin(b domain.Bulletin, isNew bool) {
	form := tview.NewForm()
	title := " New Bulletin "
	if !isNew {
		title = fmt.Sprintf(" Edit Bulletin #%d ", b.ID)
	}
	form.SetBorder(true).SetTitle(title)
	form.AddInputField("Name", b.Name, 32, nil, nil)
	form.AddInputField("Security", fmt.Sprintf("%d", b.Security), 8, nil, nil)
	form.AddTextArea("Content", b.Content, 0, 10, 0, nil)

	form.AddButton("Save", func() {
		b.Name = formText(form, "Name")
		if b.Name == "" {
			a.errorModal("Name is required.")
			return
		}
		b.Security = atoiOr(formText(form, "Security"), b.Security)
		b.Content = formArea(form, "Content")
		var err error
		if isNew {
			err = a.storage.AddBulletin(&b)
		} else {
			err = a.storage.SaveBulletin(&b)
		}
		if err != nil {
			a.errorModal(fmt.Sprintf("Save failed: %v", err))
			return
		}
		a.showBulletins()
	})
	form.AddButton("Back", a.showBulletins)
	a.setContent(form)
	a.setStatus("[::b]Tab[::-] fields  [::b]Save/Back[::-]  [::b]Esc[::-] menu")
}

// ---- Events --------------------------------------------------------------

func (a *adminApp) showEvents() {
	events, err := a.storage.GetEvents()
	if err != nil {
		a.errorModal(fmt.Sprintf("Get events failed: %v", err))
		return
	}
	labels := make([]string, len(events))
	for i, e := range events {
		slide := ""
		if e.Slide {
			slide = " [slide]"
		}
		labels[i] = fmt.Sprintf("#%-4d %-6s %-8s %s%s", e.ID, e.Time, e.Day, e.File, slide)
	}
	a.buildList("Events", labels,
		func(i int) { a.editEvent(events[i], false) },
		func() { a.editEvent(domain.Event{Time: "00:00", Day: "Daily"}, true) },
		func(i int) {
			e := events[i]
			a.confirm(fmt.Sprintf("Delete event #%d?", e.ID), func() {
				if err := a.storage.DeleteEvent(e.ID); err != nil {
					a.errorModal(fmt.Sprintf("Delete failed: %v", err))
					return
				}
				a.showEvents()
			})
		})
}

func (a *adminApp) editEvent(e domain.Event, isNew bool) {
	form := tview.NewForm()
	title := " New Event "
	if !isNew {
		title = fmt.Sprintf(" Edit Event #%d ", e.ID)
	}
	form.SetBorder(true).SetTitle(title)
	form.AddInputField("Time (HH:MM)", e.Time, 8, nil, nil)
	dayOpts := []string{"Daily", "Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	dayIdx := 0
	for i, d := range dayOpts {
		if d == e.Day {
			dayIdx = i
		}
	}
	form.AddDropDown("Day", dayOpts, dayIdx, nil)
	form.AddInputField("File / Command", e.File, 48, nil, nil)
	form.AddCheckbox("Slide (run next opportunity)", e.Slide, nil)

	form.AddButton("Save", func() {
		e.Time = formText(form, "Time (HH:MM)")
		if _, d := formDropdown(form, "Day"); d != "" {
			e.Day = d
		}
		e.File = formText(form, "File / Command")
		e.Slide = formChecked(form, "Slide (run next opportunity)")
		// SaveEvent inserts when ID == 0, updates otherwise.
		if err := a.storage.SaveEvent(&e); err != nil {
			a.errorModal(fmt.Sprintf("Save failed: %v", err))
			return
		}
		a.showEvents()
	})
	form.AddButton("Back", a.showEvents)
	a.setContent(form)
	a.setStatus("[::b]Tab[::-] fields  [::b]Save/Back[::-]  [::b]Esc[::-] menu")
}
