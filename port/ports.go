// Package port defines the interfaces between the domain and the outside world.
// These are the "ports" in ports-and-adapters (hexagonal) architecture.
//
// The domain uses these interfaces; adapters implement them. This is how we
// swap DOS-era implementations for modern ones:
//
//	FOSSIL/modem  → TelnetPort / SSHPort
//	DOS screen    → DisplayPort (ANSI terminal)
//	Binary files  → StoragePort (SQLite)
//	File semaphores → NodePort (goroutines)
//	SPAWNO        → DoorPort (subprocess)
package port

import (
	"context"
	"io"
	"time"

	"github.com/jasondostal/tresbbs/domain"
)

// ===================================================================
// DisplayPort — Terminal Output
//
// In original TriBBS, this was direct video memory writes or ANSI
// escape sequences over the serial port. In tresbbs, it's an ANSI
// terminal over SSH/Telnet or a Bubble Tea TUI.
//
// The key insight: the template engine calls these methods to render
// the BBS display. The adapter handles the actual terminal protocol.
// ===================================================================

type DisplayPort interface {
	// Clear clears the screen and moves cursor to home position.
	Clear()

	// MoveTo moves the cursor to (row, col), 0-indexed.
	MoveTo(row, col int)

	// Write sends raw text to the terminal.
	Write(text string)

	// WriteLine sends text followed by a newline.
	WriteLine(text string)

	// SetColor sets the foreground color using TriBBS's color codes.
	// This maps @X__ codes to the appropriate terminal escape sequences.
	SetColor(color byte)

	// ResetColor resets to the default terminal color.
	ResetColor()

	// SetReverse sets reverse video mode.
	SetReverse(on bool)

	// ReadLine reads a line of input from the terminal.
	ReadLine() (string, error)

	// ReadKey reads a single keypress.
	ReadKey() (byte, error)

	// ReadPassword reads a password (masked input).
	ReadPassword() (string, error)

	// ShowCursor shows/hides the cursor.
	ShowCursor(show bool)

	// Flush ensures all buffered output is sent.
	Flush()

	// Width returns the terminal width.
	Width() int

	// Height returns the terminal height.
	Height() int

	// DrawBox draws a box with line-drawing characters.
	DrawBox(row, col, width, height int)

	// SetScrollRegion sets the scrolling region.
	SetScrollRegion(top, bottom int)
}

// ===================================================================
// StoragePort — Data Persistence
//
// In original TriBBS, this was binary files (USERS.DAT, FAREA.DAT,
// M*.IDX, etc.). In tresbbs, it's SQLite.
//
// The interface is the same either way — the domain doesn't care
// how data is stored, just that it can load and save records.
// ===================================================================

type StoragePort interface {
	// User operations
	GetUser(recordNumber int) (*domain.User, error)
	GetUserByName(name string) (*domain.User, error)
	GetUserByAlias(alias string) (*domain.User, error)
	SaveUser(user *domain.User) error
	AddUser(user *domain.User) (int, error)
	DeleteUser(recordNumber int) error
	PackUsers() (int, error) // remove deleted, return new count
	ListUsers() ([]domain.User, error)
	UserCount() (int, error)

	// Message operations
	GetConferences() ([]domain.Conference, error)
	GetConference(name string) (*domain.Conference, error)
	AddConference(conf *domain.Conference) error
	SaveConference(conf *domain.Conference) error
	DeleteConference(name string) error
	GetPosts(conference string, limit int) ([]domain.Post, error)
	GetPost(id int64) (*domain.Post, error)
	AddPost(post *domain.Post) error
	DeletePost(id int64) error
	PostCount(conference string) (int64, error)

	// File area operations
	GetFileAreas() ([]domain.FileArea, error)
	GetFileArea(name string) (*domain.FileArea, error)
	AddFileArea(area *domain.FileArea) error
	SaveFileArea(area *domain.FileArea) error
	DeleteFileArea(name string) error
	GetFiles(area string, limit int) ([]domain.FileEntry, error)
	AddFile(file *domain.FileEntry) error
	SaveFile(file *domain.FileEntry) error
	DeleteFile(id int64) error

	// Bulletin operations
	GetBulletins() ([]domain.Bulletin, error)
	GetBulletin(id int) (*domain.Bulletin, error)
	AddBulletin(bulletin *domain.Bulletin) error
	SaveBulletin(bulletin *domain.Bulletin) error
	DeleteBulletin(id int) error

	// Event operations
	GetEvents() ([]domain.Event, error)
	SaveEvent(event *domain.Event) error
	DeleteEvent(id int) error

	// Door operations
	GetDoors() ([]domain.Door, error)
	GetDoor(name string) (*domain.Door, error)
	AddDoor(door *domain.Door) error
	SaveDoor(door *domain.Door) error
	DeleteDoor(name string) error

	// Config operations
	GetConfig() (*domain.Config, error)
	SaveConfig(config *domain.Config) error

	// Caller log
	LogCaller(entry string) error
	GetCallerLog(limit int) ([]string, error)

	// Close closes the storage backend.
	Close() error
}

