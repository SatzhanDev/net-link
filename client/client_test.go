package client

import (
	"bufio"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"net-link/server"
)

// safeBuffer is an io.Writer that can be read while another goroutine
// writes into it.
type safeBuffer struct {
	mu sync.Mutex
	sb strings.Builder
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sb.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sb.String()
}

// fakeServer accepts one connection and runs handle on it.
func fakeServer(t *testing.T, handle func(net.Conn)) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		handle(conn)
	}()
	return ln.Addr().String()
}

// runAsync starts c.Run in the background and returns its result channel.
func runAsync(c *Client, in io.Reader, out io.Writer) <-chan error {
	done := make(chan error, 1)
	go func() { done <- c.Run(in, out) }()
	return done
}

func waitRun(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return")
		return nil
	}
}

func TestDialUnreachableServer(t *testing.T) {
	// Take a free port and release it, so nothing listens there.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	c, err := Dial(addr, time.Second)
	if err == nil {
		c.Close()
		t.Fatal("Dial to a closed port must fail")
	}
	if !strings.Contains(err.Error(), "cannot connect to "+addr) {
		t.Errorf("error = %q, want it to mention the address", err)
	}
}

func TestRunSendsInputAndPrintsOutput(t *testing.T) {
	addr := fakeServer(t, func(conn net.Conn) {
		sc := bufio.NewScanner(conn)
		for sc.Scan() {
			io.WriteString(conn, "echo:"+sc.Text()+"\n")
		}
		// EOF from the client: say goodbye, then close.
		io.WriteString(conn, "bye\n")
	})

	c, err := Dial(addr, time.Second)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	var out safeBuffer
	err = waitRun(t, runAsync(c, strings.NewReader("hello\nworld\n"), &out))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// The client must still read what the server sends after the input ends.
	if got, want := out.String(), "echo:hello\necho:world\nbye\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestRunServerClosesConnection(t *testing.T) {
	addr := fakeServer(t, func(conn net.Conn) {
		io.WriteString(conn, "Chat is full, please try again later.\n")
	})

	c, err := Dial(addr, time.Second)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	// The input never ends, like a user sitting at the terminal.
	in, inW := io.Pipe()
	defer inW.Close()

	var out safeBuffer
	err = waitRun(t, runAsync(c, in, &out))
	if !errors.Is(err, ErrConnectionClosed) {
		t.Fatalf("Run error = %v, want ErrConnectionClosed", err)
	}
	if !strings.Contains(out.String(), "Chat is full") {
		t.Errorf("output = %q, want the server message", out.String())
	}
}

// TestClientWithRealServer runs the real chat server and checks that our
// client can join, send a message and leave cleanly.
func TestClientWithRealServer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := server.New(server.DefaultMaxConnections)
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })

	// An observer connected with a raw socket, like nc.
	observer, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial observer: %v", err)
	}
	defer observer.Close()
	io.WriteString(observer, "Observer\n")
	obs := bufio.NewReader(observer)
	readUntil(t, observer, obs, "[Observer]:")

	c, err := Dial(ln.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	var out safeBuffer
	err = waitRun(t, runAsync(c, strings.NewReader("Yenlik\nhello\n"), &out))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	got := out.String()
	for _, want := range []string{"Welcome to TCP-Chat!", "[ENTER YOUR NAME]:", "[System]: Session profile", "[Yenlik]:"} {
		if !strings.Contains(got, want) {
			t.Errorf("client output does not contain %q:\n%s", want, got)
		}
	}

	readUntil(t, observer, obs, "Yenlik has joined our chat.")
	readUntil(t, observer, obs, "[Yenlik]:hello")
	readUntil(t, observer, obs, "Yenlik has left our chat.")
}

func readUntil(t *testing.T, conn net.Conn, r *bufio.Reader, sub string) {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var seen strings.Builder
	for !strings.Contains(seen.String(), sub) {
		b, err := r.ReadByte()
		if err != nil {
			t.Fatalf("waiting for %q: %v\nreceived: %q", sub, err, seen.String())
		}
		seen.WriteByte(b)
	}
}
