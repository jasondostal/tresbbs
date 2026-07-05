// Package echo implements echo conference networking for tresbbs.
//
// Echo conferences allow messages to be synchronized between multiple BBS
// systems. In the original TriBBS, this was done via FidoNet-style message
// networking. In tresbbs, we provide the infrastructure for echo conferences
// with a simple export/import mechanism.
//
// Echo Message Format (BinkP-style):
//   - Message header with source/destination BBS identification
//   - Conference name for routing
//   - Message body with original author attribution
//   - Unique message ID for deduplication
package echo

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jasondostal/tresbbs/domain"
)

// EchoMessage represents a message destined for echo distribution.
type EchoMessage struct {
	ID          string    `json:"id"`
	Conference  string    `json:"conference"`
	FromBBS     string    `json:"from_bbs"`
	FromUser    string    `json:"from_user"`
	ToUser      string    `json:"to_user"`
	Subject     string    `json:"subject"`
	Body        string    `json:"body"`
	PostedAt    time.Time `json:"posted_at"`
	OriginalID  int64     `json:"original_id"` // Original post ID in source BBS
}

// EchoPacket represents a bundle of echo messages for exchange.
type EchoPacket struct {
	SourceBBS  string        `json:"source_bbs"`
	SourceAddr string        `json:"source_addr"` // FidoNet-style address
	CreatedAt  time.Time     `json:"created_at"`
	Messages   []EchoMessage `json:"messages"`
}

// EchoManager handles echo conference operations.
type EchoManager struct {
	storage    interface {
		GetConferences() ([]domain.Conference, error)
		GetPosts(conference string, limit int) ([]domain.Post, error)
		AddPost(post *domain.Post) error
	}
	config     *domain.Config
	exportDir  string
	importDir  string
}

// NewEchoManager creates a new echo conference manager.
func NewEchoManager(storage interface {
	GetConferences() ([]domain.Conference, error)
	GetPosts(conference string, limit int) ([]domain.Post, error)
	AddPost(post *domain.Post) error
}, config *domain.Config, exportDir, importDir string) *EchoManager {
	return &EchoManager{
		storage:   storage,
		config:    config,
		exportDir: exportDir,
		importDir: importDir,
	}
}

// ExportEchoMessages exports messages from echo conferences to a packet file.
func (m *EchoManager) ExportEchoMessages() (string, error) {
	// Get all conferences
	confs, err := m.storage.GetConferences()
	if err != nil {
		return "", fmt.Errorf("getting conferences: %w", err)
	}

	// Create export packet
	packet := EchoPacket{
		SourceBBS:  m.config.BoardName,
		SourceAddr: "0:0/0", // Default FidoNet address
		CreatedAt:  time.Now(),
		Messages:   make([]EchoMessage, 0),
	}

	// Export messages from echo conferences
	for _, conf := range confs {
		if !conf.Echo {
			continue // Skip non-echo conferences
		}

		posts, err := m.storage.GetPosts(conf.Name, 1000)
		if err != nil {
			continue
		}

		for _, post := range posts {
			echoMsg := EchoMessage{
				ID:         fmt.Sprintf("%s-%d-%d", m.config.BoardName, post.ID, time.Now().Unix()),
				Conference: conf.Name,
				FromBBS:    m.config.BoardName,
				FromUser:   post.Author,
				Subject:    post.Subject,
				Body:       post.Body,
				PostedAt:   post.PostedAt,
				OriginalID: post.ID,
			}
			packet.Messages = append(packet.Messages, echoMsg)
		}
	}

	// Write packet to file
	packetData, err := json.MarshalIndent(packet, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling packet: %w", err)
	}

	// Create export directory if it doesn't exist
	if err := os.MkdirAll(m.exportDir, 0755); err != nil {
		return "", fmt.Errorf("creating export dir: %w", err)
	}

	// Write packet file
	filename := fmt.Sprintf("echo-%s-%s.pkt", m.config.BoardName, time.Now().Format("20060102-150405"))
	packetPath := filepath.Join(m.exportDir, filename)
	if err := os.WriteFile(packetPath, packetData, 0644); err != nil {
		return "", fmt.Errorf("writing packet: %w", err)
	}

	return packetPath, nil
}

// ImportEchoMessages imports messages from an echo packet file.
func (m *EchoManager) ImportEchoMessages(packetPath string) (int, error) {
	// Read packet file
	packetData, err := os.ReadFile(packetPath)
	if err != nil {
		return 0, fmt.Errorf("reading packet: %w", err)
	}

	// Parse packet
	var packet EchoPacket
	if err := json.Unmarshal(packetData, &packet); err != nil {
		return 0, fmt.Errorf("parsing packet: %w", err)
	}

	// Don't import our own messages
	if packet.SourceBBS == m.config.BoardName {
		return 0, nil
	}

	imported := 0
	for _, msg := range packet.Messages {
		// Check if conference exists and is an echo conference
		confs, err := m.storage.GetConferences()
		if err != nil {
			continue
		}

		var targetConf *domain.Conference
		for _, conf := range confs {
			if conf.Name == msg.Conference && conf.Echo {
				targetConf = &conf
				break
			}
		}

		if targetConf == nil {
			continue // Skip if conference doesn't exist or isn't echo
		}

		// Create the post
		post := &domain.Post{
			Conference: msg.Conference,
			Author:     fmt.Sprintf("%s @%s", msg.FromUser, msg.FromBBS),
			Subject:    msg.Subject,
			Body:       msg.Body,
			PostedAt:   msg.PostedAt,
		}

		if err := m.storage.AddPost(post); err != nil {
			continue // Skip on error
		}

		imported++
	}

	// Move packet to processed directory
	processedDir := filepath.Join(m.importDir, "processed")
	os.MkdirAll(processedDir, 0755)
	os.Rename(packetPath, filepath.Join(processedDir, filepath.Base(packetPath)))

	return imported, nil
}

// GetEchoConferences returns a list of echo conferences.
func (m *EchoManager) GetEchoConferences() ([]domain.Conference, error) {
	confs, err := m.storage.GetConferences()
	if err != nil {
		return nil, err
	}

	var echoConfs []domain.Conference
	for _, conf := range confs {
		if conf.Echo {
			echoConfs = append(echoConfs, conf)
		}
	}

	return echoConfs, nil
}

// MarkConferenceAsEcho marks a conference for echo distribution.
func (m *EchoManager) MarkConferenceAsEcho(conferenceName string, isEcho bool) error {
	// This would update the conference's Echo field
	// For now, this is a placeholder - the actual implementation
	// would need to update the database
	return nil
}
