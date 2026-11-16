package server

import (
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"net-link/internal/chatprofile"
)

const ioTimeout = 2 * time.Second

// startServer runs a server on a random local port and stops it when the
// test ends. The clock is fixed so that output is predictable.
func startServer(t *testing.T, maxConns int) (*Server, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := New(maxConns)
	srv.now = func() time.Time { return fixedTime }
	// The session started one minute before the fixed clock, so the
	// message count equals the rate per minute in these tests.
	srv.profile = chatprofile.NewSession(fixedTime.Add(-time.Minute))

	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()

	t.Cleanup(func() {
		_ = srv.Close()
		if err := <-done; !errors.Is(err, ErrServerClosed) {
			t.Errorf("Serve returned %v, want ErrServerClosed", err)
		}
	})
	return srv, ln.Addr().String()
}

// testClient is a minimal nc replacement that remembers unread output.
type testClient struct {
	t       *testing.T
	conn    net.Conn
	pending string
}

func dial(t *testing.T, addr string) *testClient {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return &testClient{t: t, conn: conn}
}

// join connects and enters name, waiting until the chat prompt appears.
func join(t *testing.T, addr, name string) *testClient {
	t.Helper()
	c := dial(t, addr)
	c.expect(namePrompt)
	c.send(name)
	c.expect(prompt(fixedTime, name))
	return c
}

func (c *testClient) send(line string) {
	c.t.Helper()
	if _, err := c.conn.Write([]byte(line + "\n")); err != nil {
		c.t.Fatalf("write: %v", err)
	}
}

// expect reads until sub appears and consumes output up to and including it.
func (c *testClient) expect(sub string) string {
	c.t.Helper()
	deadline := time.Now().Add(ioTimeout)
	buf := make([]byte, 4096)
	for !strings.Contains(c.pending, sub) {
		_ = c.conn.SetReadDeadline(deadline)
		n, err := c.conn.Read(buf)
		c.pending += string(buf[:n])
		if err != nil && !strings.Contains(c.pending, sub) {
			c.t.Fatalf("waiting for %q: %v\nreceived: %q", sub, err, c.pending)
		}
	}
	i := strings.Index(c.pending, sub) + len(sub)
	out := c.pending[:i]
	c.pending = c.pending[i:]
	return out
}

// drain returns everything that arrives within d.
func (c *testClient) drain(d time.Duration) string {
	c.t.Helper()
	buf := make([]byte, 4096)
	_ = c.conn.SetReadDeadline(time.Now().Add(d))
	for {
		n, err := c.conn.Read(buf)
		c.pending += string(buf[:n])
		if err != nil {
			break
		}
	}
	out := c.pending
	c.pending = ""
	return out
}

// expectClosed waits until the server closes the connection.
func (c *testClient) expectClosed() {
	c.t.Helper()
	buf := make([]byte, 4096)
	_ = c.conn.SetReadDeadline(time.Now().Add(ioTimeout))
	for {
		n, err := c.conn.Read(buf)
		c.pending += string(buf[:n])
		if err == nil {
			continue
		}
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			c.t.Fatal("connection was not closed by the server")
		}
		return
	}
}

func TestGreetingAndNamePrompt(t *testing.T) {
	_, addr := startServer(t, 10)
	c := dial(t, addr)

	got := c.expect(namePrompt)
	if !strings.HasPrefix(got, welcomeBanner) {
		t.Errorf("greeting = %q, want it to start with the banner", got)
	}
}

func TestEmptyNameIsRejected(t *testing.T) {
	_, addr := startServer(t, 10)
	c := dial(t, addr)
	c.expect(namePrompt)

	c.send("   ")
	c.expect("Invalid name: " + ErrNameEmpty.Error())
	c.expect(namePrompt)

	c.send("Yenlik")
	c.expect(prompt(fixedTime, "Yenlik"))
}

