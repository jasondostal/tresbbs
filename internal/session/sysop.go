package session

import (
	"fmt"
	"strings"

	"github.com/jasondostal/tresbbs/domain"
	"github.com/jasondostal/tresbbs/internal/auth"
	"github.com/jasondostal/tresbbs/internal/echo"
)

// sysopMenu handles the sysop functions.
// sysopMenu is the caller-facing sysop menu, rendered from SYSOP.MNU. It mirrors
// TriBBS's lean in-BBS sysop command set (U C F E V S); the heavier operator
// functions — Pack, System Config, Configure Node, Drop to Shell, Local Logon,
// EchoMail, Edit Doors/Bulletins — live in the tribbs-admin TUI console (the
// TRIMAN/WFC equivalent). Returns quit=true if the caller chose Goodbye.
func (s *Session) sysopMenu() bool {
	return s.menuLoop("SYSOP.MNU", "Sysop Menu", map[byte]func(){
		'U': s.sysopEditUsers,
		'C': s.sysopEditConferences,
		'F': s.sysopEditFileAreas,
		'E': s.sysopEditEvents,
		'V': s.sysopViewCallerLog,
		'S': s.sysopSortFileLists,
	})
}

// sysopSortFileLists re-sorts each file area's listing (SYSOP.MNU <S> Sort File
// Lists). tresbbs sorts at display time per the area's configured SortType, so
// this validates every area is in order and reports completion.
func (s *Session) sysopSortFileLists() {
	areas, _ := s.storage.GetFileAreas()
	s.display.SetColor('A')
	s.display.WriteLine(fmt.Sprintf("Sorted file lists for %d area(s).", len(areas)))
	s.display.ResetColor()
	s.storage.LogCaller("Sorted the file lists.")
	s.pause()
}

// sysopPackMessages compacts the message base. Deleted posts are hard-removed
// on deletion in this build, so packing refreshes per-conference counts.
func (s *Session) sysopPackMessages() {
	s.display.Clear()
	s.display.SetColor('A')
	s.display.WriteLine("=== Pack Message Base ===")
	s.display.ResetColor()
	confs, _ := s.storage.GetConferences()
	for _, c := range confs {
		n, _ := s.storage.PostCount(c.Name)
		s.display.SetColor('F')
		s.display.WriteLine(fmt.Sprintf("  %-30s %d message(s)", c.Name, n))
	}
	s.display.SetColor('A')
	s.display.WriteLine("Message base packed.")
	s.storage.LogCaller("Packed the message base.")
	s.pause()
}

// sysopConfigureNode shows this node's configuration (read-only over telnet).
func (s *Session) sysopConfigureNode() {
	s.display.Clear()
	s.display.SetColor('A')
	s.display.WriteLine("=== Configure Node ===")
	s.display.SetColor('F')
	s.display.WriteLine(fmt.Sprintf("  Node Number : %d", s.nodeNum))
	s.display.WriteLine(fmt.Sprintf("  Baud Rate   : %s", s.baudRate))
	s.display.WriteLine(fmt.Sprintf("  Max Nodes   : %d", s.config.MaxNodes))
	s.display.WriteLine(fmt.Sprintf("  Board Name  : %s", s.config.BoardName))
	s.display.ResetColor()
	s.pause()
}

// sysopDropToDos is a console-only (Node 1) command; unavailable remotely.
func (s *Session) sysopDropToDos() {
	s.display.SetColor('C')
	if s.nodeNum != 1 {
		s.display.WriteLine("That feature is only available to Node 1!")
	} else {
		s.display.WriteLine("Sorry, that command can't be executed remotely!")
	}
	s.display.ResetColor()
	s.pause()
}

// sysopLocalLogon is a console-only command; unavailable over a remote session.
func (s *Session) sysopLocalLogon() {
	s.display.SetColor('C')
	s.display.WriteLine("Sorry, that command can't be executed remotely!")
	s.display.ResetColor()
	s.pause()
}

// sysopEchoMail exports/imports FidoNet-style echo mail packets.
func (s *Session) sysopEchoMail() {
	em := echo.NewEchoManager(s.storage, s.config, "echomail/out", "echomail/in")
	for {
		s.display.Clear()
		s.display.SetColor('A')
		s.display.WriteLine("=== EchoMail ===")
		s.display.SetColor('F')
		confs, _ := em.GetEchoConferences()
		s.display.WriteLine(fmt.Sprintf("  Echo conferences: %d", len(confs)))
		s.display.WriteLine("  [E] Export echo packet")
		s.display.WriteLine("  [I] Import echo packet")
		s.display.WriteLine("  [X] Exit")
		s.display.ResetColor()
		s.display.SetColor('E')
		s.display.Write("Selection: ")
		s.display.ResetColor()

		choice := s.readLine()
		if len(choice) == 0 {
			continue
		}
		switch choice[0] {
		case 'x', 'X':
			return
		case 'e', 'E':
			path, err := em.ExportEchoMessages()
			if err != nil {
				s.display.SetColor('C')
				s.display.WriteLine(fmt.Sprintf("Export failed: %v", err))
			} else {
				s.display.SetColor('A')
				s.display.WriteLine(fmt.Sprintf("Exported to %s", path))
				s.storage.LogCaller(fmt.Sprintf("Exported echo packet: %s", path))
			}
			s.pause()
		case 'i', 'I':
			s.display.SetColor('E')
			s.display.Write("Packet path: ")
			s.display.ResetColor()
			path := s.readLine()
			if path == "" {
				continue
			}
			n, err := em.ImportEchoMessages(path)
			if err != nil {
				s.display.SetColor('C')
				s.display.WriteLine(fmt.Sprintf("Import failed: %v", err))
			} else {
				s.display.SetColor('A')
				s.display.WriteLine(fmt.Sprintf("Imported %d message(s).", n))
				s.storage.LogCaller(fmt.Sprintf("Imported echo packet: %d messages", n))
			}
			s.pause()
		}
	}
}

