// Package chat provides real-time inter-node chat functionality for tresbbs.
//
// This implements the chat system that allows users on different nodes
// to communicate in real-time, including private messages and group chat.
package chat

import (
	"fmt"
	"sync"
	"time"
)

// ChatMessage represents a real-time chat message between nodes.
type ChatMessage struct {
	FromNode  int       // Sender node number
	FromAlias string    // Sender alias
	ToNode    int       // Target node (0 = broadcast)
	ToAlias   string    // Target alias (for private messages)
	Content   string    // Message content
	Timestamp time.Time // When the message was sent
	Type      MessageType
}

// MessageType represents the type of chat message.
type MessageType int

const (
	MessageTypeChat     MessageType = iota // Regular chat message
	MessageTypePage                        // Page request
	MessageTypePageResponse                // Response to page
	MessageTypePrivate                     // Private message
	MessageTypeSystem                      // System message
	MessageTypeAnnouncement                // Sysop announcement
)

// ChatChannel provides real-time messaging between nodes.
type ChatChannel struct {
	mu       sync.RWMutex
	messages map[int][]ChatMessage // node -> pending messages
	history  []ChatMessage         // Chat history (limited)
	maxHist  int                   // Max history entries
}

// NewChatChannel creates a new chat channel.
func NewChatChannel() *ChatChannel {
	return &ChatChannel{
		messages: make(map[int][]ChatMessage),
		maxHist:  100,
	}
}

// SendMessage sends a message to a specific node or broadcasts to all.
func (c *ChatChannel) SendMessage(msg ChatMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()

	msg.Timestamp = time.Now()

	if msg.ToNode == 0 {
		// Broadcast to all nodes
		for nodeNum := range c.messages {
			c.messages[nodeNum] = append(c.messages[nodeNum], msg)
		}
	} else {
		// Send to specific node
		c.messages[msg.ToNode] = append(c.messages[msg.ToNode], msg)
	}

	// Add to history
	c.history = append(c.history, msg)
	if len(c.history) > c.maxHist {
		c.history = c.history[len(c.history)-c.maxHist:]
	}
}

// GetMessages retrieves and clears pending messages for a node.
func (c *ChatChannel) GetMessages(nodeNum int) []ChatMessage {
	c.mu.Lock()
	defer c.mu.Unlock()

	msgs := c.messages[nodeNum]
	c.messages[nodeNum] = nil
	return msgs
}

// HasMessages checks if a node has pending messages.
func (c *ChatChannel) HasMessages(nodeNum int) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.messages[nodeNum]) > 0
}

// GetHistory returns recent chat history.
func (c *ChatChannel) GetHistory(limit int) []ChatMessage {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if limit <= 0 || limit > len(c.history) {
		limit = len(c.history)
	}

	start := len(c.history) - limit
	if start < 0 {
		start = 0
	}

	return c.history[start:]
}

// RegisterNode registers a node to receive messages.
func (c *ChatChannel) RegisterNode(nodeNum int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.messages[nodeNum] == nil {
		c.messages[nodeNum] = make([]ChatMessage, 0)
	}
}

// UnregisterNode removes a node from the channel.
func (c *ChatChannel) UnregisterNode(nodeNum int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.messages, nodeNum)
}

// FormatChatMessage formats a chat message for display.
func FormatChatMessage(msg ChatMessage) string {
	switch msg.Type {
	case MessageTypePage:
		return fmt.Sprintf("*** Page from %s (Node %d) ***", msg.FromAlias, msg.FromNode)
	case MessageTypePageResponse:
		return fmt.Sprintf("*** %s accepted your page ***", msg.FromAlias)
	case MessageTypeSystem:
		return fmt.Sprintf("*** %s ***", msg.Content)
	case MessageTypeAnnouncement:
		return fmt.Sprintf("*** SYSOP: %s ***", msg.Content)
	default:
		return fmt.Sprintf("[%s] %s", msg.FromAlias, msg.Content)
	}
}