func TestDuplicateNameIsRejected(t *testing.T) {
	_, addr := startServer(t, 10)
	join(t, addr, "Yenlik")

	c := dial(t, addr)
	c.expect(namePrompt)
	c.send("yenlik")
	c.expect("Invalid name: " + ErrNameTaken.Error())
}

func TestJoinIsAnnouncedToOthers(t *testing.T) {
	_, addr := startServer(t, 10)
	yenlik := join(t, addr, "Yenlik")
	lee := join(t, addr, "Lee")

	yenlik.expect(formatSystem(fixedTime, "Lee has joined our chat."))

	// The newcomer does not get a notice about themself.
	if out := lee.drain(100 * time.Millisecond); strings.Contains(out, "has joined") {
		t.Errorf("joining client received its own join notice: %q", out)
	}
}

func TestMessagesAreBroadcastButNotEchoed(t *testing.T) {
	_, addr := startServer(t, 10)
	yenlik := join(t, addr, "Yenlik")
	lee := join(t, addr, "Lee")
	yenlik.expect("has joined our chat.")

	yenlik.send("hello")
	lee.expect(formatMessage(fixedTime, "Yenlik", "hello"))

	if out := yenlik.drain(100 * time.Millisecond); strings.Contains(out, "]:hello") {
		t.Errorf("sender received its own message: %q", out)
	}
}

func TestEmptyMessagesAreNotBroadcast(t *testing.T) {
	_, addr := startServer(t, 10)
	yenlik := join(t, addr, "Yenlik")
	lee := join(t, addr, "Lee")
	yenlik.expect("has joined our chat.")

	yenlik.send("")
	yenlik.send("   \t ")
	yenlik.send("real")

	// The first thing Lee receives must be the real message.
	got := lee.expect("]:real")
	if strings.Count(got, "[Yenlik]:") != 1 {
		t.Errorf("empty messages were broadcast: %q", got)
	}
}

func TestHistoryIsSentToNewClient(t *testing.T) {
	_, addr := startServer(t, 10)
	yenlik := join(t, addr, "Yenlik")
	yenlik.send("hello")
	yenlik.send("How are you?")
	yenlik.expect(prompt(fixedTime, "Yenlik")) // wait until processed
	yenlik.expect(prompt(fixedTime, "Yenlik"))

	lee := dial(t, addr)
	lee.expect(namePrompt)
	lee.send("Lee")
	want := formatMessage(fixedTime, "Yenlik", "hello") + "\n" +
		formatMessage(fixedTime, "Yenlik", "How are you?") + "\n" +
		"[System]: Session profile | pace: quiet | top talker: Yenlik (2 msgs) | total: 2 msgs\n" +
		prompt(fixedTime, "Lee")
	if got := lee.expect(prompt(fixedTime, "Lee")); got != want {
		t.Errorf("history = %q, want %q", got, want)
	}
}

func TestProfileForFirstClient(t *testing.T) {
	_, addr := startServer(t, 10)
	c := dial(t, addr)
	c.expect(namePrompt)
	c.send("Yenlik")

	want := "[System]: Session profile | pace: quiet | no messages yet\n" + prompt(fixedTime, "Yenlik")
	if got := c.expect(prompt(fixedTime, "Yenlik")); got != want {
		t.Errorf("greeting after name = %q, want %q", got, want)
	}
}

func TestProfileShowsTopTalkerAndPace(t *testing.T) {
	_, addr := startServer(t, 10)
	yenlik := join(t, addr, "Yenlik")
	lee := join(t, addr, "Lee")
	yenlik.expect("Lee has joined our chat.")

	// 4 messages from Yenlik, 1 from Lee: 5 messages in one minute -> active.
	for _, m := range []string{"a", "b", "c", "d"} {
		yenlik.send(m)
		lee.expect("]:" + m)
	}
	lee.send("e")
	yenlik.expect("]:e")

	bob := dial(t, addr)
	bob.expect(namePrompt)
	bob.send("Bob")
	bob.expect("[System]: Session profile | pace: active | top talker: Yenlik (4 msgs) | total: 5 msgs\n")
}

