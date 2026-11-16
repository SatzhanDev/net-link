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
	done     chan struct{} // closed by stop: "flush and exit"
	finished chan struct{} // closed by writeLoop when it has exited
	stopOnce sync.Once

	// recent holds the times of this client's messages inside the flood
	// window. It is only touched under Server.mu.
	recent []time.Time
}

func newParticipant(name string, conn net.Conn) *participant {
	return &participant{
		name:     name,
		conn:     conn,
		outbox:   make(chan string, outboxSize),
		done:     make(chan struct{}),
		finished: make(chan struct{}),
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
// finished is closed when the loop exits.
func (p *participant) writeLoop() {
	defer close(p.finished)
	for {
		select {
		case msg := <-p.outbox:
			if !p.write(msg) {
				return
			}
		case <-p.done:
			p.flush()
			return
		}
	}
}

// flush sends whatever is still queued. It is called on stop so that a
// client that half-closes its side (e.g. `echo name | nc -N`) still gets
// the history, the profile and every message queued before it left.
func (p *participant) flush() {
	for {
		select {
		case msg := <-p.outbox:
			if !p.write(msg) {
				return
			}
		default:
			return
		}
	}
}

// write sends one message, bounded by writeTimeout so that a client that
// stopped reading cannot block the goroutine forever.
func (p *participant) write(msg string) bool {
	_ = p.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	if _, err := io.WriteString(p.conn, msg); err != nil {
		// The remote address is used instead of p.name: the name may be
		// changed by /name, and this goroutine does not hold Server.mu.
		log.Printf("write to %s failed: %v", p.conn.RemoteAddr(), err)
		_ = p.conn.Close()
		return false
	}
	return true
}

// stop asks the writer goroutine to flush the outbox and exit, then waits
// for it. It is idempotent. Nothing may be enqueued after the participant
// has left the chat, so the flush sees everything.
func (p *participant) stop() {
	p.stopOnce.Do(func() { close(p.done) })
	<-p.finished
}