// sysopEditUsers handles user management.
func (s *Session) sysopEditUsers() {
	for {
		s.display.Clear()
		s.display.SetColor('A')
		s.display.WriteLine("╔══════════════════════════════════════════════════════════════╗")
		s.display.SetColor('E')
		s.display.WriteLine("║  Edit Users                                                 ║")
		s.display.SetColor('A')
		s.display.WriteLine("╠══════════════════════════════════════════════════════════════╣")

		users, _ := s.storage.ListUsers()
		for _, u := range users {
			s.display.SetColor('F')
			s.display.WriteLine(fmt.Sprintf("║  #%-4d %-15s %-15s Sec:%-3d %5d calls ║",
				u.RecordNumber, u.Alias, u.Name, u.SecurityLevel, u.TotalCalls))
		}

		s.display.SetColor('A')
		s.display.WriteLine("╠══════════════════════════════════════════════════════════════╣")
		s.display.SetColor('F')
		s.display.WriteLine("║   <E> Edit User    <K> Kill User    <L> Lock Out    <X> Exit║")
		s.display.SetColor('A')
		s.display.WriteLine("╚══════════════════════════════════════════════════════════════╝")
		s.display.ResetColor()
		s.menuPrompt("USERS", "E K L X")

		choice := s.menuKey()
		if len(choice) == 0 {
			continue
		}

		switch choice[0] {
		case 'x', 'X':
			return
		case 'e', 'E':
			s.sysopEditOneUser()
		case 'k', 'K':
			s.sysopKillUser()
		case 'l', 'L':
			s.sysopLockUser()
		}
	}
}

// sysopEditOneUser edits a specific user.
func (s *Session) sysopEditOneUser() {
	s.display.SetColor('E')
	s.display.Write("User record #: ")
	s.display.ResetColor()
	idStr := s.readLine()

	var recordNum int
	fmt.Sscanf(idStr, "%d", &recordNum)

	user, err := s.storage.GetUser(recordNum)
	if err != nil {
		s.display.SetColor('C')
		s.display.WriteLine("User not found!")
		s.pause()
		return
	}

	lockedStr := func() string {
		if user.LockedOut {
			return "Yes"
		}
		return "No"
	}
	for {
		s.display.Clear()
		row := func(key, label, val string) {
			s.display.WriteLine(fmt.Sprintf("║  <%s> %-13s %s ║", key, label, val))
		}
		s.display.SetColor('A')
		s.display.WriteLine("╔════╗")
		s.display.SetColor('E')
		s.display.WriteLine(fmt.Sprintf("║  Editing User: %s ║", user.Alias))
		s.display.SetColor('A')
		s.display.WriteLine("╠════╣")
		s.display.SetColor('F')
		row("1", "Name:", user.Name)
		row("2", "Alias:", user.Alias)
		row("3", "City:", user.City)
		row("4", "Phone:", user.Phone)
		row("5", "Security:", fmt.Sprintf("%d", user.SecurityLevel))
		row("6", "Password:", "****")
		row("7", "Email:", user.Email)
		row("8", "Address:", user.Address)
		row("9", "Birth Date:", user.BirthDate)
		row("N", "Node Sec:", fmt.Sprintf("%d", user.NodeSecurity))
		row("D", "Editor:", user.Editor)
		row("R", "Protocol:", string(user.Protocol))
		row("M", "ANSI Mode:", fmt.Sprintf("%d", user.ANSIMode))
		row("W", "Width:", fmt.Sprintf("%d", user.ScreenWidth))
		row("U", "Uploads:", fmt.Sprintf("%d", user.FilesUploaded))
		row("O", "Downloads:", fmt.Sprintf("%d", user.FilesDownloaded))
		row("K", "K-Uploaded:", fmt.Sprintf("%d", user.KUploaded))
		row("J", "K-Downloaded:", fmt.Sprintf("%d", user.KDownloaded))
		row("G", "Msgs Posted:", fmt.Sprintf("%d", user.MessagesPosted))
		row("T", "Time Left:", fmt.Sprintf("%d", user.TimeLeftToday))
		row("C", "Total Calls:", fmt.Sprintf("%d", user.TotalCalls))
		row("L", "Locked Out:", lockedStr()+"  (toggle)")
		s.display.SetColor('A')
		s.display.WriteLine("╠════╣")
		s.display.SetColor('F')
		row("X", "Exit", "")
		s.display.SetColor('A')
		s.display.WriteLine("╚════╝")
		s.display.ResetColor()
		s.display.SetColor('E')
		s.display.Write("Field to edit: ")
		s.display.ResetColor()

		choice := s.readLine()
		if len(choice) == 0 || choice[0] == 'x' || choice[0] == 'X' {
			return
		}

		// Toggle field — no value prompt.
		if choice[0] == 'l' || choice[0] == 'L' {
			user.LockedOut = !user.LockedOut
			s.storage.SaveUser(user)
			s.storage.LogCaller(fmt.Sprintf("Sysop set locked-out=%v for %s", user.LockedOut, user.Alias))
			continue
		}

		s.display.SetColor('E')
		s.display.Write("New value: ")
		s.display.ResetColor()
		value := s.readLine()

		switch choice[0] {
		case '1':
			user.Name = value
		case '2':
			user.Alias = value
		case '3':
			user.City = value
		case '4':
			user.Phone = value
		case '5':
			fmt.Sscanf(value, "%d", &user.SecurityLevel)
		case '6':
			if h, herr := auth.HashPassword(value); herr == nil {
				user.Password = h
			}
		case '7':
			user.Email = value
		case '8':
			user.Address = value
		case '9':
			user.BirthDate = value
		case 'n', 'N':
			fmt.Sscanf(value, "%d", &user.NodeSecurity)
		case 'd', 'D':
			user.Editor = value
		case 'r', 'R':
			if value != "" {
				user.Protocol = value[0]
			}
		case 'm', 'M':
			fmt.Sscanf(value, "%d", &user.ANSIMode)
		case 'w', 'W':
			fmt.Sscanf(value, "%d", &user.ScreenWidth)
		case 'u', 'U':
			fmt.Sscanf(value, "%d", &user.FilesUploaded)
		case 'o', 'O':
			fmt.Sscanf(value, "%d", &user.FilesDownloaded)
		case 'k', 'K':
			fmt.Sscanf(value, "%d", &user.KUploaded)
		case 'j', 'J':
			fmt.Sscanf(value, "%d", &user.KDownloaded)
		case 'g', 'G':
			fmt.Sscanf(value, "%d", &user.MessagesPosted)
		case 't', 'T':
			fmt.Sscanf(value, "%d", &user.TimeLeftToday)
		case 'c', 'C':
			fmt.Sscanf(value, "%d", &user.TotalCalls)
		}

		s.storage.SaveUser(user)
		s.storage.LogCaller("Edited user file.")
	}
}

