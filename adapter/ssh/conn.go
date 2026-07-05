package ssh

import (
	"context"
	"io"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/jasondostal/tresbbs/adapter/ansi"
	"github.com/jasondostal/tresbbs/port"
)

// Ensure SSHConn implements port.SessionPort.
var _ port.SessionPort = (*SSHConn)(nil)

// SSHConn represents a single SSH connection implementing port.SessionPort.
type SSHConn struct {
	conn       *ssh.ServerConn
	channel    ssh.Channel
	display    *ansi.Display
	remoteAddr string
	connected  time.Time
	ctx        context.Context
	cancel     context.CancelFunc
}

// Display returns the ANSI display for this connection.
func (c *SSHConn) Display() port.DisplayPort {
	if c.display == nil {
		c.display = ansi.New(c.channel, c.channel)
	}
	return c.display
}

// RemoteAddr returns the remote address of the SSH client.
func (c *SSHConn) RemoteAddr() string {
	return c.remoteAddr
}

// ConnectedAt returns when the connection was established.
func (c *SSHConn) ConnectedAt() time.Time {
	return c.connected
}

// Context returns the session context for cancellation.
func (c *SSHConn) Context() context.Context {
	return c.ctx
}

// Close closes the SSH connection and channel.
func (c *SSHConn) Close() error {
	c.cancel()
	c.channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
	c.channel.Close()
	return c.conn.Close()
}

// Reader returns the underlying reader from the SSH channel.
func (c *SSHConn) Reader() io.Reader {
	return c.channel
}

// Writer returns the underlying writer to the SSH channel.
func (c *SSHConn) Writer() io.Writer {
	return c.channel
}
