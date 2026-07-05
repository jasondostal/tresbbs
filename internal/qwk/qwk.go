// Package qwk implements QWK networking for offline mail packets.
//
// QWK was the standard offline mail packet format for BBS systems in the
// 1990s. Users would download a QWK packet (containing new messages from
// all conferences), read and reply offline using a local reader (like
// QMailDoor), then upload a REP (reply) packet on next login.
//
// QWK Packet Format:
//   - Control file (CONTROL.DAT): BBS info, conference list
//   - Message file (MESSAGES.DAT): packed messages in 128-byte records
//   - Individual message files (*.MSG): for offline readers
//   - Index files (*.NDX): message pointers per conference
//   - Welcome bulletin and new file list
//
// REP Packet Format:
//   - Messages to upload (same format as QWK messages)
package qwk

import (
	"archive/zip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jasondostal/tresbbs/domain"
)

// QWK packet constants
const (
	HeaderSize        = 128
	BlockSize         = 128
	MessageTerminator = 0xE3
	QWKMagicNumber    = 0xE3 // Standard QWK message terminator
)

// QWKHeader represents a 128-byte message header.
type QWKHeader struct {
	Status     byte     // Message status: ' ' = public, '-' = private read, '+' = private
	Number     [7]byte  // Message number (7 digits, right-justified)
	Date       [8]byte  // Date (MM-DD-YY)
	Time       [5]byte  // Time (HH:MM)
	To         [25]byte // To field (25 chars)
	From       [25]byte // From field (25 chars)
	Subject    [25]byte // Subject (25 chars)
	Password   [12]byte // Password (unused)
	Refer      [8]byte  // Reference message number
	Blocks     [6]byte  // Number of 128-byte blocks (header + body)
	Alive      byte     // Message alive flag: 0xE1 = alive, 0xE2 = deleted
	Conference [2]byte  // Conference number (low byte, high byte)
	Private    byte     // Private flag: 0 = public, 1 = private
	HasNet     byte     // Has net node address
	Zone       [2]byte  // Net zone
	Net        [2]byte  // Net number
	Node       [2]byte  // Net node
	Cost       [2]byte  // Cost
	Unknown    [4]byte  // Unknown/padding
}

// Generator creates QWK packets for download.
type Generator struct {
	storage interface {
		GetConferences() ([]domain.Conference, error)
		GetPosts(conference string, limit int) ([]domain.Post, error)
		GetBulletins() ([]domain.Bulletin, error)
		GetFileAreas() ([]domain.FileArea, error)
		GetFiles(area string, limit int) ([]domain.FileEntry, error)
	}
	config *domain.Config
}

// NewGenerator creates a new QWK packet generator.
func NewGenerator(storage interface {
	GetConferences() ([]domain.Conference, error)
	GetPosts(conference string, limit int) ([]domain.Post, error)
	GetBulletins() ([]domain.Bulletin, error)
	GetFileAreas() ([]domain.FileArea, error)
	GetFiles(area string, limit int) ([]domain.FileEntry, error)
}, config *domain.Config) *Generator {
	return &Generator{
		storage: storage,
		config:  config,
	}
}

