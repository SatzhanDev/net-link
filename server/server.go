// Package server implements the TCP group chat server.
package server

import (
	"errors"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"net-link/internal/chatprofile"
)

// DefaultMaxConnections is the connection limit required by the subject.
const DefaultMaxConnections = 10

// ErrServerClosed is returned by Serve after Close has been called.
var ErrServerClosed = errors.New("server: closed")

const chatFullMessage = "Chat is full, please try again later.\n"

// Server is a group chat server. Create it with New and run it with Serve.
//
// Concurrency model:
//   - one goroutine accepts connections;
//   - one goroutine per connection reads from it;
//   - one goroutine per named participant writes to it (see participant);
//   - slots is a buffered channel used as a semaphore for the connection limit;
//   - mu protects members, history, the profiler counters, the chat log
//     and the bookkeeping maps.
type Server struct {
	slots chan struct{}
	now   func() time.Time // replaceable in tests

	mu      sync.Mutex
	ln      net.Listener
	closed  bool
	conns   map[net.Conn]struct{}     // every open connection, named or not
	members map[*participant]struct{} // clients that have joined the chat
	history []string                  // chat messages, oldest first
	profile *chatprofile.Session      // activity counters for the profiler
	chatLog *log.Logger               // optional chat log (see SetChatLog)

	wg sync.WaitGroup
}

// SetChatLog makes the server write every chat event (messages, joins,
// leaves, renames, flood warnings) to w, one line per event, in the same
// format clients see. Call it before Serve. A nil w disables the log.
func (s *Server) SetChatLog(w io.Writer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if w == nil {
		s.chatLog = nil
		return
	}
	// No prefix and no flags: every line already carries its timestamp.
	s.chatLog = log.New(w, "", 0)
}

// New creates a server that accepts at most maxConns simultaneous connections.
// A non-positive value falls back to DefaultMaxConnections.
func New(maxConns int) *Server {
	if maxConns <= 0 {
		maxConns = DefaultMaxConnections
	}
	// The session (for the pace calculation) starts when the server is
	// created, i.e. right before it starts listening.
	return &Server{
		slots:   make(chan struct{}, maxConns),
		now:     time.Now,
		conns:   make(map[net.Conn]struct{}),
		members: make(map[*participant]struct{}),
		profile: chatprofile.NewSession(time.Now()),
	}
}

// Serve accepts connections on ln until Close is called or ln fails.
// It always returns a non-nil error; after Close it is ErrServerClosed.
func (s *Server) Serve(ln net.Listener) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = ln.Close()
		return ErrServerClosed
	}
	s.ln = ln
	s.mu.Unlock()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if s.isClosed() {
				return ErrServerClosed
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				log.Printf("accept: %v, retrying", err)
				time.Sleep(50 * time.Millisecond)
				continue
			}
			return err
		}

		// Connection limit: take a slot without blocking. The slot is
		// held from accept until the connection closes, so clients that are
		// still typing their name count towards the limit as well.
		select {
		case s.slots <- struct{}{}:
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				defer func() { <-s.slots }()
				s.handleConn(conn)
			}()
		default:
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				reject(conn)
			}()
		}
	}
}

// Close stops accepting connections, disconnects every client and waits
// for all connection goroutines to finish.
func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	var err error
	if s.ln != nil {
		err = s.ln.Close()
	}
	for c := range s.conns {
		_ = c.Close()
	}
	s.mu.Unlock()

	s.wg.Wait()
	return err
}

func (s *Server) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// reject tells a client that the chat is full and closes the connection.
func reject(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
	_, _ = io.WriteString(conn, chatFullMessage)
}

// trackConn registers an open connection so Close can interrupt it.
// It returns false if the server is already shutting down.
func (s *Server) trackConn(c net.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.conns[c] = struct{}{}
	return true
}

func (s *Server) untrackConn(c net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.conns, c)
}