// ===================================================================
// NodePort — Multinode Coordination
//
// In original TriBBS, this was file-based semaphores on a shared
// network drive. In tresbbs, it's in-memory coordination via goroutines
// and channels.
//
// The interface is the same — the domain doesn't care how nodes
// coordinate, just that they can see each other and communicate.
// ===================================================================

type NodePort interface {
	// RegisterNode registers this node as active.
	RegisterNode(node *domain.NodeStatus) error

	// UpdateNode updates this node's status.
	UpdateNode(status *domain.NodeStatus) error

	// SetChatAvail toggles whether this node accepts inter-node chat pages.
	SetChatAvail(nodeNumber int, avail bool) error

	// UnregisterNode marks this node as inactive.
	UnregisterNode(nodeNumber int) error

	// GetNodes returns the status of all active nodes.
	GetNodes() []domain.NodeStatus

	// GetNode returns a specific node's status.
	GetNode(nodeNumber int) (*domain.NodeStatus, error)

	// SendPage sends a chat request to another node.
	SendPage(fromNode int, toNode int, fromAlias string) error

	// CheckPage checks if this node has a pending page.
	CheckPage(nodeNumber int) (bool, string, error)

	// ClearPage clears a pending page.
	ClearPage(nodeNumber int) error

	// IsDuplicateLogin checks if a user is already logged in on another node.
	IsDuplicateLogin(userName string) (bool, int, error)
}

// ===================================================================
// SessionPort — A Connected User Session
//
// This encapsulates a single user's connection — whether it's SSH,
// Telnet, or a local TUI. It provides the I/O primitives that the
// BBS session loop needs.
// ===================================================================

type SessionPort interface {
	// The display for rendering
	Display() DisplayPort

	// RemoteAddr returns the remote address (for logging).
	RemoteAddr() string

	// ConnectedAt returns when the session started.
	ConnectedAt() time.Time

	// Context returns the session context (for cancellation).
	Context() context.Context

	// Close closes the session.
	Close() error

	// Reader returns the underlying reader (for raw I/O if needed).
	Reader() io.Reader

	// Writer returns the underlying writer.
	Writer() io.Writer
}

// ===================================================================
// DoorRunnerPort — External Door Execution
//
// In original TriBBS, doors were launched via SPAWNO (overlay swap).
// In tresbbs, they're launched as subprocesses.
//
// The door runner writes drop files, executes the door, and reads
// back the results.
// ===================================================================

type DoorRunnerPort interface {
	// RunDoor executes a door program for the given user session.
	RunDoor(door domain.Door, user *domain.User, session SessionPort) error

	// WriteDoorSys writes a DOOR.SYS drop file.
	WriteDoorSys(user *domain.User, path string) error

	// WriteDorinfoDef writes a DORINFO1.DEF drop file.
	WriteDorinfoDef(user *domain.User, path string, node int) error
}
