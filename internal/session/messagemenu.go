package session

import (
	"fmt"
	"strings"

	"github.com/jasondostal/tresbbs/domain"
)

// Message-menu command handlers backing the real MESSAGE.MNU command set
// (C E R N Y S Q). Read/Change/QWK live in session.go; the scan/search
// commands are here.

// changeConference shows the accessible conferences and joins the one the
// caller picks (MESSAGE.MNU <C> Change Conference).
func (s *Session) changeConference() {
	confs, _ := s.storage.GetConferences()
	s.display.Clear()
	s.display.SetColor('A')
	s.display.WriteLine(fmt.Sprintf("%s - Message Conferences", s.config.BoardName))
	s.display.ResetColor()
	for i, c := range confs {
		if !s.canAccessConference(c) {
			continue
		}
		marker := " "
		if c.Name == s.currentConf {
			marker = "*"
		}
		s.display.SetColor('F')
		s.display.WriteLine(fmt.Sprintf("  %s[%d] %s", marker, i+1, c.Name))
	}
	s.display.SetColor('E')
	s.display.Write("Change to conference # (Enter to cancel): ")
	s.display.ResetColor()
	line := strings.TrimSpace(s.readLine())
	if line == "" {
		return
	}
	var n int
	fmt.Sscanf(line, "%d", &n)
	s.joinConference(confs, n-1)
}

// newMessages lists messages posted since the caller's last logon across every
// conference they can access (MESSAGE.MNU <N> New Messages).
func (s *Session) newMessages() {
	confs, _ := s.storage.GetConferences()
	s.display.Clear()
	s.display.SetColor('A')
	s.display.WriteLine("=== New Messages Since Your Last Call ===")
	s.display.ResetColor()
	count := 0
	for _, c := range confs {
		if !s.canAccessConference(c) {
			continue
		}
		posts, _ := s.storage.GetPosts(c.Name, 200)
		for _, p := range posts {
			if p.PostedAt.After(s.user.LastLogin) {
				s.writePostSummary(c.Name, p)
				count++
			}
		}
	}
	if count == 0 {
		s.display.SetColor('B')
		s.display.WriteLine("No new messages since your last call.")
	}
	s.display.ResetColor()
	s.pause()
}

// yourMessages lists messages the caller authored across accessible conferences
// (MESSAGE.MNU <Y> Your Messages).
func (s *Session) yourMessages() {
	confs, _ := s.storage.GetConferences()
	s.display.Clear()
	s.display.SetColor('A')
	s.display.WriteLine("=== Your Messages ===")
	s.display.ResetColor()
	count := 0
	for _, c := range confs {
		if !s.canAccessConference(c) {
			continue
		}
		posts, _ := s.storage.GetPosts(c.Name, 500)
		for _, p := range posts {
			if strings.EqualFold(p.Author, s.user.Name) || (s.user.Alias != "" && strings.EqualFold(p.Author, s.user.Alias)) {
				s.writePostSummary(c.Name, p)
				count++
			}
		}
	}
	if count == 0 {
		s.display.SetColor('B')
		s.display.WriteLine("You have not posted any messages.")
	}
	s.display.ResetColor()
	s.pause()
}

// searchMessages does a text search of subjects and bodies across accessible
// conferences (MESSAGE.MNU <S> Text Search Messages).
func (s *Session) searchMessages() {
	s.display.SetColor('E')
	s.display.Write("Search messages for: ")
	s.display.ResetColor()
	kw := strings.ToLower(strings.TrimSpace(s.readLine()))
	if kw == "" {
		return
	}
	confs, _ := s.storage.GetConferences()
	s.display.Clear()
	s.display.SetColor('A')
	s.display.WriteLine(fmt.Sprintf("=== Messages matching %q ===", kw))
	s.display.ResetColor()
	count := 0
	for _, c := range confs {
		if !s.canAccessConference(c) {
			continue
		}
		posts, _ := s.storage.GetPosts(c.Name, 500)
		for _, p := range posts {
			if strings.Contains(strings.ToLower(p.Subject), kw) || strings.Contains(strings.ToLower(p.Body), kw) {
				s.writePostSummary(c.Name, p)
				count++
			}
		}
	}
	if count == 0 {
		s.display.SetColor('B')
		s.display.WriteLine("No matching messages.")
	}
	s.display.ResetColor()
	s.pause()
}

// writePostSummary prints one message header line (conference, id, date, author,
// subject) — the shared list row for the scan/search commands.
func (s *Session) writePostSummary(conf string, p domain.Post) {
	s.display.SetColor('B')
	s.display.Write(fmt.Sprintf("%-12s ", trunc(conf, 12)))
	s.display.SetColor('E')
	s.display.Write(fmt.Sprintf("%4d ", p.ID))
	s.display.SetColor('C')
	s.display.Write(p.PostedAt.Format("01/02/06 15:04") + " ")
	s.display.SetColor('F')
	s.display.Write(fmt.Sprintf("%-15s ", trunc(p.Author, 15)))
	if p.ReplyTo > 0 {
		s.display.SetColor('B')
		s.display.Write("[RE] ")
	}
	s.display.SetColor('F')
	s.display.WriteLine(trunc(p.Subject, 30))
}
