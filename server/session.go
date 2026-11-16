package server

import (
	"bufio"
	"errors"
	"io"
	"log"
	"net"
	"strings"
)

// maxLineLen limits a single line from a client. A longer line ends the
// session: it protects the server from unbounded memory use.
const maxLineLen = 64 * 1024

// handleConn runs the whole life cycle of one connection:
// greeting -> name -> chat -> leave.
func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	if !s.trackConn(conn) {
		return
	}
	defer s.untrackConn(conn)

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 4096), maxLineLen)

	if _, err := io.WriteString(conn, welcomeBanner); err != nil {
		return
	}

	p, err := s.register(conn, scanner)
	if err != nil {
		// The client disconnected before choosing a name: nobody knew
		// about them, so nothing is announced.
		return
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		p.writeLoop()
	}()
	defer p.stop()

	s.chat(p, scanner)
	s.leave(p)

	if err := scanner.Err(); err != nil && !errors.Is(err, net.ErrClosed) {
		log.Printf("read from %q: %v", p.name, err)
	}
}

// register asks for a name until a valid, free one is entered and the
// client has joined the chat.
func (s *Server) register(conn net.Conn, scanner *bufio.Scanner) (*participant, error) {
	for {
		if _, err := io.WriteString(conn, namePrompt); err != nil {
			return nil, err
		}
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return nil, err
			}
			return nil, io.EOF
		}

		name := strings.TrimSpace(sanitize(scanner.Text()))
		if err := validateName(name); err != nil {
			if _, werr := io.WriteString(conn, "Invalid name: "+err.Error()+"\n"); werr != nil {
				return nil, werr
			}
			continue
		}

		p := newParticipant(name, conn)
		err := s.join(p)
		switch {
		case err == nil:
			return p, nil
		case errors.Is(err, ErrServerClosed):
			return nil, err
		}
		if _, werr := io.WriteString(conn, "Invalid name: "+err.Error()+"\n"); werr != nil {
			return nil, werr
		}
	}
}

// chat reads lines from the client and publishes them until the client
// disconnects. Empty (whitespace-only) lines are never broadcast.
func (s *Server) chat(p *participant, scanner *bufio.Scanner) {
	for scanner.Scan() {
		// sanitize also strips the '\r' that some clients send before '\n'.
		text := sanitize(scanner.Text())
		if strings.TrimSpace(text) == "" {
			p.enqueue(prompt(s.now(), p.name))
			continue
		}
		s.publish(p, text)
	}
}
