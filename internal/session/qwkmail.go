package session

import (
	"bytes"
	"fmt"
	"os"

	"github.com/jasondostal/tresbbs/internal/fileutil"
	"github.com/jasondostal/tresbbs/internal/qwk"
)

// qwkMailMenu offers QWK offline-mail: download a QWK packet of unread
// messages, or upload a .REP packet of replies composed offline.
func (s *Session) qwkMailMenu() {
	for {
		s.display.Clear()
		s.display.SetColor('A')
		s.display.WriteLine("=== QWK Offline Mail ===")
		s.display.SetColor('F')
		s.display.WriteLine("  [D] Download QWK packet")
		s.display.WriteLine("  [U] Upload REP packet")
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
		case 'd', 'D':
			s.downloadQWK()
		case 'u', 'U':
			s.uploadREP()
		}
	}
}

// downloadQWK generates a QWK packet and sends it over the user's protocol.
func (s *Session) downloadQWK() {
	tmpDir, err := os.MkdirTemp("", "qwk-*")
	if err != nil {
		s.qwkError("cannot create work directory", err)
		return
	}
	defer os.RemoveAll(tmpDir)

	gen := qwk.NewGenerator(s.storage, s.config)
	packetPath, err := gen.Generate(s.user, tmpDir)
	if err != nil {
		s.qwkError("generating QWK packet", err)
		return
	}

	data, err := os.ReadFile(packetPath)
	if err != nil {
		s.qwkError("reading QWK packet", err)
		return
	}

	s.display.SetColor('B')
	s.display.WriteLine(fmt.Sprintf("Sending QWK packet (%s)...", fileutil.FormatFileSize(int64(len(data)))))
	s.display.ResetColor()

	if err := s.sendDataWithProtocol("MAIL.QWK", data); err != nil {
		s.qwkError("sending packet", err)
		return
	}
	s.display.SetColor('A')
	s.display.WriteLine("QWK packet sent!")
	s.storage.LogCaller("Downloaded QWK mail packet")
	s.pause()
}

// uploadREP receives a REP packet and imports the replies.
func (s *Session) uploadREP() {
	s.display.SetColor('B')
	s.display.WriteLine("Begin your REP upload now...")
	s.display.ResetColor()

	data, err := s.receiveDataWithProtocol()
	if err != nil {
		s.qwkError("receiving REP packet", err)
		return
	}

	proc := qwk.NewREPProcessor(s.storage, s.config)
	count, err := proc.ProcessREPFromReader(bytes.NewReader(data), s.user)
	if err != nil {
		s.qwkError("importing replies", err)
		return
	}

	s.display.SetColor('A')
	s.display.WriteLine(fmt.Sprintf("Imported %d repl%s from REP packet.", count, plural(count, "y", "ies")))
	s.storage.LogCaller(fmt.Sprintf("Uploaded REP packet: %d messages", count))
	s.pause()
}

func (s *Session) qwkError(what string, err error) {
	s.display.SetColor('C')
	s.display.WriteLine(fmt.Sprintf("QWK error (%s): %v", what, err))
	s.display.ResetColor()
	s.pause()
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
