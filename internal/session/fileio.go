package session

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jasondostal/tresbbs/domain"
	"github.com/jasondostal/tresbbs/internal/fileutil"
	"github.com/jasondostal/tresbbs/internal/protocol"
)

// resolveFilePath resolves a file entry to its actual path on disk.
func (s *Session) resolveFilePath(file domain.FileEntry) (string, error) {
	// Get the file area to find the base path
	area, err := s.storage.GetFileArea(file.Area)
	if err != nil {
		return "", fmt.Errorf("file area %s not found: %w", file.Area, err)
	}

	// Build full path
	fullPath := filepath.Join(area.Path, file.Name)

	// Verify file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return "", fmt.Errorf("file not found: %s", fullPath)
	}

	return fullPath, nil
}

// readFileData reads a file from disk and returns its contents.
func readFileData(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// displayReadWriter adapts the display port to io.ReadWriter.
type displayReadWriter struct {
	display interface {
		ReadLine() (string, error)
		ReadKey() (byte, error)
		Write(text string)
	}
}

func (d *displayReadWriter) Read(p []byte) (n int, err error) {
	b, err := d.display.ReadKey()
	if err != nil {
		return 0, err
	}
	p[0] = b
	return 1, nil
}

func (d *displayReadWriter) Write(p []byte) (n int, err error) {
	d.display.Write(string(p))
	return len(p), nil
}

// sendDataWithProtocol sends arbitrary bytes (e.g. a QWK packet) over the
// user's selected file-transfer protocol.
func (s *Session) sendDataWithProtocol(name string, data []byte) error {
	rw := &displayReadWriter{display: s.display}
	switch s.user.Protocol {
	case protocol.ProtoXmodem:
		return protocol.NewXmodemSender(rw, false, false).Send(data)
	case protocol.ProtoXmodem1K:
		return protocol.NewXmodemSender(rw, true, true).Send(data)
	case protocol.ProtoZmodem:
		return protocol.NewZmodemSender(rw).SendBatch(name, data, 0)
	default:
		return protocol.NewXmodemSender(rw, true, true).Send(data)
	}
}

// receiveDataWithProtocol receives arbitrary bytes (e.g. a REP packet) over the
// user's selected file-transfer protocol.
func (s *Session) receiveDataWithProtocol() ([]byte, error) {
	rw := &displayReadWriter{display: s.display}
	switch s.user.Protocol {
	case protocol.ProtoXmodem:
		return protocol.NewXmodemReceiver(rw, false).Receive()
	case protocol.ProtoZmodem:
		return protocol.NewZmodemReceiver(rw).Receive()
	default:
		return protocol.NewXmodemReceiver(rw, true).Receive()
	}
}

// firstFile returns the single file's data from a Ymodem batch receive — BBS
// uploads are one file at a time.
func firstFile(files map[string][]byte) []byte {
	for _, d := range files {
		return d
	}
	return nil
}

// downloadFileWithProtocol sends a file using the user's selected protocol.
func (s *Session) downloadFileWithProtocol(file domain.FileEntry) error {
	// Resolve file path
	filePath, err := s.resolveFilePath(file)
	if err != nil {
		return err
	}

	// Read file data
	data, err := readFileData(filePath)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	rw := &displayReadWriter{display: s.display}

	// Send using selected protocol
	switch s.user.Protocol {
	case protocol.ProtoAscii:
		// Ascii capture: stream the raw file bytes with no protocol framing,
		// as TriBBS's <A> Ascii option did for plain-text viewing/capture.
		_, werr := rw.Write(data)
		return werr

	case protocol.ProtoXmodem:
		sender := protocol.NewXmodemSender(rw, false, false)
		return sender.Send(data)

	case protocol.ProtoXmodem1K:
		sender := protocol.NewXmodemSender(rw, true, true)
		return sender.Send(data)

	case protocol.ProtoYmodem:
		return protocol.NewYmodemSender(rw).SendBatch(map[string][]byte{file.Name: data})

	case protocol.ProtoYmodemG:
		return protocol.NewYmodemGSender(rw).SendBatch(map[string][]byte{file.Name: data})

	case protocol.ProtoZmodem:
		sender := protocol.NewZmodemSender(rw)
		return sender.SendBatch(file.Name, data, 0)

	default:
		// Default to Xmodem-1K
		sender := protocol.NewXmodemSender(rw, true, true)
		return sender.Send(data)
	}
}

// uploadFileWithProtocol receives a file using the user's selected protocol.
func (s *Session) uploadFileWithProtocol(area string, filename string) error {
	// Get file area
	fileArea, err := s.storage.GetFileArea(area)
	if err != nil {
		return fmt.Errorf("file area %s not found: %w", area, err)
	}
	if fileutil.IsCDROMArea(*fileArea) {
		return fmt.Errorf("%s is a read-only CD-ROM area — uploads not allowed", area)
	}

	rw := &displayReadWriter{display: s.display}

	// Receive using selected protocol
	var data []byte
	switch s.user.Protocol {
	case protocol.ProtoXmodem:
		receiver := protocol.NewXmodemReceiver(rw, false)
		data, err = receiver.Receive()
	case protocol.ProtoXmodem1K:
		receiver := protocol.NewXmodemReceiver(rw, true)
		data, err = receiver.Receive()
	case protocol.ProtoZmodem:
		receiver := protocol.NewZmodemReceiver(rw)
		data, err = receiver.Receive()
	case protocol.ProtoYmodem:
		var files map[string][]byte
		files, err = protocol.NewYmodemReceiver(rw).Receive()
		data = firstFile(files)
	case protocol.ProtoYmodemG:
		var files map[string][]byte
		files, err = protocol.NewYmodemGReceiver(rw).Receive()
		data = firstFile(files)
	default:
		receiver := protocol.NewXmodemReceiver(rw, true)
		data, err = receiver.Receive()
	}
	if err != nil {
		return fmt.Errorf("receiving file: %w", err)
	}

	// Write file to disk
	filePath := filepath.Join(fileArea.Path, filename)
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	// Pull FILE_ID.DIZ out of the archive for the file description
	description := ""
	if diz, derr := fileutil.ExtractFileIDDIZ(filePath); derr == nil {
		description = diz
	}

	// Add to database
	entry := &domain.FileEntry{
		Area:        area,
		Name:        filename,
		Size:        int64(len(data)),
		Description: description,
		UploadedBy:  s.user.Alias,
	}
	if err := s.storage.AddFile(entry); err != nil {
		return fmt.Errorf("recording file: %w", err)
	}

	// Update user stats
	s.user.FilesUploaded++
	s.user.KUploaded += int64(len(data)) / 1024
	s.storage.SaveUser(s.user)

	s.storage.LogCaller(fmt.Sprintf("Uploaded file: %s.", filename))
	return nil
}
