package qwk

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jasondostal/tresbbs/domain"
)

// REPProcessor handles incoming REP (reply) packets.
type REPProcessor struct {
	storage interface {
		AddPost(post *domain.Post) error
		GetConferences() ([]domain.Conference, error)
		GetUserByAlias(alias string) (*domain.User, error)
	}
	config *domain.Config
}

// NewREPProcessor creates a new REP packet processor.
func NewREPProcessor(storage interface {
	AddPost(post *domain.Post) error
	GetConferences() ([]domain.Conference, error)
	GetUserByAlias(alias string) (*domain.User, error)
}, config *domain.Config) *REPProcessor {
	return &REPProcessor{
		storage: storage,
		config:  config,
	}
}

// ProcessREP reads a REP packet and imports the reply messages.
// The repPath should point to the .REP file (which is a ZIP archive).
func (r *REPProcessor) ProcessREP(repPath string, user *domain.User) (int, error) {
	// Extract the REP packet
	extractDir, err := extractREPPacket(repPath)
	if err != nil {
		return 0, fmt.Errorf("extracting REP packet: %w", err)
	}
	defer os.RemoveAll(extractDir)

	// Process the MESSAGES.DAT file
	messagesPath := filepath.Join(extractDir, "MESSAGES.DAT")
	if _, err := os.Stat(messagesPath); os.IsNotExist(err) {
		return 0, fmt.Errorf("no MESSAGES.DAT found in REP packet")
	}

	count, err := r.processMessagesFile(messagesPath, user)
	if err != nil {
		return 0, fmt.Errorf("processing messages: %w", err)
	}

	return count, nil
}

// extractREPPacket extracts a REP packet (ZIP archive) to a temporary directory.
func extractREPPacket(repPath string) (string, error) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "tribbs-rep-*")
	if err != nil {
		return "", err
	}

	// Open the ZIP file
	reader, err := zip.OpenReader(repPath)
	if err != nil {
		os.RemoveAll(tempDir)
		return "", fmt.Errorf("opening REP packet: %w", err)
	}
	defer reader.Close()

	// Extract each file
	for _, file := range reader.File {
		// Sanitize path to prevent directory traversal
		extractPath := filepath.Join(tempDir, filepath.Base(file.Name))

		// Open the file in the ZIP
		rc, err := file.Open()
		if err != nil {
			os.RemoveAll(tempDir)
			return "", err
		}

		// Create the output file
		outFile, err := os.Create(extractPath)
		if err != nil {
			rc.Close()
			os.RemoveAll(tempDir)
			return "", err
		}

		// Copy the contents
		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			os.RemoveAll(tempDir)
			return "", err
		}
	}

	return tempDir, nil
}

// processMessagesFile reads MESSAGES.DAT and imports reply messages.
func (r *REPProcessor) processMessagesFile(path string, user *domain.User) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	// Get file size
	fi, err := f.Stat()
	if err != nil {
		return 0, err
	}
	fileSize := fi.Size()

	// Get conferences for mapping
	confs, err := r.storage.GetConferences()
	if err != nil {
		return 0, err
	}

	// Build conference number to name mapping
	confNames := make(map[int]string)
	for i, conf := range confs {
		confNames[i+1] = conf.Name
	}

	count := 0
	offset := int64(0)

	// Read messages until end of file
	for offset < fileSize {
		// Read 128-byte header
		var header QWKHeader
		if err := binary.Read(f, binary.LittleEndian, &header); err != nil {
			if err == io.EOF {
				break
			}
			return count, fmt.Errorf("reading header at offset %d: %w", offset, err)
		}
		offset += HeaderSize

		// Check if this is a valid message header
		if header.Alive != 0xE1 {
			// Skip dead messages
			blocks := parseBlockCount(header.Blocks)
			skipBytes := int64(blocks-1) * BlockSize
			f.Seek(skipBytes, io.SeekCurrent)
			offset += skipBytes
			continue
		}

		// Parse conference number
		confNum := int(binary.LittleEndian.Uint16(header.Conference[:]))

		// Parse message fields
		to := strings.TrimRight(string(header.To[:]), " ")
		from := strings.TrimRight(string(header.From[:]), " ")
		subject := strings.TrimRight(string(header.Subject[:]), " ")
		dateStr := strings.TrimRight(string(header.Date[:]), " ")
		timeStr := strings.TrimRight(string(header.Time[:]), " ")
		refStr := strings.TrimRight(string(header.Refer[:]), " ")

		// Parse reference (reply to)
		var replyTo int64
		if refStr != "" {
			replyTo, _ = strconv.ParseInt(strings.TrimSpace(refStr), 10, 64)
		}

		// Parse date/time
		postedAt := parseQWKDateTime(dateStr, timeStr)

		// Read message body
		blocks := parseBlockCount(header.Blocks)
		bodyBlocks := blocks - 1 // Subtract header block
		if bodyBlocks < 0 {
			bodyBlocks = 0
		}

		body := readBodyBlocks(f, bodyBlocks)
		offset += int64(bodyBlocks) * BlockSize

		// Get conference name
		confName := confNames[confNum]
		if confName == "" {
			confName = "General" // Default conference
		}

		// Create the post
		post := &domain.Post{
			Conference: confName,
			Author:     from,
			Subject:    subject,
			Body:       body,
			ReplyTo:    replyTo,
			PostedAt:   postedAt,
		}

		// Only process if the message is to "All" or to the sysop
		// (private messages to specific users would need special handling)
		if strings.ToLower(to) == "all" || strings.ToLower(to) == "sysop" {
			if err := r.storage.AddPost(post); err != nil {
				// Log error but continue processing
				fmt.Printf("Error adding post from %s: %v\n", from, err)
			} else {
				count++
			}
		}
	}

	return count, nil
}