// Generate creates a QWK packet for the given user.
// Returns the path to the generated QWK archive.
func (g *Generator) Generate(user *domain.User, outputDir string) (string, error) {
	// Create packet directory
	packetDir := filepath.Join(outputDir, "QWK")
	if err := os.MkdirAll(packetDir, 0755); err != nil {
		return "", fmt.Errorf("creating packet dir: %w", err)
	}
	defer os.RemoveAll(packetDir) // Clean up after archiving

	// Get conferences and build conference index
	confs, err := g.storage.GetConferences()
	if err != nil {
		return "", fmt.Errorf("getting conferences: %w", err)
	}

	// Build conference name to number mapping
	confMap := make(map[string]int)
	for i, conf := range confs {
		confMap[conf.Name] = i + 1 // Conference numbers start at 1
	}

	// Generate CONTROL.DAT (control file)
	if err := g.generateControlFile(packetDir, user, confs); err != nil {
		return "", fmt.Errorf("generating control file: %w", err)
	}

	// Generate MESSAGES.DAT and collect per-conference messages
	msgCounts, err := g.generateMessages(packetDir, user, confMap)
	if err != nil {
		return "", fmt.Errorf("generating messages: %w", err)
	}

	// Generate NDX files (conference indexes)
	if err := g.generateIndexes(packetDir, user, confs, msgCounts); err != nil {
		return "", fmt.Errorf("generating indexes: %w", err)
	}

	// Generate individual .MSG files for offline readers
	if err := g.generateMsgFiles(packetDir, user, confMap); err != nil {
		return "", fmt.Errorf("generating MSG files: %w", err)
	}

	// Generate welcome display
	if err := g.generateWelcome(packetDir); err != nil {
		return "", fmt.Errorf("generating welcome: %w", err)
	}

	// Generate bulletins
	if err := g.generateBulletins(packetDir); err != nil {
		return "", fmt.Errorf("generating bulletins: %w", err)
	}

	// Generate new file list
	if err := g.generateNewFileList(packetDir, user); err != nil {
		return "", fmt.Errorf("generating new file list: %w", err)
	}

	// Create the QWK archive (ZIP)
	bbsID := sanitizeFilename(g.config.BoardName)
	archivePath := filepath.Join(outputDir, bbsID+".QWK")
	if err := createZipArchive(packetDir, archivePath); err != nil {
		return "", fmt.Errorf("creating archive: %w", err)
	}

	return archivePath, nil
}

// generateControlFile creates the CONTROL.DAT control file.
func (g *Generator) generateControlFile(dir string, user *domain.User, confs []domain.Conference) error {
	path := filepath.Join(dir, "CONTROL.DAT")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Control file format (one field per line, CRLF line endings)
	fmt.Fprintf(f, "%s\r\n", g.config.BoardName)
	fmt.Fprintf(f, "%s\r\n", "")                     // City/state
	fmt.Fprintf(f, "%s\r\n", "555-1234")             // Phone
	fmt.Fprintf(f, "%s\r\n", g.config.SysopName)     // Sysop name
	fmt.Fprintf(f, "%s\r\n", "1")                    // BBS ID (serial number)
	fmt.Fprintf(f, "%s\r\n", time.Now().Format("01-02-06,15:04:05"))
	fmt.Fprintf(f, "%s\r\n", user.Alias)              // User name for packet
	fmt.Fprintf(f, "%s\r\n", "***")                   // Packet password (unused)

	// Conference list
	// Conference 0 is always the "General" or "Mail" conference
	fmt.Fprintf(f, "%d\r\n", 0)
	fmt.Fprintf(f, "%s\r\n", "General")

	for _, conf := range confs {
		confNum := 0
		for i, c := range confs {
			if c.Name == conf.Name {
				confNum = i + 1
				break
			}
		}
		fmt.Fprintf(f, "%d\r\n", confNum)
		fmt.Fprintf(f, "%s\r\n", conf.Name)
	}

	// End marker
	fmt.Fprintf(f, "0\r\n")

	return nil
}

// generateMessages creates the MESSAGES.DAT file.
// Returns a map of conference number to message count.
func (g *Generator) generateMessages(dir string, user *domain.User, confMap map[string]int) (map[int]int, error) {
	path := filepath.Join(dir, "MESSAGES.DAT")
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	msgCounts := make(map[int]int)
	msgNum := 1

	// Get all conferences and their messages
	confs, err := g.storage.GetConferences()
	if err != nil {
		return nil, err
	}

	for _, conf := range confs {
		// Check security
		if conf.SecurityLevel > user.SecurityLevel {
			continue
		}
		if conf.PrivateConf && user.SecurityLevel < 100 { // TODO: Use sysop security
			continue
		}

		confNum := confMap[conf.Name]
		posts, err := g.storage.GetPosts(conf.Name, 1000)
		if err != nil {
			continue
		}

		for _, post := range posts {
			// Write 128-byte header
			header := buildHeader(post, confNum, msgNum)
			if err := binary.Write(f, binary.LittleEndian, header); err != nil {
				return nil, err
			}

			// Write message body in 128-byte blocks
			if err := writeBlocks(f, post.Body); err != nil {
				return nil, err
			}

			msgCounts[confNum]++
			msgNum++
		}
	}

	return msgCounts, nil
}

