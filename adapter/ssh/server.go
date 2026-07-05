// Package ssh implements an SSH server for tresbbs.
//
// SSH provides encrypted terminal access to the BBS, which is much more
// secure than plain telnet for internet-facing deployments.
//
// The SSH server supports password authentication against the BBS user
// database and optional public key authentication.
package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"crypto/x509"

	"golang.org/x/crypto/ssh"

	"github.com/jasondostal/tresbbs/internal/auth"
	"github.com/jasondostal/tresbbs/port"
)

// Server is the SSH BBS server.
type Server struct {
	listener net.Listener
	config   *ssh.ServerConfig
}

// NewServer creates a new SSH server.
// addr is the listen address (e.g., ":2222").
// hostKeyPath is the path to the SSH host key file.
// storage is used for password authentication.
func NewServer(addr string, hostKeyPath string, storage port.StoragePort) (*Server, error) {
	// Load or generate host key
	hostKey, err := loadOrGenerateHostKey(hostKeyPath)
	if err != nil {
		return nil, fmt.Errorf("loading host key: %w", err)
	}

	// Configure SSH server
	config := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			return handlePasswordAuth(conn, password, storage)
		},
	}
	config.AddHostKey(hostKey)

	// Start listening
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("ssh listen: %w", err)
	}

	return &Server{
		listener: listener,
		config:   config,
	}, nil
}

// Addr returns the listener address.
func (s *Server) Addr() string {
	return s.listener.Addr().String()
}

// Accept waits for and returns the next SSH connection.
// Returns a session implementing port.SessionPort.
func (s *Server) Accept() (*SSHConn, error) {
	conn, err := s.listener.Accept()
	if err != nil {
		return nil, err
	}

	// Perform SSH handshake
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, s.config)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("ssh handshake: %w", err)
	}

	// Discard global requests
	go ssh.DiscardRequests(reqs)

	// Handle channel requests in background
	go s.handleChannels(sshConn, chans)

	// Accept the first session channel
	channel, err := s.acceptSessionChannel(chans)
	if err != nil {
		sshConn.Close()
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &SSHConn{
		conn:       sshConn,
		channel:    channel,
		remoteAddr: sshConn.RemoteAddr().String(),
		connected:  time.Now(),
		ctx:        ctx,
		cancel:     cancel,
	}, nil
}

// acceptSessionChannel accepts the first "session" channel from the client.
func (s *Server) acceptSessionChannel(chans <-chan ssh.NewChannel) (ssh.Channel, error) {
	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unsupported channel type")
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			return nil, fmt.Errorf("accepting channel: %w", err)
		}

		// Handle channel requests (pty-req, shell, etc.)
		go s.handleChannelRequests(requests)

		return channel, nil
	}

	return nil, fmt.Errorf("no session channel")
}

// handleChannels handles incoming channel requests.
func (s *Server) handleChannels(conn *ssh.ServerConn, chans <-chan ssh.NewChannel) {
	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unsupported channel type")
			continue
		}
		// Accept additional channels but we don't use them
		_, _, err := newChannel.Accept()
		if err != nil {
			log.Printf("SSH channel accept error: %v", err)
			return
		}
	}
}

// handleChannelRequests handles requests on a channel (pty-req, shell, etc.).
func (s *Server) handleChannelRequests(reqs <-chan *ssh.Request) {
	for req := range reqs {
		switch req.Type {
		case "pty-req":
			// PTY request - we accept it but don't need to do anything special
			// The terminal size is negotiated through this request
			if req.WantReply {
				req.Reply(true, nil)
			}
		case "shell":
			// Shell request - we accept it
			if req.WantReply {
				req.Reply(true, nil)
			}
		case "window-change":
			// Terminal size change - we could track this
			if req.WantReply {
				req.Reply(true, nil)
			}
		default:
			// Unknown request - reject
			if req.WantReply {
				req.Reply(false, nil)
			}
		}
	}
}

// Close shuts down the server.
func (s *Server) Close() error {
	return s.listener.Close()
}

// handlePasswordAuth handles password authentication.
func handlePasswordAuth(conn ssh.ConnMetadata, password []byte, storage port.StoragePort) (*ssh.Permissions, error) {
	username := conn.User()

	// Look up user in storage
	user, err := storage.GetUserByName(username)
	if err != nil {
		log.Printf("SSH auth failed: user %q not found: %v", username, err)
		return nil, fmt.Errorf("authentication failed")
	}

	// Check if account is locked
	if user.LockedOut {
		log.Printf("SSH auth failed: user %q is locked out", username)
		return nil, fmt.Errorf("account locked")
	}

	// Check password
	if !auth.CheckPassword(string(password), user.Password) {
		log.Printf("SSH auth failed: invalid password for user %q", username)
		return nil, fmt.Errorf("authentication failed")
	}

	log.Printf("SSH auth success: user %q from %s", username, conn.RemoteAddr())

	return &ssh.Permissions{
		Extensions: map[string]string{
			"username": username,
		},
	}, nil
}

// loadOrGenerateHostKey loads an SSH host key or generates a new one.
func loadOrGenerateHostKey(path string) (ssh.Signer, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("creating key directory: %w", err)
	}

	// Try to load existing key
	if _, err := os.Stat(path); err == nil {
		keyBytes, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading host key: %w", err)
		}
		return ssh.ParsePrivateKey(keyBytes)
	}

	// Generate new Ed25519 key
	log.Printf("Generating new SSH host key at %s", path)
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating key: %w", err)
	}

	// Convert to PKCS8 PEM
	privBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("marshaling private key: %w", err)
	}

	pemBlock := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privBytes,
	}

	// Write to file with restrictive permissions
	if err := os.WriteFile(path, pem.EncodeToMemory(pemBlock), 0600); err != nil {
		return nil, fmt.Errorf("writing key file: %w", err)
	}

	// Parse the key we just wrote
	keyBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading generated key: %w", err)
	}

	return ssh.ParsePrivateKey(keyBytes)
}
