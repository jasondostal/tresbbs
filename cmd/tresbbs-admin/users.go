package main

import (
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/jasondostal/tresbbs/domain"
	"github.com/jasondostal/tresbbs/internal/auth"
)

const subDateLayout = "01/02/2006"

// showUsers renders the user table. Enter edits; '/' searches by alias.
func (a *adminApp) showUsers() {
	users, err := a.storage.ListUsers()
	if err != nil {
		a.errorModal(fmt.Sprintf("List users failed: %v", err))
		return
	}

	table := tview.NewTable().SetBorders(false).SetSelectable(true, false).SetFixed(1, 0)
	table.SetBorder(true).SetTitle(fmt.Sprintf(" Users (%d) ", len(users)))

	headers := []string{"Rec#", "Name", "Alias", "Sec", "Node", "Locked"}
	for c, h := range headers {
		table.SetCell(0, c, tview.NewTableCell(h).
			SetTextColor(tcell.NewRGBColor(120, 200, 255)).
			SetSelectable(false).SetAttributes(tcell.AttrBold))
	}
	for i, u := range users {
		row := i + 1
		locked := ""
		if u.LockedOut {
			locked = "LOCKED"
		}
		table.SetCell(row, 0, tview.NewTableCell(fmt.Sprintf("%d", u.RecordNumber)))
		table.SetCell(row, 1, tview.NewTableCell(u.Name))
		table.SetCell(row, 2, tview.NewTableCell(u.Alias))
		table.SetCell(row, 3, tview.NewTableCell(fmt.Sprintf("%d", u.SecurityLevel)))
		table.SetCell(row, 4, tview.NewTableCell(fmt.Sprintf("%d", u.NodeSecurity)))
		table.SetCell(row, 5, tview.NewTableCell(locked).
			SetTextColor(tcell.NewRGBColor(230, 120, 120)))
	}
	if len(users) > 0 {
		table.Select(1, 0)
	}

	table.SetSelectedFunc(func(row, _ int) {
		if row >= 1 && row-1 < len(users) {
			a.editUser(users, row-1)
		}
	})
	table.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Rune() == '/' {
			a.prompt("Find user", "Alias:", "", func(alias string) {
				if alias == "" {
					return
				}
				u, err := a.storage.GetUserByAlias(alias)
				if err != nil {
					a.errorModal(fmt.Sprintf("No user with alias %q", alias))
					return
				}
				for i := range users {
					if users[i].RecordNumber == u.RecordNumber {
						a.editUser(users, i)
						return
					}
				}
				a.editUser([]domain.User{*u}, 0)
			})
			return nil
		}
		return ev
	})

	a.setContent(table)
	a.setStatus("[::b]Enter[::-] edit  [::b]/[::-] find by alias  [::b]Up/Down[::-] move  [::b]Esc[::-] menu")
}

// editUser builds the full-field editor form for users[idx], with paging.
func (a *adminApp) editUser(users []domain.User, idx int) {
	if idx < 0 || idx >= len(users) {
		return
	}
	u := users[idx]

	form := tview.NewForm()
	form.SetBorder(true).
		SetTitle(fmt.Sprintf(" Edit User  #%d  (%d of %d) ", u.RecordNumber, idx+1, len(users)))

	sub := ""
	if u.Subscription != nil {
		sub = u.Subscription.Format(subDateLayout)
	}
	protoOpts := []string{"X", "Y", "G", "Z", "K"}
	protoIdx := 3
	for i, p := range protoOpts {
		if len(p) > 0 && p[0] == u.Protocol {
			protoIdx = i
		}
	}
	ansiOpts := []string{"None", "ANSI", "RIP"}
	ansiIdx := u.ANSIMode
	if ansiIdx < 0 || ansiIdx >= len(ansiOpts) {
		ansiIdx = 0
	}

	form.AddInputField("Name", u.Name, 32, nil, nil)
	form.AddInputField("Alias", u.Alias, 32, nil, nil)
	form.AddPasswordField("New Password", "", 32, '*', nil)
	form.AddInputField("Security Level", fmt.Sprintf("%d", u.SecurityLevel), 8, nil, nil)
	form.AddInputField("Node Security", fmt.Sprintf("%d", u.NodeSecurity), 8, nil, nil)
	form.AddInputField("Phone", u.Phone, 24, nil, nil)
	form.AddInputField("Caller ID", u.CallerID, 24, nil, nil)
	form.AddCheckbox("Block Caller ID", u.BlockCallerID, nil)
	form.AddInputField("City", u.City, 32, nil, nil)
	form.AddInputField("State", u.State, 12, nil, nil)
	form.AddInputField("Zip Code", u.ZipCode, 16, nil, nil)
	form.AddInputField("Country", u.Country, 24, nil, nil)
	form.AddInputField("Street Address", u.StreetAddress, 40, nil, nil)
	form.AddInputField("Address", u.Address, 40, nil, nil)
	form.AddInputField("Email", u.Email, 40, nil, nil)
	form.AddInputField("Birthdate", u.BirthDate, 16, nil, nil)
	form.AddInputField("Registration", u.Registration, 24, nil, nil)
	form.AddInputField("Subscription (MM/DD/YYYY)", sub, 16, nil, nil)
	form.AddCheckbox("Locked Out", u.LockedOut, nil)
	form.AddInputField("Files Uploaded", itoa64(u.FilesUploaded), 12, nil, nil)
	form.AddInputField("Files Downloaded", itoa64(u.FilesDownloaded), 12, nil, nil)
	form.AddInputField("K Uploaded", itoa64(u.KUploaded), 12, nil, nil)
	form.AddInputField("K Downloaded", itoa64(u.KDownloaded), 12, nil, nil)
	form.AddInputField("Messages Posted", itoa64(u.MessagesPosted), 12, nil, nil)
	form.AddInputField("Total Calls", itoa64(u.TotalCalls), 12, nil, nil)
	form.AddInputField("Time Left Today", fmt.Sprintf("%d", u.TimeLeftToday), 8, nil, nil)
	form.AddInputField("Daily File Limit", fmt.Sprintf("%d", u.DailyFileLimit), 8, nil, nil)
	form.AddInputField("Daily Byte Limit", fmt.Sprintf("%d", u.DailyByteLimit), 12, nil, nil)
	form.AddInputField("Editor", u.Editor, 20, nil, nil)
	form.AddDropDown("Protocol", protoOpts, protoIdx, nil)
	form.AddDropDown("ANSI Mode", ansiOpts, ansiIdx, nil)
	form.AddInputField("Screen Width", fmt.Sprintf("%d", u.ScreenWidth), 8, nil, nil)

	form.AddButton("Save", func() {
		a.saveUserForm(form, &u)
	})
	form.AddButton("Prev", func() {
		if idx > 0 {
			a.editUser(users, idx-1)
		}
	})
	form.AddButton("Next", func() {
		if idx < len(users)-1 {
			a.editUser(users, idx+1)
		}
	})
	form.AddButton("Delete", func() {
		a.confirm(fmt.Sprintf("Delete user %q (#%d)?", u.Name, u.RecordNumber), func() {
			if err := a.storage.DeleteUser(u.RecordNumber); err != nil {
				a.errorModal(fmt.Sprintf("Delete failed: %v", err))
				return
			}
			a.showUsers()
		})
	})
	form.AddButton("Back", a.showUsers)

	a.setContent(form)
	a.setStatus("[::b]Tab[::-] fields  [::b]Enter[::-] activate button  [::b]Save/Prev/Next/Delete/Back[::-]  [::b]Esc[::-] menu")
}

