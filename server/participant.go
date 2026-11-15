package server

import (
	"io"
	"log"
	"net"
	"sync"
	"time"
)

const (
	// outboxSize is how many pending messages a slow client may accumulate
	// before new ones are dropped for it. One slow reader must never block
	// the whole chat.
	outboxSize   = 256
	writeTimeout = 10 * time.Second
)

// participant is a connected client that has chosen a name.
//
// All writes to the connection go through the outbox channel and are
// performed by a single writer goroutine, so concurrent broadcasts never
// interleave bytes on the same socket.
type participant struct {
	name string
	conn net.Conn

	outbox   chan string
	done     chan struct{}
	stopOnce sync.Once
}

func newParticipant(name string, conn net.Conn) *participant {
	return &participant{
		name:   name,
		conn:   conn,
		outbox: make(chan string, outboxSize),
		done:   make(chan struct{}),
	}
}

// enqueue schedules msg for delivery without blocking. It is safe to call
// while holding the server lock.
func (p *participant) enqueue(msg string) {
	select {
	case p.outbox <- msg:
	default:
		log.Printf("outbox of %q is full, dropping a message", p.name)
	}
}

// writeLoop drains the outbox until stop is called or a write fails.
// On a write error the connection is closed, which makes the reader
// goroutine return and the participant leave the chat.
func (p *participant) writeLoop() {
	for {
		select {
		case msg := <-p.outbox:
			_ = p.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if _, err := io.WriteString(p.conn, msg); err != nil {
				log.Printf("write to %q failed: %v", p.name, err)
				_ = p.conn.Close()
				return
			}
		case <-p.done:
			return
		}
	}
}

// stop terminates the writer goroutine. It is idempotent.
func (p *participant) stop() {
	p.stopOnce.Do(func() { close(p.done) })
}
