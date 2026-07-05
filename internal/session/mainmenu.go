package session

import (
	"fmt"
	"strings"

	"github.com/jasondostal/tresbbs/domain"
)

// This file holds the main-menu command handlers that back the real TriBBS
// MAIN.MNU command set (see internal/menu). Each maps a MAIN.MNU item to
// behavior; the rendering/prompt come from the .MNU file itself.

// userSecurity is the caller's security level (0 if not yet logged in).
func (s *Session) userSecurity() int {
	if s.user == nil {
		return 0
	}
	return s.user.SecurityLevel
}

// dispatchMain runs the handler for a MAIN.MNU key. It returns true when the
// session should end (Goodbye). Security is already enforced by the menu, but
// Sysop is re-checked here as defense in depth.
func (s *Session) dispatchMain(key byte) (quit bool) {
	switch upperByte(key) {
	case 'B':
		s.setActivity("Bulletins")
		s.readBulletins()
	case 'M':
		s.setActivity("Messages")
		if s.messageMenu() {
			return true
		}
	case 'F':
		s.setActivity("Files")
		if s.fileMenu() {
			return true
		}
	case 'D':
		s.setActivity("Doors")
		s.doorMenu()
	case 'T':
		s.setActivity("TeleChat")
		s.chatMenu()
	case 'Q':
		s.questionnaire()
	case 'S':
		if s.userSecurity() >= s.config.SysopSecurity {
			s.setActivity("Sysop")
			if s.sysopMenu() {
				return true
			}
		} else {
			s.notAvailable("Sysop access required!")
		}
	case 'C':
		s.commentToSysop()
	case 'A':
		s.toggleANSI()
	case 'R':
		s.toggleRIP()
	case 'N':
		s.newsletter()
	case 'Y':
		s.userConfig()
	case 'I':
		s.systemInfo()
	case 'U':
		s.listUsers()
	case 'W':
		s.whoOnline()
	case 'X':
		s.toggleExpert()
	case 'P':
		s.setActivity("Chat")
		s.chatRequest()
	case 'G':
		s.logoff()
		return true
	}
	return false
}

// menuLoop drives a .MNU-rendered sub-menu (Message/File/Sysop/Door): it renders
// the menu from file, logs the selected item, handles the keys every TriBBS menu
// shares (M=back to Main, G=Goodbye/logoff, X=Expert, P=Page Sysop, ?=redraw),
// and dispatches the menu-specific keys through handlers. It returns quit=true
// when the caller chose Goodbye (end the whole session) or the carrier dropped.
func (s *Session) menuLoop(file, logName string, handlers map[byte]func()) (quit bool) {
	for {
		if s.hangup {
			return true
		}
		key, m := s.showMenu(file)
		key = upperByte(key)
		if key == '?' {
			continue
		}
		if m != nil {
			if it, ok := m.Find(key, s.userSecurity()); ok {
				s.logMenuSelection(logName, it.Description)
			}
		}
		switch key {
		case 'M':
			return false
		case 'G':
			s.logoff()
			return true
		case 'X':
			s.toggleExpert()
			continue
		case 'P':
			s.setActivity("Chat")
			s.chatRequest()
			continue
		}
		if h, ok := handlers[key]; ok {
			h()
		}
	}
}

// setActivity updates this node's advertised activity for who's-online.
func (s *Session) setActivity(activity string) {
	s.nodes.UpdateNode(&domain.NodeStatus{NodeNumber: s.nodeNum, Active: true, Activity: activity})
}

func (s *Session) notAvailable(msg string) {
	s.display.SetColor('C')
	s.display.WriteLine(msg)
	s.display.ResetColor()
	s.pause()
}

// toggleANSI flips ANSI graphics (MAIN.MNU <A>). RIP mode collapses to plain
// ANSI first, matching TriBBS's single graphics toggle.
func (s *Session) toggleANSI() {
	if s.user.ANSIMode == 0 {
		s.user.ANSIMode = 1
	} else {
		s.user.ANSIMode = 0
	}
	s.storage.SaveUser(s.user)
	s.applyGraphicsMode()
	s.display.SetColor('A')
	if s.user.ANSIMode == 0 {
		s.display.WriteLine("ANSI graphics are now OFF.")
	} else {
		s.display.WriteLine("ANSI graphics are now ON.")
	}
	s.display.ResetColor()
	s.pause()
}

// toggleRIP flips RIPscrip graphics (MAIN.MNU <R>).
func (s *Session) toggleRIP() {
	if s.user.ANSIMode == 2 {
		s.user.ANSIMode = 1
	} else {
		s.user.ANSIMode = 2
	}
	s.storage.SaveUser(s.user)
	s.applyGraphicsMode()
	s.display.SetColor('A')
	if s.user.ANSIMode == 2 {
		s.display.WriteLine("RIPscrip graphics are now ON.")
	} else {
		s.display.WriteLine("RIPscrip graphics are now OFF.")
	}
	s.display.ResetColor()
	s.pause()
}

// toggleExpert flips expert mode (MAIN.MNU <X>): suppresses the menu boxes and
// shows only the command prompt.
func (s *Session) toggleExpert() {
	s.expert = !s.expert
	s.display.SetColor('A')
	if s.expert {
		s.display.WriteLine("Expert mode is now ON.")
	} else {
		s.display.WriteLine("Expert mode is now OFF.")
	}
	s.display.ResetColor()
	s.pause()
}