// parseBlockCount parses the 6-character block count field.
func parseBlockCount(blocks [6]byte) int {
	s := strings.TrimRight(string(blocks[:]), " ")
	count, _ := strconv.Atoi(s)
	if count < 1 {
		count = 1
	}
	return count
}

// readBodyBlocks reads N 128-byte blocks and returns the message body.
func readBodyBlocks(f *os.File, blockCount int) string {
	if blockCount == 0 {
		return ""
	}

	buf := make([]byte, blockCount*BlockSize)
	_, err := io.ReadFull(f, buf)
	if err != nil {
		return ""
	}

	// Find the terminator
	body := string(buf)
	if idx := bytes.IndexByte(buf, MessageTerminator); idx >= 0 {
		body = string(buf[:idx])
	}

	// Convert QWK line endings (carriage return) to standard newlines
	body = strings.ReplaceAll(body, "\r", "\n")

	// Trim trailing whitespace
	body = strings.TrimRight(body, " \n\r\t")

	return body
}

// parseQWKDateTime parses QWK date and time strings.
func parseQWKDateTime(dateStr, timeStr string) time.Time {
	// QWK date format: MM-DD-YY
	// QWK time format: HH:MM

	// Try to parse the date
	if len(dateStr) >= 8 && len(timeStr) >= 5 {
		month, _ := strconv.Atoi(dateStr[0:2])
		day, _ := strconv.Atoi(dateStr[3:5])
		year, _ := strconv.Atoi(dateStr[6:8])

		hour, _ := strconv.Atoi(timeStr[0:2])
		minute, _ := strconv.Atoi(timeStr[3:5])

		// Assume 2000s for years < 50, 1900s for years >= 50
		if year < 50 {
			year += 2000
		} else {
			year += 1900
		}

		// Validate ranges
		if month < 1 || month > 12 {
			month = 1
		}
		if day < 1 || day > 31 {
			day = 1
		}
		if hour < 0 || hour > 23 {
			hour = 0
		}
		if minute < 0 || minute > 59 {
			minute = 0
		}

		return time.Date(year, time.Month(month), day, hour, minute, 0, 0, time.Local)
	}

	// Fallback to current time
	return time.Now()
}

// ProcessREPFromReader processes a REP packet from an io.Reader.
// This is useful for streaming uploads.
func (r *REPProcessor) ProcessREPFromReader(reader io.Reader, user *domain.User) (int, error) {
	// Create a temporary file to store the uploaded data
	tempFile, err := os.CreateTemp("", "tribbs-rep-upload-*.zip")
	if err != nil {
		return 0, fmt.Errorf("creating temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Copy the uploaded data to the temp file
	if _, err := io.Copy(tempFile, reader); err != nil {
		return 0, fmt.Errorf("reading upload: %w", err)
	}

	// Close and reopen to ensure all data is written
	tempFile.Close()

	// Process the REP packet
	return r.ProcessREP(tempFile.Name(), user)
}

// ValidateREP validates that a file is a valid REP packet.
func ValidateREP(path string) error {
	// Check if the file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("REP packet not found: %s", path)
	}

	// Try to open as ZIP
	reader, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("invalid REP packet format: %w", err)
	}
	defer reader.Close()

	// Check for MESSAGES.DAT
	found := false
	for _, file := range reader.File {
		if strings.ToUpper(file.Name) == "MESSAGES.DAT" {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("invalid REP packet: MESSAGES.DAT not found")
	}

	return nil
}