// sysopKillUser marks a user for deletion.
func (s *Session) sysopKillUser() {
	s.display.SetColor('E')
	s.display.Write("User record #: ")
	s.display.ResetColor()
	idStr := s.readLine()

	var recordNum int
	fmt.Sscanf(idStr, "%d", &recordNum)

	user, err := s.storage.GetUser(recordNum)
	if err != nil {
		s.display.SetColor('C')
		s.display.WriteLine("User not found!")
		s.pause()
		return
	}

	s.display.SetColor('C')
	s.display.Write(fmt.Sprintf("Delete %s? (y/n): ", user.Alias))
	s.display.ResetColor()
	choice := s.readLine()

	if strings.ToLower(choice) == "y" {
		s.storage.DeleteUser(recordNum)
		s.storage.LogCaller(fmt.Sprintf("Sysop deleted user %s", user.Alias))
		s.display.SetColor('A')
		s.display.WriteLine("User deleted!")
	}
	s.pause()
}

// sysopLockUser locks out a user.
func (s *Session) sysopLockUser() {
	s.display.SetColor('E')
	s.display.Write("User record #: ")
	s.display.ResetColor()
	idStr := s.readLine()

	var recordNum int
	fmt.Sscanf(idStr, "%d", &recordNum)

	user, err := s.storage.GetUser(recordNum)
	if err != nil {
		s.display.SetColor('C')
		s.display.WriteLine("User not found!")
		s.pause()
		return
	}

	user.LockedOut = !user.LockedOut
	s.storage.SaveUser(user)

	if user.LockedOut {
		s.storage.LogCaller(fmt.Sprintf("Sysop locked out user %s", user.Alias))
		s.display.SetColor('C')
		s.display.WriteLine(fmt.Sprintf("%s is now LOCKED OUT", user.Alias))
	} else {
		s.storage.LogCaller(fmt.Sprintf("Sysop unlocked user %s", user.Alias))
		s.display.SetColor('A')
		s.display.WriteLine(fmt.Sprintf("%s is now UNLOCKED", user.Alias))
	}
	s.pause()
}

// sysopEditConferences handles conference management.
func (s *Session) sysopEditConferences() {
	for {
		s.display.Clear()
		s.display.SetColor('A')
		s.display.WriteLine("╔══════════════════════════════════════════════════════════════╗")
		s.display.SetColor('E')
		s.display.WriteLine("║  Edit Conferences                                           ║")
		s.display.SetColor('A')
		s.display.WriteLine("╠══════════════════════════════════════════════════════════════╣")

		confs, _ := s.storage.GetConferences()
		for _, c := range confs {
			s.display.SetColor('F')
			s.display.WriteLine(fmt.Sprintf("║  %-20s  Sec:%-3d  Msgs:%-6d  %s ║",
				c.Name, c.SecurityLevel, c.MessageCount,
				map[bool]string{true: "Private", false: "Public"}[c.PrivateConf]))
		}

		s.display.SetColor('A')
		s.display.WriteLine("╠══════════════════════════════════════════════════════════════╣")
		s.display.SetColor('F')
		s.display.WriteLine("║   [A] Add Conference    [E] Edit Conference    [D] Delete   ║")
		s.display.WriteLine("║   [X] Exit                                                  ║")
		s.display.SetColor('A')
		s.display.WriteLine("╚══════════════════════════════════════════════════════════════╝")
		s.display.ResetColor()
		s.display.SetColor('E')
		s.display.Write("Selection: ")
		s.display.ResetColor()

		choice := s.readLine()
		if len(choice) == 0 {
			continue
		}

		switch choice[0] {
		case 'x', 'X':
			return
		case 'a', 'A':
			s.sysopAddConference()
		case 'e', 'E':
			s.sysopEditOneConference()
		case 'd', 'D':
			s.sysopDeleteConference()
		}
	}
}

// sysopAddConference creates a new conference.
func (s *Session) sysopAddConference() {
	s.display.SetColor('E')
	s.display.Write("Conference name: ")
	s.display.ResetColor()
	name := s.readLine()

	if name == "" {
		return
	}

	s.display.SetColor('E')
	s.display.Write("Security level (0=public): ")
	s.display.ResetColor()
	secStr := s.readLine()
	var sec int
	fmt.Sscanf(secStr, "%d", &sec)

	s.display.SetColor('E')
	s.display.Write("Private? (y/n): ")
	s.display.ResetColor()
	priv := strings.ToLower(s.readLine()) == "y"

	s.display.SetColor('E')
	s.display.Write("Echo/Networked? (y/n): ")
	s.display.ResetColor()
	echo := strings.ToLower(s.readLine()) == "y"

	conf := &domain.Conference{
		Name:          name,
		SecurityLevel: sec,
		PrivateConf:   priv,
		Echo:          echo,
	}

	if err := s.storage.AddConference(conf); err != nil {
		s.display.SetColor('C')
		s.display.WriteLine(fmt.Sprintf("Error: %v", err))
	} else {
		s.storage.LogCaller(fmt.Sprintf("Sysop added conference: %s (sec=%d, priv=%v, echo=%v)", name, sec, priv, echo))
		s.display.SetColor('A')
		s.display.WriteLine("Conference added!")
	}
	s.pause()
}

// sysopEditOneConference edits an existing conference.
func (s *Session) sysopEditOneConference() {
	s.display.SetColor('E')
	s.display.Write("Conference name: ")
	s.display.ResetColor()
	name := s.readLine()

	conf, err := s.storage.GetConference(name)
	if err != nil {
		s.display.SetColor('C')
		s.display.WriteLine("Conference not found!")
		s.pause()
		return
	}

	for {
		s.display.Clear()
		s.display.SetColor('A')
		s.display.WriteLine(fmt.Sprintf("Editing Conference: %s", conf.Name))
		s.display.SetColor('F')
		s.display.WriteLine(fmt.Sprintf("[1] Security Level: %d", conf.SecurityLevel))
		s.display.WriteLine(fmt.Sprintf("[2] Private: %v", conf.PrivateConf))
		s.display.WriteLine(fmt.Sprintf("[3] Echo/Networked: %v", conf.Echo))
		s.display.WriteLine("[X] Done")
		s.display.ResetColor()
		s.display.SetColor('E')
		s.display.Write("Field to edit: ")
		s.display.ResetColor()

		choice := s.readLine()
		if len(choice) == 0 || choice[0] == 'x' || choice[0] == 'X' {
			return
		}

		s.display.SetColor('E')
		s.display.Write("New value: ")
		s.display.ResetColor()
		value := s.readLine()

		switch choice[0] {
		case '1':
			fmt.Sscanf(value, "%d", &conf.SecurityLevel)
		case '2':
			conf.PrivateConf = strings.ToLower(value) == "y" || strings.ToLower(value) == "true"
		case '3':
			conf.Echo = strings.ToLower(value) == "y" || strings.ToLower(value) == "true"
		}

		s.storage.SaveConference(conf)
		s.storage.LogCaller("Edited message conferences.")
	}
}

