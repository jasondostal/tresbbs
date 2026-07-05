// Package telnet implements a Telnet server for tresbbs.
//
// In original TriBBS, callers dialed in via modem. In tresbbs, they connect
// via Telnet (or SSH). This adapter maps a TCP connection to a BBS session —
// each connection is a "phone line" that goes through the login sequence,
// session, and logoff.
package telnet

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/jasondostal/tresbbs/adapter/ansi"
	"github.com/jasondostal/tresbbs/domain"
	"github.com/jasondostal/tresbbs/port"
)

// Ensure we implement the port interface.
var _ port.SessionPort = (*Session)(nil)

// Server is the Telnet BBS server.
type Server struct {
	listener net.Listener
	sessions sync.Map
	nextNode int
	mu       sync.Mutex
}

// Session represents a single Telnet connection.
type Session struct {
	conn       net.Conn
	display    *ansi.Display
	remoteAddr string
	connected  time.Time
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewServer creates a new Telnet server.
func NewServer(addr string) (*Server, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("telnet listen: %w", err)
	}

	return &Server{
		listener: ln,
		nextNode: 1,
	}, nil
}

// Addr returns the listener address.
func (s *Server) Addr() string {
	return s.listener.Addr().String()
}

// Accept waits for and returns the next connection.
func (s *Server) Accept() (*Session, error) {
	conn, err := s.listener.Accept()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	sess := &Session{
		conn:       conn,
		display:    ansi.New(conn, conn),
		remoteAddr: conn.RemoteAddr().String(),
		connected:  time.Now(),
		ctx:        ctx,
		cancel:     cancel,
	}

	s.mu.Lock()
	nodeNum := s.nextNode
	s.nextNode++
	s.mu.Unlock()

	s.sessions.Store(nodeNum, sess)

	return sess, nil
}

// Close shuts down the server.
func (s *Server) Close() error {
	return s.listener.Close()
}

// Display implements SessionPort.
func (sess *Session) Display() port.DisplayPort {
	return sess.display
}

// RemoteAddr implements SessionPort.
func (sess *Session) RemoteAddr() string {
	return sess.remoteAddr
}

// ConnectedAt implements SessionPort.
func (sess *Session) ConnectedAt() time.Time {
	return sess.connected
}

// Context implements SessionPort.
func (sess *Session) Context() context.Context {
	return sess.ctx
}

// Close implements SessionPort.
func (sess *Session) Close() error {
	sess.cancel()
	return sess.conn.Close()
}

// Reader implements SessionPort.
func (sess *Session) Reader() io.Reader {
	return sess.conn
}

// Writer implements SessionPort.
func (sess *Session) Writer() io.Writer {
	return sess.conn
}

