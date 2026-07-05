package main

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"
)

// ---- Config editor -------------------------------------------------------

func (a *adminApp) showConfig() {
	cfg, err := a.storage.GetConfig()
	if err != nil {
		a.errorModal(fmt.Sprintf("Get config failed: %v", err))
		return
	}

	form := tview.NewForm()
	form.SetBorder(true).SetTitle(" Board Configuration ")

	form.AddInputField("Board Name", cfg.BoardName, 40, nil, nil)
	form.AddInputField("Sysop Name", cfg.SysopName, 40, nil, nil)
	form.AddPasswordField("System Password", cfg.SystemPassword, 32, '*', nil)
	form.AddInputField("Max Nodes", fmt.Sprintf("%d", cfg.MaxNodes), 8, nil, nil)
	form.AddInputField("Max Baud", fmt.Sprintf("%d", cfg.MaxBaud), 10, nil, nil)
	form.AddInputField("New User Security", fmt.Sprintf("%d", cfg.NewUserSecurity), 8, nil, nil)
	form.AddInputField("Sysop Security", fmt.Sprintf("%d", cfg.SysopSecurity), 8, nil, nil)
	form.AddInputField("Max Time / Logon (min)", fmt.Sprintf("%d", cfg.MaxTimePerLogon), 8, nil, nil)
	form.AddInputField("New User Time Limit (min)", fmt.Sprintf("%d", cfg.NewUserTimeLimit), 8, nil, nil)
	form.AddCheckbox("Allow Aliases", cfg.AllowAliases, nil)
	form.AddCheckbox("Allow 300 Baud", cfg.Allow300Baud, nil)
	form.AddCheckbox("Allow 1200 Baud", cfg.Allow1200Baud, nil)
	form.AddCheckbox("Allow 2400 Baud", cfg.Allow2400Baud, nil)
	protoOpts := []string{"X", "Y", "G", "Z", "K"}
	protoIdx := 3
	for i, p := range protoOpts {
		if p[0] == cfg.DefaultProtocol {
			protoIdx = i
		}
	}
	form.AddDropDown("Default Protocol", protoOpts, protoIdx, nil)
	form.AddInputField("BBS Start Date", cfg.BBSStartDate, 16, nil, nil)
	form.AddCheckbox("Enable Caller ID", cfg.EnableCallerID, nil)
	form.AddCheckbox("Block No Caller ID", cfg.BlockNoCallerID, nil)
	form.AddCheckbox("Block Blocked Caller ID", cfg.BlockBlockedCID, nil)
	form.AddInputField("Default Country", cfg.DefaultCountry, 24, nil, nil)

	form.AddButton("Save", func() {
		cfg.BoardName = formText(form, "Board Name")
		cfg.SysopName = formText(form, "Sysop Name")
		cfg.SystemPassword = formText(form, "System Password")
		cfg.MaxNodes = atoiOr(formText(form, "Max Nodes"), cfg.MaxNodes)
		cfg.MaxBaud = atoiOr(formText(form, "Max Baud"), cfg.MaxBaud)
		cfg.NewUserSecurity = atoiOr(formText(form, "New User Security"), cfg.NewUserSecurity)
		cfg.SysopSecurity = atoiOr(formText(form, "Sysop Security"), cfg.SysopSecurity)
		cfg.MaxTimePerLogon = atoiOr(formText(form, "Max Time / Logon (min)"), cfg.MaxTimePerLogon)
		cfg.NewUserTimeLimit = atoiOr(formText(form, "New User Time Limit (min)"), cfg.NewUserTimeLimit)
		cfg.AllowAliases = formChecked(form, "Allow Aliases")
		cfg.Allow300Baud = formChecked(form, "Allow 300 Baud")
		cfg.Allow1200Baud = formChecked(form, "Allow 1200 Baud")
		cfg.Allow2400Baud = formChecked(form, "Allow 2400 Baud")
		if _, p := formDropdown(form, "Default Protocol"); p != "" {
			cfg.DefaultProtocol = p[0]
		}
		cfg.BBSStartDate = formText(form, "BBS Start Date")
		cfg.EnableCallerID = formChecked(form, "Enable Caller ID")
		cfg.BlockNoCallerID = formChecked(form, "Block No Caller ID")
		cfg.BlockBlockedCID = formChecked(form, "Block Blocked Caller ID")
		cfg.DefaultCountry = formText(form, "Default Country")

		if err := a.storage.SaveConfig(cfg); err != nil {
			a.errorModal(fmt.Sprintf("Save failed: %v", err))
			return
		}
		a.setStatus("[green]Configuration saved.[white]")
	})
	form.AddButton("Reload", a.showConfig)
	a.setContent(form)
	a.setStatus("[::b]Tab[::-] fields  [::b]Save/Reload[::-]  [::b]Esc[::-] menu")
}

// ---- Callers log ---------------------------------------------------------

func (a *adminApp) showCallerLog() {
	entries, err := a.storage.GetCallerLog(500)
	if err != nil {
		a.errorModal(fmt.Sprintf("Get caller log failed: %v", err))
		return
	}
	view := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(false)
	view.SetBorder(true).SetTitle(fmt.Sprintf(" Callers Log (%d, newest first) ", len(entries)))
	if len(entries) == 0 {
		view.SetText("[gray](no caller log entries)[white]")
	} else {
		view.SetText(strings.Join(entries, "\n"))
	}
	view.ScrollToBeginning()
	a.setContent(view)
	a.setStatus("[::b]Up/Down/PgUp/PgDn[::-] scroll  [::b]Esc[::-] menu")
}

// ---- Maintenance ---------------------------------------------------------

func (a *adminApp) showMaintenance() {
	list := tview.NewList().ShowSecondaryText(true)
	list.SetBorder(true).SetTitle(" Maintenance ")

	list.AddItem("Pack Users", "Permanently remove users flagged as deleted", 'p', func() {
		a.confirm("Permanently purge all deleted-flagged users?", func() {
			n, err := a.storage.PackUsers()
			if err != nil {
				a.errorModal(fmt.Sprintf("Pack failed: %v", err))
				return
			}
			a.setStatus(fmt.Sprintf("[green]Packed users: %d record(s) removed.[white]", n))
		})
	})

	list.AddItem("Refresh Conference Counts", "Recompute each conference's message_count from posts", 'r', func() {
		a.confirm("Recompute message counts for all conferences?", func() {
			a.refreshConferenceCounts()
		})
	})

	a.setContent(list)
	a.setStatus("[::b]Enter[::-] run task  [::b]Esc[::-] menu")
}

// refreshConferenceCounts recomputes each conference's cached message_count
// from the posts table via the storage layer.
func (a *adminApp) refreshConferenceCounts() {
	if err := a.storage.RefreshConferenceMessageCounts(); err != nil {
		a.errorModal(fmt.Sprintf("Refresh failed: %v", err))
		return
	}
	confs, _ := a.storage.GetConferences()
	a.setStatus(fmt.Sprintf("[green]Refreshed message counts for %d conference(s).[white]", len(confs)))
}