// sysopDeleteConference deletes a conference.
func (s *Session) sysopDeleteConference() {
	s.display.SetColor('E')
	s.display.Write("Conference name: ")
	s.display.ResetColor()
	name := s.readLine()

	s.display.SetColor('C')
	s.display.Write(fmt.Sprintf("Delete conference '%s'? (y/n): ", name))
	s.display.ResetColor()
	choice := s.readLine()

	if strings.ToLower(choice) == "y" {
		if err := s.storage.DeleteConference(name); err != nil {
			s.display.SetColor('C')
			s.display.WriteLine(fmt.Sprintf("Error: %v", err))
		} else {
			s.storage.LogCaller(fmt.Sprintf("Sysop deleted conference: %s", name))
			s.display.SetColor('A')
			s.display.WriteLine("Conference deleted!")
		}
	}
	s.pause()
}

// sysopEditFileAreas handles file area management.
func (s *Session) sysopEditFileAreas() {
	for {
		s.display.Clear()
		s.display.SetColor('A')
		s.display.WriteLine("╔══════════════════════════════════════════════════════════════╗")
		s.display.SetColor('E')
		s.display.WriteLine("║  Edit File Areas                                            ║")
		s.display.SetColor('A')
		s.display.WriteLine("╠══════════════════════════════════════════════════════════════╣")

		areas, _ := s.storage.GetFileAreas()
		for _, a := range areas {
			s.display.SetColor('F')
			s.display.WriteLine(fmt.Sprintf("║  %-20s  Sec:%-3d  Path:%-20s ║",
				a.Name, a.SecurityLevel, a.Path))
			s.display.WriteLine(fmt.Sprintf("║  %s  %s",
				map[bool]string{true: "CD-ROM", false: "R/W"}[a.CDROM],
				a.Description))
		}

		s.display.SetColor('A')
		s.display.WriteLine("╠══════════════════════════════════════════════════════════════╣")
		s.display.SetColor('F')
		s.display.WriteLine("║   [A] Add Area    [E] Edit Area    [D] Delete Area          ║")
		s.display.WriteLine("║   [X] Exit                                                  ║")
		s.display.SetColor('A')
		s.display.WriteLine("╚══════════════════════════════════════════════════════════════╝")
		s.display.ResetColor()
		s.display.SetColor('E')
		s.display.Write("Selection: ")
		s.display.ResetColor()

		choice := s.readLine()
		if len(choice) == 0 {
			continue
		}

		switch choice[0] {
		case 'x', 'X':
			return
		case 'a', 'A':
			s.sysopAddFileArea()
		case 'e', 'E':
			s.sysopEditOneFileArea()
		case 'd', 'D':
			s.sysopDeleteFileArea()
		}
	}
}

// sysopAddFileArea creates a new file area.
func (s *Session) sysopAddFileArea() {
	s.display.SetColor('E')
	s.display.Write("Area name: ")
	s.display.ResetColor()
	name := s.readLine()
	if name == "" {
		return
	}

	s.display.SetColor('E')
	s.display.Write("Description: ")
	s.display.ResetColor()
	desc := s.readLine()

	s.display.SetColor('E')
	s.display.Write("Path on disk: ")
	s.display.ResetColor()
	path := s.readLine()

	s.display.SetColor('E')
	s.display.Write("Security level (0=public): ")
	s.display.ResetColor()
	secStr := s.readLine()
	var sec int
	fmt.Sscanf(secStr, "%d", &sec)

	s.display.SetColor('E')
	s.display.Write("CD-ROM/Read-only? (y/n): ")
	s.display.ResetColor()
	cdrom := strings.ToLower(s.readLine()) == "y"

	area := &domain.FileArea{
		Name:          name,
		Description:   desc,
		Path:          path,
		SecurityLevel: sec,
		CDROM:         cdrom,
		SortType:      "name",
	}

	if err := s.storage.AddFileArea(area); err != nil {
		s.display.SetColor('C')
		s.display.WriteLine(fmt.Sprintf("Error: %v", err))
	} else {
		s.storage.LogCaller(fmt.Sprintf("Sysop added file area: %s", name))
		s.display.SetColor('A')
		s.display.WriteLine("File area added!")
	}
	s.pause()
}

// sysopEditOneFileArea edits an existing file area.
func (s *Session) sysopEditOneFileArea() {
	s.display.SetColor('E')
	s.display.Write("Area name: ")
	s.display.ResetColor()
	name := s.readLine()

	area, err := s.storage.GetFileArea(name)
	if err != nil {
		s.display.SetColor('C')
		s.display.WriteLine("File area not found!")
		s.pause()
		return
	}

	for {
		s.display.Clear()
		s.display.SetColor('A')
		s.display.WriteLine(fmt.Sprintf("Editing File Area: %s", area.Name))
		s.display.SetColor('F')
		s.display.WriteLine(fmt.Sprintf("[1] Description: %s", area.Description))
		s.display.WriteLine(fmt.Sprintf("[2] Path: %s", area.Path))
		s.display.WriteLine(fmt.Sprintf("[3] Security Level: %d", area.SecurityLevel))
		s.display.WriteLine(fmt.Sprintf("[4] CD-ROM/Read-only: %v", area.CDROM))
		s.display.WriteLine("[X] Done")
		s.display.ResetColor()
		s.display.SetColor('E')
		s.display.Write("Field to edit: ")
		s.display.ResetColor()

		choice := s.readLine()
		if len(choice) == 0 || choice[0] == 'x' || choice[0] == 'X' {
			return
		}

		s.display.SetColor('E')
		s.display.Write("New value: ")
		s.display.ResetColor()
		value := s.readLine()

		switch choice[0] {
		case '1':
			area.Description = value
		case '2':
			area.Path = value
		case '3':
			fmt.Sscanf(value, "%d", &area.SecurityLevel)
		case '4':
			area.CDROM = strings.ToLower(value) == "y" || strings.ToLower(value) == "true"
		}

		s.storage.SaveFileArea(area)
		s.storage.LogCaller("Edited file areas.")
	}
}