// buildHeader creates a 128-byte QWK message header.
func buildHeader(post domain.Post, confNum, msgNum int) QWKHeader {
	var header QWKHeader

	// Status: space for public, '*' for private, '-' for private read
	header.Status = ' '

	// Message number (7 digits, right-justified)
	numStr := fmt.Sprintf("%7d", msgNum)
	copy(header.Number[:], numStr)

	// Date and time (MM-DD-YY format)
	copy(header.Date[:], post.PostedAt.Format("01-02-06"))
	copy(header.Time[:], post.PostedAt.Format("15:04"))

	// To field
	copy(header.To[:], padOrTrim("All", 25))

	// From field
	copy(header.From[:], padOrTrim(post.Author, 25))

	// Subject field
	copy(header.Subject[:], padOrTrim(post.Subject, 25))

	// Reference message number (for replies)
	if post.ReplyTo > 0 {
		refStr := fmt.Sprintf("%7d", post.ReplyTo)
		copy(header.Refer[:], refStr)
	}

	// Calculate number of 128-byte blocks (header + body)
	bodyLen := len(post.Body)
	blockCount := 1 + ((bodyLen + BlockSize - 1) / BlockSize) // 1 for header + ceil(bodyLen/128)
	blocksStr := fmt.Sprintf("%6d", blockCount)
	copy(header.Blocks[:], blocksStr)

	// Conference number (stored as 2 bytes, little-endian)
	binary.LittleEndian.PutUint16(header.Conference[:], uint16(confNum))

	// Message alive
	header.Alive = 0xE1

	return header
}

// writeBlocks writes message body in 128-byte blocks with proper padding.
func writeBlocks(f *os.File, body string) error {
	// Convert newlines to QWK format (single carriage return)
	body = strings.ReplaceAll(body, "\r\n", "\r")
	body = strings.ReplaceAll(body, "\n", "\r")

	data := []byte(body)
	if len(data) == 0 {
		// Empty body - write one block with terminator
		block := make([]byte, BlockSize)
		block[0] = MessageTerminator
		return binary.Write(f, binary.LittleEndian, block)
	}

	// Write body in 128-byte blocks
	for i := 0; i < len(data); i += BlockSize {
		end := i + BlockSize
		if end > len(data) {
			end = len(data)
		}

		block := make([]byte, BlockSize)
		copy(block, data[i:end])

		// Pad with spaces (0x20)
		for j := end - i; j < BlockSize; j++ {
			block[j] = ' '
		}

		if err := binary.Write(f, binary.LittleEndian, block); err != nil {
			return err
		}
	}

	return nil
}

// generateIndexes creates .NDX index files for each conference.
func (g *Generator) generateIndexes(dir string, user *domain.User, confs []domain.Conference, msgCounts map[int]int) error {
	// Track current message position in MESSAGES.DAT
	// Each message is: 128-byte header + N*128-byte body blocks
	// We need to track the starting record number for each conference

	// For simplicity, we'll create empty NDX files as placeholders
	// Real QWK readers can scan MESSAGES.DAT directly
	for i := range confs {
		confNum := i + 1
		ndxPath := filepath.Join(dir, fmt.Sprintf("%d.NDX", confNum))
		f, err := os.Create(ndxPath)
		if err != nil {
			return err
		}

		// Write placeholder NDX entries (5 bytes each: 4 byte offset + 1 byte conference)
		// For now, just create empty files
		f.Close()
	}

	return nil
}

// generateMsgFiles creates individual .MSG files for offline readers.
func (g *Generator) generateMsgFiles(dir string, user *domain.User, confMap map[string]int) error {
	msgDir := filepath.Join(dir, "MSG")
	if err := os.MkdirAll(msgDir, 0755); err != nil {
		return err
	}

	confs, err := g.storage.GetConferences()
	if err != nil {
		return err
	}

	msgNum := 1
	for _, conf := range confs {
		// Check security
		if conf.SecurityLevel > user.SecurityLevel {
			continue
		}

		confNum := confMap[conf.Name]
		posts, err := g.storage.GetPosts(conf.Name, 1000)
		if err != nil {
			continue
		}

		for _, post := range posts {
			// Create individual .MSG file
			msgPath := filepath.Join(msgDir, fmt.Sprintf("%d.MSG", msgNum))
			f, err := os.Create(msgPath)
			if err != nil {
				return err
			}

			// Write header
			header := buildHeader(post, confNum, msgNum)
			binary.Write(f, binary.LittleEndian, header)

			// Write body
			writeBlocks(f, post.Body)

			f.Close()
			msgNum++
		}
	}

	return nil
}

