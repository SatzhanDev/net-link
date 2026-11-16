// Package client implements the client side of the chat: a minimal
// NetCat-like program that connects to the server, sends everything typed
// on its input and prints everything the server sends back.
package client

import (
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	// DefaultDialTimeout limits how long Dial waits for the server.
	DefaultDialTimeout = 5 * time.Second

	// closeTimeout is how long Run waits for the server to finish the
	// conversation after the input has ended (Ctrl+D or end of a pipe).
	closeTimeout = 5 * time.Second
)

// ErrConnectionClosed is returned by Run when the server ends the
// connection while the user is still connected (server stopped, chat full).
var ErrConnectionClosed = errors.New("connection closed by server")

// Client is a single connection to the chat server.
type Client struct {
	conn net.Conn
}

// Dial connects to the server at addr ("host:port").
func Dial(addr string, timeout time.Duration) (*Client, error) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to %s: %w", addr, err)
	}
	return &Client{conn: conn}, nil
}

// Close closes the connection. Run closes it too, so calling Close is only
// needed when Run is never called.
func (c *Client) Close() error {
	return c.conn.Close()
}

// Run copies in -> server and server -> out at the same time, one goroutine
// per direction, until the conversation is over:
//
//   - the server closes the connection first: returns ErrConnectionClosed;
//   - the input ends first (like `nc -N`): the client half-closes the
//     connection so the server sees EOF and lets the others know the user
//     has left, then waits for the server to close and returns nil.
//
// If in is a terminal, the goroutine reading it may stay blocked after Run
// returns; that is fine because the program exits right after.
func (c *Client) Run(in io.Reader, out io.Writer) error {
	defer c.conn.Close()

	received := make(chan error, 1)
	sent := make(chan error, 1)

	go func() {
		_, err := io.Copy(out, c.conn)
		received <- err
	}()
	go func() {
		_, err := io.Copy(c.conn, in)
		sent <- err
	}()

	select {
	case err := <-received:
		if isUnexpected(err) {
			return fmt.Errorf("receive: %w", err)
		}
		return ErrConnectionClosed

	case err := <-sent:
		if err != nil {
			// Writing failed: the connection is broken.
			_ = c.conn.Close()
			<-received
			return fmt.Errorf("send: %w", err)
		}
		c.closeWrite()
		_ = c.conn.SetReadDeadline(time.Now().Add(closeTimeout))
		if err := <-received; isUnexpected(err) {
			return fmt.Errorf("receive: %w", err)
		}
		return nil
	}
}

// closeWrite tells the server "no more data from me" while still letting
// the client read the server's last messages. Non-TCP connections (used in
// tests) do not support half-close, so they are closed completely.
func (c *Client) closeWrite() {
	if hc, ok := c.conn.(interface{ CloseWrite() error }); ok {
		_ = hc.CloseWrite()
		return
	}
	_ = c.conn.Close()
}

// isUnexpected reports whether a copy error is a real failure and not just
// the normal end of the connection.
func isUnexpected(err error) bool {
	return err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF)
}