// Serve runs the BBS session loop for this connection.
// This is called by the main server after a connection is accepted.
func Serve(sess *Session, storage port.StoragePort, nodes port.NodePort, config *domain.Config) {
	defer sess.Close()

	d := sess.display

	// Small delay to let telnet negotiation happen
	time.Sleep(100 * time.Millisecond)

	d.Clear()

	// Welcome screen
	d.SetColor('A') // Green
	d.WriteLine("╔══════════════════════════════════════════════════════════════╗")
	d.WriteLine("║                                                            ║")
	d.SetColor('E') // Yellow
	d.WriteLine("║                    Welcome to TresBBS                       ║")
	d.SetColor('A') // Green
	d.WriteLine("║                                                            ║")
	d.SetColor('B') // Cyan
	d.WriteLine(fmt.Sprintf("║  %-58s  ║", config.BoardName))
	d.SetColor('A') // Green
	d.WriteLine("║                                                            ║")
	d.WriteLine("╚══════════════════════════════════════════════════════════════╝")
	d.ResetColor()
	d.WriteLine("")

	// Login sequence
	d.SetColor('E')
	d.Write("User Name: ")
	d.ResetColor()
	name, _ := d.ReadLine()

	if name == "" {
		d.WriteLine("No name entered. Goodbye!")
		return
	}

	// Look up user
	user, err := storage.GetUserByName(name)
	if err != nil {
		// New user?
		d.SetColor('C')
		d.Write("New user? (y/n): ")
		d.ResetColor()
		choice, _ := d.ReadLine()
		if choice == "y" || choice == "Y" {
			// New user registration
			d.SetColor('E')
			d.Write("Choose an alias: ")
			d.ResetColor()
			alias, _ := d.ReadLine()

			user = &domain.User{
				Name:           name,
				Alias:          alias,
				SecurityLevel:  config.NewUserSecurity,
				ANSIMode:       1,
				ScreenWidth:    80,
				TimeLeftToday:  config.NewUserTimeLimit,
				DailyFileLimit: 10,
				DailyByteLimit: 1024,
			}

			if _, err := storage.AddUser(user); err != nil {
				d.SetColor('C')
				d.WriteLine(fmt.Sprintf("Error creating user: %v", err))
				return
			}

			storage.LogCaller(fmt.Sprintf("NEW USER added: %s", name))
			d.SetColor('A')
			d.WriteLine("Welcome to the BBS!")
		} else {
			d.WriteLine("Goodbye!")
			return
		}
	} else {
		// Existing user - check password
		if user.LockedOut {
			d.SetColor('C')
			d.WriteLine("Your account has been locked. Contact the sysop.")
			return
		}

		d.SetColor('E')
		d.Write("Password: ")
		d.ResetColor()
		password, _ := d.ReadPassword()

		// In a real implementation, we'd verify the password hash
		// For now, accept any password
		_ = password

		user.CallsToday++
		user.TotalCalls++
		storage.SaveUser(user)

		storage.LogCaller(fmt.Sprintf("%s logged on at %s", user.Name, time.Now().Format("03:04 PM")))
	}

	// Main session loop
	running := true
	for running {
		d.Clear()
		d.SetColor('A')
		d.WriteLine("╔══════════════════════════════════════════════════════════════╗")
		d.SetColor('E')
		d.WriteLine(fmt.Sprintf("║  Welcome, %-49s ║", user.Alias))
		d.SetColor('A')
		d.WriteLine("╠══════════════════════════════════════════════════════════════╣")
		d.SetColor('B')
		d.WriteLine("║                                                            ║")
		d.SetColor('F')
		d.WriteLine("║   [M] Messages        [F] Files         [D] Doors          ║")
		d.WriteLine("║   [C] Chat            [W] Who's Online  [U] User Config    ║")
		d.WriteLine("║   [B] Bulletins       [G] Goodbye                          ║")
		d.SetColor('B')
		d.WriteLine("║                                                            ║")
		d.SetColor('A')
		d.WriteLine("╚══════════════════════════════════════════════════════════════╝")
		d.ResetColor()
		d.WriteLine("")
		d.SetColor('E')
		d.Write("Enter Selection: ")
		d.ResetColor()

		choice, _ := d.ReadLine()
		if len(choice) == 0 {
			continue
		}

		switch choice[0] {
		case 'g', 'G':
			d.SetColor('A')
			d.WriteLine("Thanks for calling! Goodbye!")
			storage.LogCaller(fmt.Sprintf("%s logged off", user.Name))
			running = false

		case 'w', 'W':
			// Who's online
			d.Clear()
			d.SetColor('A')
			d.WriteLine("=== Who's Online ===")
			d.ResetColor()
			nodeList := nodes.GetNodes()
			for _, n := range nodeList {
				if n.Active {
					d.SetColor('E')
					d.WriteLine(fmt.Sprintf("  Node %d: %s (%s)", n.NodeNumber, n.UserAlias, n.Activity))
				}
			}
			d.SetColor('B')
			d.Write("Press Enter to continue...")
			d.ReadLine()

		case 'm', 'M':
			// Messages - placeholder
			d.Clear()
			d.SetColor('A')
			d.WriteLine("=== Message Conferences ===")
			d.ResetColor()
			confs, _ := storage.GetConferences()
			for i, c := range confs {
				d.SetColor('E')
				d.WriteLine(fmt.Sprintf("  %d. %s", i+1, c.Name))
			}
			d.SetColor('B')
			d.Write("Press Enter to continue...")
			d.ReadLine()

		default:
			d.SetColor('C')
			d.WriteLine("Not implemented yet!")
			time.Sleep(time.Second)
		}
	}
}