// generateWelcome creates the WELCOME.TXT file for the QWK packet.
func (g *Generator) generateWelcome(dir string) error {
	path := filepath.Join(dir, "WELCOME.TXT")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "Welcome to %s\r\n", g.config.BoardName)
	fmt.Fprintf(f, "========================================\r\n")
	fmt.Fprintf(f, "\r\n")
	fmt.Fprintf(f, "This QWK packet was generated on %s\r\n", time.Now().Format("01/02/2006 03:04 PM"))
	fmt.Fprintf(f, "\r\n")
	fmt.Fprintf(f, "To read your messages offline, use a QWK-compatible\r\n")
	fmt.Fprintf(f, "offline reader such as QMailDoor, OLXE, or RAE.\r\n")
	fmt.Fprintf(f, "\r\n")
	fmt.Fprintf(f, "When you're ready to upload your replies, create a\r\n")
	fmt.Fprintf(f, "REP packet and upload it to the BBS.\r\n")
	fmt.Fprintf(f, "\r\n")
	fmt.Fprintf(f, "Sysop: %s\r\n", g.config.SysopName)
	fmt.Fprintf(f, "\r\n")
	fmt.Fprintf(f, "Enjoy!\r\n")

	return nil
}

// generateBulletins creates bulletin files in the QWK packet.
func (g *Generator) generateBulletins(dir string) error {
	bulletins, err := g.storage.GetBulletins()
	if err != nil {
		return nil // Bulletins are optional
	}

	if len(bulletins) == 0 {
		return nil
	}

	path := filepath.Join(dir, "BULLETIN.TXT")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "Bulletins from %s\r\n", g.config.BoardName)
	fmt.Fprintf(f, "========================================\r\n")
	fmt.Fprintf(f, "\r\n")

	for _, b := range bulletins {
		fmt.Fprintf(f, "--- %s ---\r\n", b.Name)
		fmt.Fprintf(f, "%s\r\n", b.Content)
		fmt.Fprintf(f, "\r\n")
	}

	return nil
}

// generateNewFileList creates a new file list for the QWK packet.
func (g *Generator) generateNewFileList(dir string, user *domain.User) error {
	path := filepath.Join(dir, "NEWFILES.TXT")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "New Files on %s\r\n", g.config.BoardName)
	fmt.Fprintf(f, "========================================\r\n")
	fmt.Fprintf(f, "\r\n")

	areas, err := g.storage.GetFileAreas()
	if err != nil {
		return nil
	}

	for _, area := range areas {
		if area.SecurityLevel > user.SecurityLevel {
			continue
		}

		files, err := g.storage.GetFiles(area.Name, 50)
		if err != nil {
			continue
		}

		if len(files) > 0 {
			fmt.Fprintf(f, "Area: %s\r\n", area.Name)
			fmt.Fprintf(f, "----------------------------------------\r\n")
			for _, file := range files {
				fmt.Fprintf(f, "  %-30s %8d bytes\r\n", file.Name, file.Size)
				fmt.Fprintf(f, "    %s\r\n", file.Description)
			}
			fmt.Fprintf(f, "\r\n")
		}
	}

	return nil
}

// createZipArchive creates a ZIP archive from a directory.
func createZipArchive(sourceDir, destPath string) error {
	// Create the ZIP file
	zipFile, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	writer := zip.NewWriter(zipFile)
	defer writer.Close()

	// Walk the source directory
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		// Create file in ZIP
		zipEntry, err := writer.Create(relPath)
		if err != nil {
			return err
		}

		// Open source file
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		// Copy file contents
		_, err = io.Copy(zipEntry, srcFile)
		return err
	})
}

// Helper functions

func padOrTrim(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

func sanitizeFilename(name string) string {
	// Replace spaces and special chars with underscores
	result := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, name)

	// Limit length
	if len(result) > 8 {
		result = result[:8]
	}

	return strings.ToUpper(result)
}