// newsletter shows the board newsletter (MAIN.MNU <N>): the NEWS display file if
// the sysop has one, else a friendly notice.
func (s *Session) newsletter() {
	s.display.Clear()
	if rendered, err := s.templateEngine.RenderFile("newsletter.tpl", s.templateData); err == nil {
		s.writeTemplate(rendered)
	} else {
		s.display.SetColor('E')
		s.display.WriteLine(fmt.Sprintf("%s Newsletter", s.config.BoardName))
		s.display.SetColor('F')
		s.display.WriteLine("")
		s.display.WriteLine("No newsletter is available at this time.")
		s.display.ResetColor()
	}
	s.pause()
}

// systemInfo shows board statistics (MAIN.MNU <I>): the @I System Information
// screen — board identity plus live totals.
func (s *Session) systemInfo() {
	s.display.Clear()
	users, _ := s.storage.UserCount()
	confs, _ := s.storage.GetConferences()
	var posts int64
	for _, c := range confs {
		if n, err := s.storage.PostCount(c.Name); err == nil {
			posts += n
		}
	}

	s.display.SetColor('E')
	s.display.WriteLine(fmt.Sprintf("%s - System Information", s.config.BoardName))
	s.display.SetColor('F')
	s.display.WriteLine("")
	rows := [][2]string{
		{"Board Name", s.config.BoardName},
		{"Sysop", s.config.SysopName},
		{"Software", "TresBBS v" + domain.Version},
		{"In Service Since", s.config.BBSStartDate},
		{"Total Users", fmt.Sprintf("%d", users)},
		{"Conferences", fmt.Sprintf("%d", len(confs))},
		{"Messages", fmt.Sprintf("%d", posts)},
		{"Nodes", fmt.Sprintf("%d", s.config.MaxNodes)},
	}
	for _, r := range rows {
		s.display.WriteLine(fmt.Sprintf("  %-20s: %s", r[0], r[1]))
	}
	s.display.ResetColor()
	s.pause()
}

// listUsers shows the user roster (MAIN.MNU <U>): alias, location, last on.
func (s *Session) listUsers() {
	s.display.Clear()
	users, err := s.storage.ListUsers()
	if err != nil {
		s.notAvailable("User list is unavailable.")
		return
	}
	s.display.SetColor('E')
	s.display.WriteLine(fmt.Sprintf("%s - List of Users", s.config.BoardName))
	s.display.WriteLine("")
	s.display.WriteLine(fmt.Sprintf("  %-24s %-22s %-12s", "Handle", "From", "Last On"))
	s.display.WriteLine(fmt.Sprintf("  %-24s %-22s %-12s", strings.Repeat("-", 24), strings.Repeat("-", 22), strings.Repeat("-", 12)))
	s.display.SetColor('F')
	shown := 0
	for _, u := range users {
		if u.Deleted || u.LockedOut {
			continue
		}
		handle := u.Alias
		if handle == "" {
			handle = u.Name
		}
		loc := u.City
		if u.State != "" {
			loc = u.City + ", " + u.State
		}
		last := ""
		if !u.LastLogin.IsZero() {
			last = u.LastLogin.Format("01/02/2006")
		}
		s.display.WriteLine(fmt.Sprintf("  %-24s %-22s %-12s", trunc(handle, 24), trunc(loc, 22), last))
		shown++
		if shown%20 == 0 {
			s.pause()
		}
	}
	s.display.ResetColor()
	s.display.WriteLine("")
	s.display.SetColor('A')
	s.display.WriteLine(fmt.Sprintf("%d user(s) listed.", shown))
	s.display.ResetColor()
	s.pause()
}

// commentToSysop leaves a comment for the sysop (MAIN.MNU <C>).
func (s *Session) commentToSysop() {
	s.display.Clear()
	s.display.SetColor('E')
	s.display.WriteLine(fmt.Sprintf("Comment to %s", s.config.SysopName))
	s.display.SetColor('F')
	s.display.WriteLine("Enter your comment. Finish with a single '.' on its own line, or blank to abort.")
	s.display.WriteLine("")
	s.display.ResetColor()

	var lines []string
	for {
		line := s.readLine()
		if line == "." {
			break
		}
		if line == "" && len(lines) == 0 {
			s.display.SetColor('C')
			s.display.WriteLine("Comment aborted.")
			s.display.ResetColor()
			s.pause()
			return
		}
		if line == "" {
			// allow blank lines within the body only after content started
			lines = append(lines, "")
			continue
		}
		lines = append(lines, line)
	}

	post := &domain.Post{
		Conference: s.commentConference(),
		Author:     s.user.Name,
		Subject:    "Comment to Sysop",
		Body:       strings.Join(lines, "\n"),
	}
	if err := s.storage.AddPost(post); err != nil {
		s.notAvailable("Could not save your comment.")
		return
	}
	s.storage.LogCaller("Left a comment for the sysop.")
	s.display.SetColor('A')
	s.display.WriteLine("Thank you — your comment has been saved.")
	s.display.ResetColor()
	s.pause()
}

// commentConference picks a conference to file a sysop comment under, preferring
// the caller's current conference, else the first accessible one.
func (s *Session) commentConference() string {
	if s.currentConf != "" {
		return s.currentConf
	}
	confs, _ := s.storage.GetConferences()
	for _, c := range confs {
		if s.canAccessConference(c) {
			return c.Name
		}
	}
	return "General"
}

// questionnaire runs the board questionnaire (MAIN.MNU <Q>). The stock item is
// sysop-only (security 999); a friendly notice covers the no-questionnaire case.
func (s *Session) questionnaire() {
	s.display.Clear()
	s.display.SetColor('E')
	s.display.WriteLine(fmt.Sprintf("%s Questionnaire", s.config.BoardName))
	s.display.SetColor('F')
	s.display.WriteLine("")
	s.display.WriteLine("No questionnaire is available at this time.")
	s.display.ResetColor()
	s.pause()
}

func upperByte(b byte) byte {
	if b >= 'a' && b <= 'z' {
		return b - 32
	}
	return b
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
