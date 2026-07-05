package session

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jasondostal/tresbbs/domain"
)

// isFlagged reports whether a file is in the session's batch-download flag list.
func (s *Session) isFlagged(f domain.FileEntry) bool {
	for _, ff := range s.flagged {
		if ff.Name == f.Name && ff.Area == f.Area {
			return true
		}
	}
	return false
}

// toggleFlag tags or untags a file for batch download (TriBBS's flagged-files
// list, held per session).
func (s *Session) toggleFlag(f domain.FileEntry) {
	for i, ff := range s.flagged {
		if ff.Name == f.Name && ff.Area == f.Area {
			s.flagged = append(s.flagged[:i], s.flagged[i+1:]...)
			s.display.SetColor('B')
			s.display.WriteLine(fmt.Sprintf("Untagged %s.", f.Name))
			s.display.ResetColor()
			return
		}
	}
	s.flagged = append(s.flagged, f)
	s.display.SetColor('A')
	s.display.WriteLine(fmt.Sprintf("Tagged %s. (%d file(s) flagged)", f.Name, len(s.flagged)))
	s.display.ResetColor()
}

// batchDownloadFlagged downloads every flagged file in turn, then clears the
// flag list (TriBBS's batch/EBATCH download).
func (s *Session) batchDownloadFlagged() {
	if len(s.flagged) == 0 {
		s.display.SetColor('C')
		s.display.WriteLine("No files are flagged for batch download.")
		s.display.ResetColor()
		s.pause()
		return
	}
	s.display.SetColor('A')
	s.display.WriteLine(fmt.Sprintf("Batch download: %d flagged file(s).", len(s.flagged)))
	s.display.ResetColor()
	for _, f := range s.flagged {
		s.downloadFile(f)
	}
	s.flagged = nil
}

// archiveListCmd returns the command that lists an archive's contents, or nil
// if the extension isn't a recognized archive.
func archiveListCmd(path string) *exec.Cmd {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".zip":
		return exec.Command("unzip", "-l", path)
	case ".arj":
		return exec.Command("arj", "l", path)
	case ".lzh", ".lha":
		return exec.Command("lha", "l", path)
	case ".tar":
		return exec.Command("tar", "-tvf", path)
	case ".gz", ".tgz":
		return exec.Command("tar", "-tzvf", path)
	default:
		return nil
	}
}

// viewArchive lists the contents of an archive file before download (TriBBS's
// VIEW command). Uses the host's archive tool for the file type if available.
func (s *Session) viewArchive(f domain.FileEntry) {
	path, err := s.resolveFilePath(f)
	if err != nil {
		s.display.SetColor('C')
		s.display.WriteLine("File not found.")
		s.display.ResetColor()
		s.pause()
		return
	}
	cmd := archiveListCmd(path)
	s.display.Clear()
	s.display.SetColor('A')
	s.display.WriteLine(fmt.Sprintf("=== Contents of %s ===", f.Name))
	s.display.ResetColor()
	if cmd == nil {
		s.display.SetColor('C')
		s.display.WriteLine(fmt.Sprintf("%s is not a recognized archive (zip/arj/lzh/tar/gz).", f.Name))
		s.display.ResetColor()
		s.pause()
		return
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		s.display.SetColor('C')
		s.display.WriteLine(fmt.Sprintf("Could not read archive: %v", err))
		s.display.WriteLine("(The archive tool may not be installed on this host.)")
		s.display.ResetColor()
	} else {
		s.display.SetColor('F')
		for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
			s.display.WriteLine(line)
		}
		s.display.ResetColor()
	}
	s.storage.LogCaller(fmt.Sprintf("Downloaded file : VIEW%s.", f.Name))
	s.pause()
}