func TestFloodWarning(t *testing.T) {
	_, addr := startServer(t, 10)
	yenlik := join(t, addr, "Yenlik")
	lee := join(t, addr, "Lee")
	yenlik.expect("Lee has joined our chat.")

	warning := "[System]: " + chatprofile.FloodWarning

	// The clock is frozen, so all messages fall into the same 30s window.
	for i := 1; i <= chatprofile.FloodLimit; i++ {
		yenlik.send("spam")
		lee.expect("]:spam")
	}
	if out := yenlik.drain(100 * time.Millisecond); strings.Contains(out, warning) {
		t.Fatalf("warning sent after only %d messages: %q", chatprofile.FloodLimit, out)
	}

	yenlik.send("one more")
	yenlik.expect(warning)
	// The message is still delivered, and the warning is private.
	if out := lee.expect("]:one more"); strings.Contains(out, warning) {
		t.Errorf("flood warning leaked to another client: %q", out)
	}
}

func TestLeaveIsAnnouncedAndOthersStayConnected(t *testing.T) {
	_, addr := startServer(t, 10)
	yenlik := join(t, addr, "Yenlik")
	lee := join(t, addr, "Lee")
	bob := join(t, addr, "Bob")
	yenlik.expect("Bob has joined our chat.")
	lee.expect("Bob has joined our chat.")

	_ = lee.conn.Close()
	yenlik.expect(formatSystem(fixedTime, "Lee has left our chat."))
	bob.expect(formatSystem(fixedTime, "Lee has left our chat."))

	yenlik.send("still here")
	bob.expect(formatMessage(fixedTime, "Yenlik", "still here"))
}

func TestDisconnectBeforeNameIsSilent(t *testing.T) {
	_, addr := startServer(t, 10)
	yenlik := join(t, addr, "Yenlik")

	anon := dial(t, addr)
	anon.expect(namePrompt)
	_ = anon.conn.Close()

	if out := yenlik.drain(150 * time.Millisecond); strings.Contains(out, "left") {
		t.Errorf("unnamed client produced a leave notice: %q", out)
	}
}