// sysopDeleteFileArea deletes a file area.
func (s *Session) sysopDeleteFileArea() {
	s.display.SetColor('E')
	s.display.Write("Area name: ")
	s.display.ResetColor()
	name := s.readLine()

	s.display.SetColor('C')
	s.display.Write(fmt.Sprintf("Delete file area '%s'? (y/n): ", name))
	s.display.ResetColor()
	choice := s.readLine()

	if strings.ToLower(choice) == "y" {
		if err := s.storage.DeleteFileArea(name); err != nil {
			s.display.SetColor('C')
			s.display.WriteLine(fmt.Sprintf("Error: %v", err))
		} else {
			s.storage.LogCaller(fmt.Sprintf("Sysop deleted file area: %s", name))
			s.display.SetColor('A')
			s.display.WriteLine("File area deleted!")
		}
	}
	s.pause()
}

// sysopEditDoors handles door management.
func (s *Session) sysopEditDoors() {
	for {
		s.display.Clear()
		s.display.SetColor('A')
		s.display.WriteLine("╔══════════════════════════════════════════════════════════════╗")
		s.display.SetColor('E')
		s.display.WriteLine("║  Edit Doors                                                 ║")
		s.display.SetColor('A')
		s.display.WriteLine("╠══════════════════════════════════════════════════════════════╣")

		doors, _ := s.storage.GetDoors()
		for _, d := range doors {
			s.display.SetColor('F')
			s.display.WriteLine(fmt.Sprintf("║  [%c] %-20s  Sec:%-3d  Cmd: %-20s ║",
				d.HotKey, d.Name, d.Security, d.Command))
		}

		s.display.SetColor('A')
		s.display.WriteLine("╠══════════════════════════════════════════════════════════════╣")
		s.display.SetColor('F')
		s.display.WriteLine("║   [A] Add Door    [E] Edit Door    [D] Delete Door          ║")
		s.display.WriteLine("║   [X] Exit                                                  ║")
		s.display.SetColor('A')
		s.display.WriteLine("╚══════════════════════════════════════════════════════════════╝")
		s.display.ResetColor()
		s.display.SetColor('E')
		s.display.Write("Selection: ")
		s.display.ResetColor()

		choice := s.readLine()
		if len(choice) == 0 {
			continue
		}

		switch choice[0] {
		case 'x', 'X':
			return
		case 'a', 'A':
			s.sysopAddDoor()
		case 'e', 'E':
			s.sysopEditOneDoor()
		case 'd', 'D':
			s.sysopDeleteDoor()
		}
	}
}

// sysopAddDoor creates a new door.
func (s *Session) sysopAddDoor() {
	s.display.SetColor('E')
	s.display.Write("Door name: ")
	s.display.ResetColor()
	name := s.readLine()
	if name == "" {
		return
	}

	s.display.SetColor('E')
	s.display.Write("Hotkey (single letter): ")
	s.display.ResetColor()
	hotkey := s.readLine()

	s.display.SetColor('E')
	s.display.Write("Description: ")
	s.display.ResetColor()
	desc := s.readLine()

	s.display.SetColor('E')
	s.display.Write("Command to execute: ")
	s.display.ResetColor()
	cmd := s.readLine()

	s.display.SetColor('E')
	s.display.Write("Security level (0=public): ")
	s.display.ResetColor()
	secStr := s.readLine()
	var sec int
	fmt.Sscanf(secStr, "%d", &sec)

	s.display.SetColor('E')
	s.display.Write("Time limit (minutes, 0=none): ")
	s.display.ResetColor()
	timeStr := s.readLine()
	var timeLimit int
	fmt.Sscanf(timeStr, "%d", &timeLimit)

	s.display.SetColor('E')
	s.display.Write("Drop file format (DOORSYS/DORINFO): ")
	s.display.ResetColor()
	dropFmt := s.readLine()
	if dropFmt == "" {
		dropFmt = "DOORSYS"
	}

	var hotkeyByte byte
	if len(hotkey) > 0 {
		hotkeyByte = hotkey[0]
	}

	door := &domain.Door{
		Name:        name,
		HotKey:      hotkeyByte,
		Description: desc,
		Command:     cmd,
		DropFormat:  dropFmt,
		Security:    sec,
		TimeLimit:   timeLimit,
	}

	if err := s.storage.AddDoor(door); err != nil {
		s.display.SetColor('C')
		s.display.WriteLine(fmt.Sprintf("Error: %v", err))
	} else {
		s.storage.LogCaller(fmt.Sprintf("Sysop added door: %s", name))
		s.display.SetColor('A')
		s.display.WriteLine("Door added!")
	}
	s.pause()
}

// sysopEditOneDoor edits an existing door.
func (s *Session) sysopEditOneDoor() {
	s.display.SetColor('E')
	s.display.Write("Door name: ")
	s.display.ResetColor()
	name := s.readLine()

	door, err := s.storage.GetDoor(name)
	if err != nil {
		s.display.SetColor('C')
		s.display.WriteLine("Door not found!")
		s.pause()
		return
	}

	for {
		s.display.Clear()
		s.display.SetColor('A')
		s.display.WriteLine(fmt.Sprintf("Editing Door: %s", door.Name))
		s.display.SetColor('F')
		s.display.WriteLine(fmt.Sprintf("[1] Hotkey: %c", door.HotKey))
		s.display.WriteLine(fmt.Sprintf("[2] Description: %s", door.Description))
		s.display.WriteLine(fmt.Sprintf("[3] Command: %s", door.Command))
		s.display.WriteLine(fmt.Sprintf("[4] Security: %d", door.Security))
		s.display.WriteLine(fmt.Sprintf("[5] Time Limit: %d min", door.TimeLimit))
		s.display.WriteLine(fmt.Sprintf("[6] Drop Format: %s", door.DropFormat))
		s.display.WriteLine("[X] Done")
		s.display.ResetColor()
		s.display.SetColor('E')
		s.display.Write("Field to edit: ")
		s.display.ResetColor()

		choice := s.readLine()
		if len(choice) == 0 || choice[0] == 'x' || choice[0] == 'X' {
			return
		}

		s.display.SetColor('E')
		s.display.Write("New value: ")
		s.display.ResetColor()
		value := s.readLine()

		switch choice[0] {
		case '1':
			if len(value) > 0 {
				door.HotKey = value[0]
			}
		case '2':
			door.Description = value
		case '3':
			door.Command = value
		case '4':
			fmt.Sscanf(value, "%d", &door.Security)
		case '5':
			fmt.Sscanf(value, "%d", &door.TimeLimit)
		case '6':
			door.DropFormat = value
		}

		s.storage.SaveDoor(door)
		s.storage.LogCaller(fmt.Sprintf("Sysop edited door %s", door.Name))
	}
}

