package session

import (
	"fmt"
	"strings"

	"github.com/jasondostal/tresbbs/internal/chat"
)

// teleconf is the board-wide multi-node teleconference channel, shared by
// every session (there is one teleconference per BBS).
var teleconf = chat.NewChatChannel()

// chatMenu handles inter-node chat.
func (s *Session) chatMenu() {
	s.display.Clear()
	s.display.SetColor('A')
	s.display.WriteLine("╔══════════════════════════════════════════════════════════════╗")
	s.display.SetColor('E')
	s.display.WriteLine("║  Teleconference                                             ║")
	s.display.SetColor('A')
	s.display.WriteLine("╠══════════════════════════════════════════════════════════════╣")
	s.display.SetColor('F')
	s.display.WriteLine("║   <J> Join Teleconference                                   ║")
	s.display.WriteLine("║   <P> Page User for Chat                                    ║")
	s.display.WriteLine("║   <W> Who's Online                                          ║")
	s.display.SetColor('A')
	s.display.WriteLine("╠══════════════════════════════════════════════════════════════╣")
	s.display.SetColor('F')
	s.display.WriteLine("║   <X> Exit                                                  ║")
	s.display.SetColor('A')
	s.display.WriteLine("╚══════════════════════════════════════════════════════════════╝")
	s.display.ResetColor()
	s.menuPrompt("CHAT", "J P W X")

	choice := s.menuKey()
	if len(choice) == 0 {
		return
	}

	switch choice[0] {
	case 'x', 'X':
		return
	case 'j', 'J':
		s.joinTeleconference()
	case 'p', 'P':
		s.pageUserForChat()
	case 'w', 'W':
		s.whoOnline()
	}
}

// joinTeleconference enters the board-wide multi-node teleconference. It is a
// turn-based chat: each time you press Enter, any lines other nodes have said
// since your last turn are shown, then your line (if any) is broadcast.
func (s *Session) joinTeleconference() {
	teleconf.RegisterNode(s.nodeNum)
	defer teleconf.UnregisterNode(s.nodeNum)

	s.fireChatBAT("")

	s.display.Clear()
	s.display.SetColor('A')
	s.display.WriteLine("=== Teleconference ===")
	s.display.SetColor('B')
	s.display.WriteLine("Press Enter to speak. /who lists who's here, /exit leaves.")
	s.display.ResetColor()

	// Show recent history so a joiner has context.
	for _, m := range teleconf.GetHistory(10) {
		s.display.SetColor('F')
		s.display.WriteLine(chat.FormatChatMessage(m))
		s.display.ResetColor()
	}
	// Don't replay that history to ourselves as "new" messages.
	teleconf.GetMessages(s.nodeNum)

	s.broadcastTeleconf(fmt.Sprintf("%s has joined", s.user.Alias), chat.MessageTypeSystem)

	for {
		// Drain and show anything said since our last turn (skip our own chat
		// lines — we already saw them as we typed).
		for _, m := range teleconf.GetMessages(s.nodeNum) {
			if m.FromNode == s.nodeNum && m.Type == chat.MessageTypeChat {
				continue
			}
			s.display.SetColor('F')
			s.display.WriteLine(chat.FormatChatMessage(m))
			s.display.ResetColor()
		}

		s.display.SetColor('E')
		s.display.Write("> ")
		s.display.ResetColor()
		line := s.readLine()

		switch {
		case strings.EqualFold(line, "/exit"):
			s.broadcastTeleconf(fmt.Sprintf("%s has left", s.user.Alias), chat.MessageTypeSystem)
			return
		case strings.EqualFold(line, "/who"):
			s.whoOnline()
		case line != "":
			s.broadcastTeleconf(line, chat.MessageTypeChat)
		}
	}
}

// broadcastTeleconf sends a message to every node in the teleconference.
func (s *Session) broadcastTeleconf(content string, msgType chat.MessageType) {
	teleconf.SendMessage(chat.ChatMessage{
		FromNode:  s.nodeNum,
		FromAlias: s.user.Alias,
		ToNode:    0, // broadcast
		Content:   content,
		Type:      msgType,
	})
}

// pageUserForChat sends a chat page to another user.
func (s *Session) pageUserForChat() {
	s.display.SetColor('E')
	s.display.Write("Page which node #: ")
	s.display.ResetColor()
	nodeStr := s.readLine()

	var targetNode int
	fmt.Sscanf(nodeStr, "%d", &targetNode)

	if targetNode <= 0 {
		return
	}

	// Send the page
	if err := s.nodes.SendPage(s.nodeNum, targetNode, s.user.Alias); err != nil {
		s.display.SetColor('C')
		s.display.WriteLine("That node isn't online or isn't accepting pages.")
		s.display.ResetColor()
		s.pause()
		return
	}
	s.firePageBAT("", targetNode)
	s.storage.LogCaller("Requested chat.")

	s.display.SetColor('A')
	s.display.WriteLine("Page sent! Waiting for response...")
	s.display.ResetColor()
	s.pause()
}

// startChat begins a real-time chat session.
func (s *Session) startChat(targetNode int, targetAlias string) {
	s.display.Clear()
	s.display.SetColor('A')
	s.display.WriteLine(fmt.Sprintf("=== Chat with %s (Node %d) ===", targetAlias, targetNode))
	s.display.WriteLine("Type /exit to end chat")
	s.display.ResetColor()

	s.storage.LogCaller(fmt.Sprintf("Chat started: %s and %s", s.user.Alias, targetAlias))

	for {
		line := s.readLine()
		if strings.ToLower(line) == "/exit" {
			break
		}

		if line != "" {
			// In a real implementation, this would send to the other node
			// via the node port's chat channel
			s.display.SetColor('E')
			s.display.WriteLine(fmt.Sprintf("[%s] %s", s.user.Alias, line))
			s.display.ResetColor()
		}
	}

	s.storage.LogCaller(fmt.Sprintf("Chat ended: %s and %s", s.user.Alias, targetAlias))
}
