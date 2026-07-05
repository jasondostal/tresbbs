package session

import (
	"github.com/jasondostal/tresbbs/internal/bathandler"
)

// batBaseDir is where the BBS looks for the v11.6+ BAT hook files
// (CHAT.BAT, GOODBYE.BAT, EDITOR.BAT, PAGE.BAT) — the working directory.
const batBaseDir = "."

// batContext builds the environment context passed to BAT file hooks.
func (s *Session) batContext() *bathandler.BATContext {
	return &bathandler.BATContext{
		UserName:   s.user.Name,
		UserAlias:  s.user.Alias,
		City:       s.user.City,
		Phone:      s.user.Phone,
		Security:   s.user.SecurityLevel,
		BaudRate:   s.baudRate,
		Node:       s.nodeNum,
		TimeLeft:   s.user.TimeLeftToday,
		TotalCalls: s.user.TotalCalls,
		LoginTime:  s.loginAt,
	}
}

// fireGoodbyeBAT runs GOODBYE.BAT on logoff (no-op if the file is absent).
func (s *Session) fireGoodbyeBAT() {
	bathandler.GoodbyeBAT(batBaseDir, s.batContext())
}

// firePageBAT runs PAGE.BAT when a user pages another for chat.
func (s *Session) firePageBAT(targetAlias string, targetNode int) {
	ctx := s.batContext()
	ctx.TargetUser = targetAlias
	ctx.TargetNode = targetNode
	bathandler.PageBAT(batBaseDir, ctx)
}

// fireChatBAT runs CHAT.BAT when a chat/teleconference is entered.
func (s *Session) fireChatBAT(message string) {
	ctx := s.batContext()
	ctx.Message = message
	bathandler.ChatBAT(batBaseDir, ctx)
}