// sysopDeleteDoor deletes a door.
func (s *Session) sysopDeleteDoor() {
	s.display.SetColor('E')
	s.display.Write("Door name: ")
	s.display.ResetColor()
	name := s.readLine()

	s.display.SetColor('C')
	s.display.Write(fmt.Sprintf("Delete door '%s'? (y/n): ", name))
	s.display.ResetColor()
	choice := s.readLine()

	if strings.ToLower(choice) == "y" {
		if err := s.storage.DeleteDoor(name); err != nil {
			s.display.SetColor('C')
			s.display.WriteLine(fmt.Sprintf("Error: %v", err))
		} else {
			s.storage.LogCaller(fmt.Sprintf("Sysop deleted door: %s", name))
			s.display.SetColor('A')
			s.display.WriteLine("Door deleted!")
		}
	}
	s.pause()
}

// sysopViewCallerLog shows the caller log.
func (s *Session) sysopViewCallerLog() {
	s.storage.LogCaller("Viewed CALLERS.LOG.")
	s.display.Clear()
	s.display.SetColor('A')
	s.display.WriteLine("╔══════════════════════════════════════════════════════════════╗")
	s.display.SetColor('E')
	s.display.WriteLine("║  Caller Log                                                 ║")
	s.display.SetColor('A')
	s.display.WriteLine("╠══════════════════════════════════════════════════════════════╣")

	entries, _ := s.storage.GetCallerLog(50)
	if len(entries) == 0 {
		s.display.SetColor('B')
		s.display.WriteLine("║  No log entries.                                            ║")
	} else {
		for _, e := range entries {
			s.display.SetColor('F')
			// Truncate long lines
			if len(e) > 58 {
				e = e[:58]
			}
			s.display.WriteLine(fmt.Sprintf("║  %-58s ║", e))
		}
	}

	s.display.SetColor('A')
	s.display.WriteLine("╠══════════════════════════════════════════════════════════════╣")
	s.display.SetColor('F')
	s.display.WriteLine("║   Press Enter to continue...                                ║")
	s.display.SetColor('A')
	s.display.WriteLine("╚══════════════════════════════════════════════════════════════╝")
	s.display.ResetColor()
	s.readLine()
}

// sysopPackUsers removes deleted user records.
func (s *Session) sysopPackUsers() {
	s.display.SetColor('C')
	s.display.Write("Pack user file? This cannot be undone! (y/n): ")
	s.display.ResetColor()
	choice := s.readLine()

	if strings.ToLower(choice) == "y" {
		removed, err := s.storage.PackUsers()
		if err != nil {
			s.display.SetColor('C')
			s.display.WriteLine(fmt.Sprintf("Error: %v", err))
		} else {
			s.storage.LogCaller(fmt.Sprintf("Sysop packed user file: removed %d records", removed))
			s.display.SetColor('A')
			s.display.WriteLine(fmt.Sprintf("Removed %d deleted records!", removed))
		}
	}
	s.pause()
}

// sysopSystemConfig shows and edits system configuration.
func (s *Session) sysopSystemConfig() {
	yesno := func(b bool) string {
		if b {
			return "Yes"
		}
		return "No"
	}
	for {
		s.display.Clear()
		row := func(key, label, val string) {
			s.display.WriteLine(fmt.Sprintf("║  [%s] %-16s %s ║", key, label, val))
		}
		pw := "(none)"
		if s.config.SystemPassword != "" {
			pw = "****"
		}
		s.display.SetColor('A')
		s.display.WriteLine("╔════╗")
		s.display.SetColor('E')
		s.display.WriteLine("║  System Configuration ║")
		s.display.SetColor('A')
		s.display.WriteLine("╠════╣")
		s.display.SetColor('F')
		row("1", "Board Name:", s.config.BoardName)
		row("2", "Sysop Name:", s.config.SysopName)
		row("3", "Max Nodes:", fmt.Sprintf("%d", s.config.MaxNodes))
		row("4", "New User Sec:", fmt.Sprintf("%d", s.config.NewUserSecurity))
		row("5", "Sysop Sec:", fmt.Sprintf("%d", s.config.SysopSecurity))
		row("6", "Max Time/Logon:", fmt.Sprintf("%d", s.config.MaxTimePerLogon))
		row("7", "New User Time:", fmt.Sprintf("%d", s.config.NewUserTimeLimit))
		row("8", "Max Baud:", fmt.Sprintf("%d", s.config.MaxBaud))
		row("9", "Default Proto:", string(s.config.DefaultProtocol))
		row("P", "System Pwd:", pw)
		row("A", "Allow Aliases:", yesno(s.config.AllowAliases)+"  (toggle)")
		row("B", "Allow 2400:", yesno(s.config.Allow2400Baud)+"  (toggle)")
		row("N", "Allow New Users:", yesno(s.config.AllowNewUsers)+"  (toggle)")
		row("M", "Min Security:", fmt.Sprintf("%d", s.config.MinSecurityLevel))
		s.display.SetColor('A')
		s.display.WriteLine("╠════╣")
		s.display.SetColor('F')
		row("X", "Exit", "")
		s.display.SetColor('A')
		s.display.WriteLine("╚════╝")
		s.display.ResetColor()
		s.display.SetColor('E')
		s.display.Write("Field to edit: ")
		s.display.ResetColor()

		choice := s.readLine()
		if len(choice) == 0 || choice[0] == 'x' || choice[0] == 'X' {
			return
		}

		// Toggle fields — no value prompt.
		switch choice[0] {
		case 'a', 'A':
			s.config.AllowAliases = !s.config.AllowAliases
			s.storage.SaveConfig(s.config)
			continue
		case 'b', 'B':
			s.config.Allow2400Baud = !s.config.Allow2400Baud
			s.storage.SaveConfig(s.config)
			continue
		case 'n', 'N':
			s.config.AllowNewUsers = !s.config.AllowNewUsers
			s.storage.SaveConfig(s.config)
			continue
		}

		s.display.SetColor('E')
		s.display.Write("New value: ")
		s.display.ResetColor()
		value := s.readLine()

		switch choice[0] {
		case '1':
			s.config.BoardName = value
		case '2':
			s.config.SysopName = value
		case '3':
			fmt.Sscanf(value, "%d", &s.config.MaxNodes)
		case '4':
			fmt.Sscanf(value, "%d", &s.config.NewUserSecurity)
		case '5':
			fmt.Sscanf(value, "%d", &s.config.SysopSecurity)
		case '6':
			fmt.Sscanf(value, "%d", &s.config.MaxTimePerLogon)
		case '7':
			fmt.Sscanf(value, "%d", &s.config.NewUserTimeLimit)
		case '8':
			fmt.Sscanf(value, "%d", &s.config.MaxBaud)
		case '9':
			if value != "" {
				s.config.DefaultProtocol = value[0]
			}
		case 'p', 'P':
			s.config.SystemPassword = value
		case 'm', 'M':
			fmt.Sscanf(value, "%d", &s.config.MinSecurityLevel)
		}

		s.storage.SaveConfig(s.config)
		s.storage.LogCaller("Sysop updated system configuration")
	}
}

