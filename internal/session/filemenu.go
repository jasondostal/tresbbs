package session

import (
	"fmt"
	"strings"

	"github.com/jasondostal/tresbbs/domain"
)

// File-menu command handlers backing the real FILES.MNU command set. The
// listing/tag/move/delete operations live in session.go and filetag.go; this
// file adds the direct by-name download/view and the batch-queue editor.

// findFileByName searches every file area the caller can access for a file whose
// name matches (case-insensitive), returning the first match.
func (s *Session) findFileByName(name string) (domain.FileEntry, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.FileEntry{}, false
	}
	areas, _ := s.storage.GetFileAreas()
	for _, a := range areas {
		if a.SecurityLevel > s.user.SecurityLevel {
			continue
		}
		files, _ := s.storage.GetFiles(a.Name, 1000)
		for _, f := range files {
			if strings.EqualFold(f.Name, name) {
				return f, true
			}
		}
	}
	return domain.FileEntry{}, false
}

// downloadPrompt downloads a file the caller names (FILES.MNU <D> Download File).
func (s *Session) downloadPrompt() {
	s.display.SetColor('E')
	s.display.Write("Download which file? ")
	s.display.ResetColor()
	name := s.readLine()
	f, ok := s.findFileByName(name)
	if !ok {
		s.notAvailable("File not found.")
		return
	}
	s.downloadFile(f)
}

// viewArchivePrompt lists an archive's contents by name (FILES.MNU <V> View).
func (s *Session) viewArchivePrompt() {
	s.display.SetColor('E')
	s.display.Write("View which archive? ")
	s.display.ResetColor()
	name := s.readLine()
	f, ok := s.findFileByName(name)
	if !ok {
		s.notAvailable("File not found.")
		return
	}
	s.viewArchive(f)
}

// editBatchQueue manages the flagged-files batch queue (FILES.MNU <E> Edit Batch
// Queue): review tagged files, download them all, remove one, or clear the list.
func (s *Session) editBatchQueue() {
	for {
		s.display.Clear()
		s.display.SetColor('A')
		s.display.WriteLine("=== Batch Download Queue ===")
		s.display.ResetColor()
		if len(s.flagged) == 0 {
			s.display.SetColor('B')
			s.display.WriteLine("No files are flagged for batch download.")
			s.display.ResetColor()
			s.pause()
			return
		}
		var total int64
		for i, f := range s.flagged {
			s.display.SetColor('E')
			s.display.Write(fmt.Sprintf("  [%d] ", i+1))
			s.display.SetColor('F')
			s.display.Write(fmt.Sprintf("%-30s ", trunc(f.Name, 30)))
			s.display.SetColor('B')
			s.display.WriteLine(fmt.Sprintf("%8d  %s", f.Size, f.Area))
			total += f.Size
		}
		s.display.SetColor('A')
		s.display.WriteLine(fmt.Sprintf("%d file(s), %d bytes total.", len(s.flagged), total))
		s.display.SetColor('E')
		s.display.Write("<D>ownload all  <R>emove #  <C>lear  0=exit: ")
		s.display.ResetColor()
		choice := strings.TrimSpace(s.readLine())
		if choice == "" || choice == "0" {
			return
		}
		switch strings.ToUpper(choice)[0] {
		case 'D':
			s.batchDownloadFlagged()
			return
		case 'C':
			s.flagged = nil
			s.display.SetColor('B')
			s.display.WriteLine("Batch queue cleared.")
			s.display.ResetColor()
			s.pause()
			return
		case 'R':
			var n int
			fmt.Sscanf(choice[1:], "%d", &n)
			if n >= 1 && n <= len(s.flagged) {
				removed := s.flagged[n-1].Name
				s.flagged = append(s.flagged[:n-1], s.flagged[n:]...)
				s.display.SetColor('B')
				s.display.WriteLine(fmt.Sprintf("Removed %s from the queue.", removed))
				s.display.ResetColor()
			}
		}
	}
}