// saveUserForm reads the form back into u, persists it, and applies a password
// change if one was entered.
func (a *adminApp) saveUserForm(form *tview.Form, u *domain.User) {
	u.Name = formText(form, "Name")
	u.Alias = formText(form, "Alias")
	u.SecurityLevel = atoiOr(formText(form, "Security Level"), u.SecurityLevel)
	u.NodeSecurity = atoiOr(formText(form, "Node Security"), u.NodeSecurity)
	u.Phone = formText(form, "Phone")
	u.CallerID = formText(form, "Caller ID")
	u.BlockCallerID = formChecked(form, "Block Caller ID")
	u.City = formText(form, "City")
	u.State = formText(form, "State")
	u.ZipCode = formText(form, "Zip Code")
	u.Country = formText(form, "Country")
	u.StreetAddress = formText(form, "Street Address")
	u.Address = formText(form, "Address")
	u.Email = formText(form, "Email")
	u.BirthDate = formText(form, "Birthdate")
	u.Registration = formText(form, "Registration")
	u.LockedOut = formChecked(form, "Locked Out")
	u.FilesUploaded = atoi64Or(formText(form, "Files Uploaded"), u.FilesUploaded)
	u.FilesDownloaded = atoi64Or(formText(form, "Files Downloaded"), u.FilesDownloaded)
	u.KUploaded = atoi64Or(formText(form, "K Uploaded"), u.KUploaded)
	u.KDownloaded = atoi64Or(formText(form, "K Downloaded"), u.KDownloaded)
	u.MessagesPosted = atoi64Or(formText(form, "Messages Posted"), u.MessagesPosted)
	u.TotalCalls = atoi64Or(formText(form, "Total Calls"), u.TotalCalls)
	u.TimeLeftToday = atoiOr(formText(form, "Time Left Today"), u.TimeLeftToday)
	u.DailyFileLimit = atoiOr(formText(form, "Daily File Limit"), u.DailyFileLimit)
	u.DailyByteLimit = atoiOr(formText(form, "Daily Byte Limit"), u.DailyByteLimit)
	u.Editor = formText(form, "Editor")
	if _, p := formDropdown(form, "Protocol"); p != "" {
		u.Protocol = p[0]
	}
	if ai, _ := formDropdown(form, "ANSI Mode"); ai >= 0 {
		u.ANSIMode = ai
	}
	u.ScreenWidth = atoiOr(formText(form, "Screen Width"), u.ScreenWidth)

	subStr := formText(form, "Subscription (MM/DD/YYYY)")
	if subStr == "" {
		u.Subscription = nil
	} else if t, err := time.Parse(subDateLayout, subStr); err == nil {
		u.Subscription = &t
	} else {
		a.errorModal("Subscription must be MM/DD/YYYY or blank.")
		return
	}

	// Apply a password change before saving; SaveUser persists password_hash.
	if pw := formText(form, "New Password"); pw != "" {
		hash, err := auth.HashPassword(pw)
		if err != nil {
			a.errorModal(fmt.Sprintf("Password hash failed: %v", err))
			return
		}
		u.Password = hash
	}

	if err := a.storage.SaveUser(u); err != nil {
		a.errorModal(fmt.Sprintf("Save failed: %v", err))
		return
	}
	a.setStatus(fmt.Sprintf("[green]Saved user #%d (%s).[white]", u.RecordNumber, u.Name))
}