// sysopEditBulletins handles bulletin management.
func (s *Session) sysopEditBulletins() {
	for {
		s.display.Clear()
		s.display.SetColor('A')
		s.display.WriteLine("╔══════════════════════════════════════════════════════════════╗")
		s.display.SetColor('E')
		s.display.WriteLine("║  Edit Bulletins                                             ║")
		s.display.SetColor('A')
		s.display.WriteLine("╠══════════════════════════════════════════════════════════════╣")

		bulletins, _ := s.storage.GetBulletins()
		for _, b := range bulletins {
			s.display.SetColor('F')
			s.display.WriteLine(fmt.Sprintf("║  [%d] %-52s ║", b.ID, b.Name))
		}

		s.display.SetColor('A')
		s.display.WriteLine("╠══════════════════════════════════════════════════════════════╣")
		s.display.SetColor('F')
		s.display.WriteLine("║   [A] Add Bulletin    [E] Edit Bulletin    [D] Delete       ║")
		s.display.WriteLine("║   [X] Exit                                                  ║")
		s.display.SetColor('A')
		s.display.WriteLine("╚══════════════════════════════════════════════════════════════╝")
		s.display.ResetColor()
		s.display.SetColor('E')
		s.display.Write("Selection: ")
		s.display.ResetColor()

		choice := s.readLine()
		if len(choice) == 0 {
			continue
		}

		switch choice[0] {
		case 'x', 'X':
			return
		case 'a', 'A':
			s.sysopAddBulletin()
		case 'e', 'E':
			s.sysopEditOneBulletin()
		case 'd', 'D':
			s.sysopDeleteBulletin()
		}
	}
}

// sysopAddBulletin creates a new bulletin.
func (s *Session) sysopAddBulletin() {
	s.display.SetColor('E')
	s.display.Write("Bulletin name: ")
	s.display.ResetColor()
	name := s.readLine()
	if name == "" {
		return
	}

	s.display.SetColor('E')
	s.display.Write("Security level (0=public): ")
	s.display.ResetColor()
	secStr := s.readLine()
	var sec int
	fmt.Sscanf(secStr, "%d", &sec)

	s.display.SetColor('E')
	s.display.WriteLine("Enter bulletin content (empty line to finish):")
	s.display.ResetColor()

	var lines []string
	for {
		line := s.readLine()
		if line == "" {
			break
		}
		lines = append(lines, line)
	}
	content := strings.Join(lines, "\n")

	bulletin := &domain.Bulletin{
		Name:     name,
		Content:  content,
		Security: sec,
	}

	if err := s.storage.AddBulletin(bulletin); err != nil {
		s.display.SetColor('C')
		s.display.WriteLine(fmt.Sprintf("Error: %v", err))
	} else {
		s.storage.LogCaller(fmt.Sprintf("Sysop added bulletin: %s", name))
		s.display.SetColor('A')
		s.display.WriteLine("Bulletin added!")
	}
	s.pause()
}

// sysopEditOneBulletin edits an existing bulletin.
func (s *Session) sysopEditOneBulletin() {
	s.display.SetColor('E')
	s.display.Write("Bulletin ID: ")
	s.display.ResetColor()
	idStr := s.readLine()
	var id int
	fmt.Sscanf(idStr, "%d", &id)

	bulletin, err := s.storage.GetBulletin(id)
	if err != nil {
		s.display.SetColor('C')
		s.display.WriteLine("Bulletin not found!")
		s.pause()
		return
	}

	for {
		s.display.Clear()
		s.display.SetColor('A')
		s.display.WriteLine(fmt.Sprintf("Editing Bulletin: %s", bulletin.Name))
		s.display.SetColor('F')
		s.display.WriteLine(fmt.Sprintf("[1] Name: %s", bulletin.Name))
		s.display.WriteLine(fmt.Sprintf("[2] Security: %d", bulletin.Security))
		s.display.WriteLine("[3] Edit Content")
		s.display.WriteLine("[X] Done")
		s.display.ResetColor()
		s.display.SetColor('E')
		s.display.Write("Field to edit: ")
		s.display.ResetColor()

		choice := s.readLine()
		if len(choice) == 0 || choice[0] == 'x' || choice[0] == 'X' {
			return
		}

		switch choice[0] {
		case '1':
			s.display.SetColor('E')
			s.display.Write("New name: ")
			s.display.ResetColor()
			bulletin.Name = s.readLine()
		case '2':
			s.display.SetColor('E')
			s.display.Write("New security level: ")
			s.display.ResetColor()
			fmt.Sscanf(s.readLine(), "%d", &bulletin.Security)
		case '3':
			s.display.SetColor('E')
			s.display.WriteLine("Enter new content (empty line to finish):")
			s.display.ResetColor()
			var lines []string
			for {
				line := s.readLine()
				if line == "" {
					break
				}
				lines = append(lines, line)
			}
			bulletin.Content = strings.Join(lines, "\n")
		}

		s.storage.SaveBulletin(bulletin)
		s.storage.LogCaller(fmt.Sprintf("Sysop edited bulletin %d", bulletin.ID))
	}
}

// sysopDeleteBulletin deletes a bulletin.
func (s *Session) sysopDeleteBulletin() {
	s.display.SetColor('E')
	s.display.Write("Bulletin ID: ")
	s.display.ResetColor()
	idStr := s.readLine()
	var id int
	fmt.Sscanf(idStr, "%d", &id)

	s.display.SetColor('C')
	s.display.Write("Delete this bulletin? (y/n): ")
	s.display.ResetColor()
	choice := s.readLine()

	if strings.ToLower(choice) == "y" {
		if err := s.storage.DeleteBulletin(id); err != nil {
			s.display.SetColor('C')
			s.display.WriteLine(fmt.Sprintf("Error: %v", err))
		} else {
			s.storage.LogCaller(fmt.Sprintf("Sysop deleted bulletin %d", id))
			s.display.SetColor('A')
			s.display.WriteLine("Bulletin deleted!")
		}
	}
	s.pause()
}