func TestConnectionLimit(t *testing.T) {
	_, addr := startServer(t, 2)
	join(t, addr, "Yenlik")
	lee := join(t, addr, "Lee")

	extra := dial(t, addr)
	extra.expect(chatFullMessage)
	extra.expectClosed()

	// When somebody leaves, the slot becomes free again.
	_ = lee.conn.Close()
	deadline := time.Now().Add(ioTimeout)
	for {
		c := dial(t, addr)
		out := c.expect("\n")
		if strings.Contains(out, "Welcome to TCP-Chat!") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("slot was not released after a client left")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A client may send its input and half-close the connection right away
// (`echo ... | nc -N`, or our client on Ctrl+D). Everything queued for it
// before it left must still be delivered.
func TestQueuedOutputIsFlushedOnHalfClose(t *testing.T) {
	_, addr := startServer(t, 10)
	c := dial(t, addr)
	if _, err := c.conn.Write([]byte("Yenlik\nhello\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := c.conn.(*net.TCPConn).CloseWrite(); err != nil {
		t.Fatalf("CloseWrite: %v", err)
	}

	c.expect("[System]: Session profile | pace: quiet | no messages yet\n")
	c.expect(prompt(fixedTime, "Yenlik")) // after the name
	c.expect(prompt(fixedTime, "Yenlik")) // after "hello"
	c.expectClosed()
}

func TestCloseDisconnectsClients(t *testing.T) {
	srv, addr := startServer(t, 10)
	c := join(t, addr, "Yenlik")

	if err := srv.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	c.expectClosed()
}

func TestRenameIsAnnounced(t *testing.T) {
	_, addr := startServer(t, 10)
	yenlik := join(t, addr, "Yenlik")
	lee := join(t, addr, "Lee")
	yenlik.expect("Lee has joined our chat.")

	notice := formatSystem(fixedTime, "Lee has changed their name to Leo.")
	lee.send("/name Leo")
	lee.expect(notice)                   // the user sees it worked
	lee.expect(prompt(fixedTime, "Leo")) // and gets a prompt with the new name
	yenlik.expect(notice)

	// The command itself is not a chat message, the next message uses the new name.
	lee.send("hi")
	if got := yenlik.expect("]:hi"); strings.Contains(got, "/name") || !strings.Contains(got, "[Leo]:hi") {
		t.Errorf("unexpected output after rename: %q", got)
	}

	// The old name is free again.
	join(t, addr, "Lee")
}

func TestRenameErrors(t *testing.T) {
	_, addr := startServer(t, 10)
	yenlik := join(t, addr, "Yenlik")
	lee := join(t, addr, "Lee")
	yenlik.expect("Lee has joined our chat.")

	for _, tc := range []struct{ cmd, reason string }{
		{"/name", ErrNameEmpty.Error()},
		{"/name yenlik", ErrNameTaken.Error()},
		{"/name Lee", ErrNameSame.Error()},
		{"/name System", ErrNameReserved.Error()},
		{"/name [x]", ErrNameInvalid.Error()},
	} {
		lee.send(tc.cmd)
		lee.expect("[System]: Cannot change name: " + tc.reason)
		lee.expect(prompt(fixedTime, "Lee"))
	}

	// Changing only the letter case of your own name is allowed.
	lee.send("/name LEE")
	yenlik.expect("Lee has changed their name to LEE.")

	if out := yenlik.drain(100 * time.Millisecond); strings.Contains(out, "Cannot change name") {
		t.Errorf("rename errors must be private: %q", out)
	}
}

func TestRenameKeepsProfileCounts(t *testing.T) {
	_, addr := startServer(t, 10)
	yenlik := join(t, addr, "Yenlik")
	lee := join(t, addr, "Lee")
	yenlik.expect("Lee has joined our chat.")

	lee.send("one")
	yenlik.expect("]:one")
	lee.send("two")
	yenlik.expect("]:two")
	lee.send("/name Leo")
	yenlik.expect("changed their name to Leo.")

	bob := dial(t, addr)
	bob.expect(namePrompt)
	bob.send("Bob")
	bob.expect("[System]: Session profile | pace: quiet | top talker: Leo (2 msgs) | total: 2 msgs\n")
}

// lockedBuffer is a strings.Builder that is safe to read while the server writes.
type lockedBuffer struct {
	mu sync.Mutex
	sb strings.Builder
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sb.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sb.String()
}

func TestChatLogFile(t *testing.T) {
	srv, addr := startServer(t, 10)
	var logBuf lockedBuffer
	srv.SetChatLog(&logBuf)

	yenlik := join(t, addr, "Yenlik")
	lee := join(t, addr, "Lee")
	yenlik.expect("Lee has joined our chat.")
	yenlik.send("hello")
	lee.expect("]:hello")
	lee.send("/name Leo")
	yenlik.expect("changed their name to Leo.")
	_ = lee.conn.Close()
	yenlik.expect("Leo has left our chat.")

	want := strings.Join([]string{
		formatSystem(fixedTime, "Yenlik has joined our chat."),
		formatSystem(fixedTime, "Lee has joined our chat."),
		formatMessage(fixedTime, "Yenlik", "hello"),
		formatSystem(fixedTime, "Lee has changed their name to Leo."),
		formatSystem(fixedTime, "Leo has left our chat."),
	}, "\n") + "\n"
	if got := logBuf.String(); got != want {
		t.Errorf("chat log =\n%s\nwant\n%s", got, want)
	}
}