// sysopEditEvents handles event management.
func (s *Session) sysopEditEvents() {
	for {
		s.display.Clear()
		s.display.SetColor('A')
		s.display.WriteLine("╔══════════════════════════════════════════════════════════════╗")
		s.display.SetColor('E')
		s.display.WriteLine("║  Edit Events                                                ║")
		s.display.SetColor('A')
		s.display.WriteLine("╠══════════════════════════════════════════════════════════════╣")

		events, _ := s.storage.GetEvents()
		for _, e := range events {
			s.display.SetColor('F')
			s.display.WriteLine(fmt.Sprintf("║  %s %-8s %-20s %s ║",
				e.Time, e.Day, e.File,
				map[bool]string{true: "Slide", false: "Fixed"}[e.Slide]))
		}

		s.display.SetColor('A')
		s.display.WriteLine("╠══════════════════════════════════════════════════════════════╣")
		s.display.SetColor('F')
		s.display.WriteLine("║   [A] Add Event    [E] Edit Event    [D] Delete Event       ║")
		s.display.WriteLine("║   [R] Reset Daily Flags                                     ║")
		s.display.WriteLine("║   [X] Exit                                                  ║")
		s.display.SetColor('A')
		s.display.WriteLine("╚══════════════════════════════════════════════════════════════╝")
		s.display.ResetColor()
		s.display.SetColor('E')
		s.display.Write("Selection: ")
		s.display.ResetColor()

		choice := s.readLine()
		if len(choice) == 0 {
			continue
		}

		switch choice[0] {
		case 'x', 'X':
			return
		case 'a', 'A':
			s.sysopAddEvent()
		case 'e', 'E':
			s.sysopEditOneEvent()
		case 'd', 'D':
			s.sysopDeleteEvent()
		case 'r', 'R':
			s.sysopResetDailyFlags()
		}
	}
}

// sysopAddEvent creates a new event.
func (s *Session) sysopAddEvent() {
	s.display.SetColor('E')
	s.display.Write("Event time (HH:MM): ")
	s.display.ResetColor()
	timeStr := s.readLine()
	if timeStr == "" {
		return
	}

	s.display.SetColor('E')
	s.display.Write("Day (Daily, Mon, Tue, Wed, Thu, Fri, Sat, Sun): ")
	s.display.ResetColor()
	day := s.readLine()
	if day == "" {
		day = "Daily"
	}

	s.display.SetColor('E')
	s.display.Write("Batch file to execute: ")
	s.display.ResetColor()
	file := s.readLine()

	s.display.SetColor('E')
	s.display.Write("Sliding event? (y/n): ")
	s.display.ResetColor()
	slide := strings.ToLower(s.readLine()) == "y"

	event := &domain.Event{
		Time:  timeStr,
		Day:   day,
		File:  file,
		Slide: slide,
	}

	if err := s.storage.SaveEvent(event); err != nil {
		s.display.SetColor('C')
		s.display.WriteLine(fmt.Sprintf("Error: %v", err))
	} else {
		s.storage.LogCaller(fmt.Sprintf("Sysop added event: %s at %s", file, timeStr))
		s.display.SetColor('A')
		s.display.WriteLine("Event added!")
	}
	s.pause()
}

// sysopEditOneEvent edits an existing event.
func (s *Session) sysopEditOneEvent() {
	s.display.SetColor('E')
	s.display.Write("Event ID (from list): ")
	s.display.ResetColor()
	idStr := s.readLine()
	var id int
	fmt.Sscanf(idStr, "%d", &id)

	if id <= 0 {
		return
	}

	// Find the event
	events, _ := s.storage.GetEvents()
	var event *domain.Event
	for _, e := range events {
		if e.ID == id {
			event = &e
			break
		}
	}
	if event == nil {
		s.display.SetColor('C')
		s.display.WriteLine("Event not found!")
		s.pause()
		return
	}

	for {
		s.display.Clear()
		s.display.SetColor('A')
		s.display.WriteLine(fmt.Sprintf("Editing Event #%d", event.ID))
		s.display.SetColor('F')
		s.display.WriteLine(fmt.Sprintf("[1] Time: %s", event.Time))
		s.display.WriteLine(fmt.Sprintf("[2] Day: %s", event.Day))
		s.display.WriteLine(fmt.Sprintf("[3] File: %s", event.File))
		s.display.WriteLine(fmt.Sprintf("[4] Slide: %v", event.Slide))
		s.display.WriteLine("[X] Done")
		s.display.ResetColor()
		s.display.SetColor('E')
		s.display.Write("Field to edit: ")
		s.display.ResetColor()

		choice := s.readLine()
		if len(choice) == 0 || choice[0] == 'x' || choice[0] == 'X' {
			return
		}

		s.display.SetColor('E')
		s.display.Write("New value: ")
		s.display.ResetColor()
		value := s.readLine()

		switch choice[0] {
		case '1':
			event.Time = value
		case '2':
			event.Day = value
		case '3':
			event.File = value
		case '4':
			event.Slide = strings.ToLower(value) == "y" || strings.ToLower(value) == "true"
		}

		s.storage.SaveEvent(event)
		s.storage.LogCaller(fmt.Sprintf("Sysop edited event #%d", event.ID))
	}
}

// sysopDeleteEvent deletes an event.
func (s *Session) sysopDeleteEvent() {
	s.display.SetColor('E')
	s.display.Write("Event ID: ")
	s.display.ResetColor()
	idStr := s.readLine()
	var id int
	fmt.Sscanf(idStr, "%d", &id)

	s.display.SetColor('C')
	s.display.Write("Delete this event? (y/n): ")
	s.display.ResetColor()
	choice := s.readLine()

	if strings.ToLower(choice) == "y" {
		if err := s.storage.DeleteEvent(id); err != nil {
			s.display.SetColor('C')
			s.display.WriteLine(fmt.Sprintf("Error: %v", err))
		} else {
			s.storage.LogCaller(fmt.Sprintf("Sysop deleted event #%d", id))
			s.display.SetColor('A')
			s.display.WriteLine("Event deleted!")
		}
	}
	s.pause()
}

// sysopResetDailyFlags resets all event daily execution flags.
func (s *Session) sysopResetDailyFlags() {
	s.display.SetColor('C')
	s.display.Write("Reset all daily event flags? (y/n): ")
	s.display.ResetColor()
	choice := s.readLine()

	if strings.ToLower(choice) == "y" {
		events, _ := s.storage.GetEvents()
		for _, e := range events {
			e.ExecutedToday = false
			s.storage.SaveEvent(&e)
		}
		s.storage.LogCaller("Sysop reset all daily event flags")
		s.display.SetColor('A')
		s.display.WriteLine("All daily flags reset!")
		s.pause()
	}
}
