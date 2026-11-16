# net-link

A NetCat-like group chat over TCP, written in Go with the standard library only.
One server serves up to 10 clients. Each new client gets the chat history and a
live **session profile**: how busy the room is and who talks the most.

## Usage

```console
$ go run .
Listening on the port :8989
$ go run . 2525
Listening on the port :2525
$ go run . 2525 localhost
[USAGE]: ./TCPChat $port
```

Connect with `nc`, or with the built-in client:

```console
$ nc localhost 2525
$ go run . -c localhost 2525
```

To build a binary that matches the usage message:

```console
$ go build -o TCPChat .
$ ./TCPChat 2525
```

### In the chat

| Input | Effect |
|---|---|
| any text | sent to everyone else as `[2020-01-20 15:48:41][name]:text` |
| empty line | ignored, never broadcast |
| `/name NewName` | changes your name; the whole group is informed |
| `Ctrl+C` (nc) or `Ctrl+D` (built-in client) | leaves the chat |

## Features

**Required**
- TCP server with 1-to-many clients, at most 10 connections at a time. Further clients get `Chat is full, please try again later.`
- Linux logo greeting and `[ENTER YOUR NAME]:`. The name must be non-empty and unique.
- Messages are formatted `[time][name]:text`. They are not echoed back to the sender.
- New clients receive the whole message history.
- `[System]` notices when someone joins or leaves. The other clients stay connected.
- Default port `8989`. Any other arguments print `[USAGE]: ./TCPChat $port`.

**Chat Activity Profiler** (`internal/chatprofile`)
- Right after the history every new client gets one of these lines:
  ```
  [System]: Session profile | pace: active | top talker: Yenlik (8 msgs) | total: 15 msgs
  [System]: Session profile | pace: quiet | no messages yet
  ```
- Pace is total messages ÷ minutes since the server started:

  | messages per minute | label |
  |---|---|
  | `< 3` | quiet |
  | `3 … 10` (inclusive) | active |
  | `> 10` | lively |

- Bonus: a client that sends more than 5 messages within 30 seconds gets a private
  `[System]: You are sending messages too fast. Please slow down.`

**Other bonuses**
- Name change with `/name NewName`, announced to the group.
- Chat log: every message and event is appended to `chat.log` in the working directory.
- Client mode: `-c host port`.

## Project structure

```
net-link/
├── main.go                     argument parsing, server / client mode
├── server/
│   ├── server.go               accept loop, connection limit, Close
│   ├── session.go              one connection: greeting → name → chat, /name command
│   ├── chat.go                 room logic: join, leave, publish, rename, broadcast, log
│   ├── participant.go          per-client outbox and writer goroutine
│   ├── message.go              line formats, input sanitizing
│   ├── name.go                 name validation
│   └── banner.go               welcome text and logo
├── client/
│   └── client.go               Dial + Run: stdin → server, server → stdout
└── internal/chatprofile/
    └── profiler.go             pace label, top talker, summary, flood detection
```

## Concurrency model

- **Goroutines:** one accepts connections, one reads from each connection, one writes to each named client. The built-in client uses one goroutine per direction.
- **Channel as a semaphore:** `slots` has a buffer of 10. A connection takes a slot without blocking, or is rejected. It gives the slot back when it closes.
- **Channel as an outbox:** every client has a buffered queue that a single writer goroutine drains. Concurrent broadcasts therefore never mix bytes on one socket, and a slow client cannot block the chat.
- **Mutex:** `Server.mu` guards the members, history, profiler counters and chat log. Join, publish, rename and leave each run under the lock. Every client therefore sees events in the same order, and a newcomer sees every message exactly once: in the history or live.

## Assumptions

These are also documented in code comments.
- Names are trimmed, at most 32 characters, case-insensitively unique, and may not contain `[` or `]`. `System` is reserved. This prevents impersonating the server.
- Control characters, including ANSI escape sequences, are removed from user input so that nobody can mess with other users' terminals.
- The history contains chat messages only, not join or leave notices. This matches the example in the subject.
- The session profile and the flood warning have no timestamp, exactly as shown in the subject. Join, leave and rename notices have one.
- The session for the pace calculation starts when the server starts. With messages but zero elapsed time, the pace is `lively`.
- On a tie, the top talker is the name that comes first alphabetically, so the result is deterministic.
- A flooding client is only warned, not blocked. The warning repeats for every message above the limit inside the 30-second window.
- When a client renames itself, its message count moves to the new name.
- The server overwrites its prompt line with `\r\033[K` before printing an incoming message. In a terminal the output then looks like the subject example.

## Tests

```console
$ go test ./...
$ go test -race ./...
```

- `internal/chatprofile`: each helper on its own, including pace boundaries (2.999, 3, 10, 10.001), ties, and the sliding window.
- `server`: unit tests for formatting and validation, plus integration tests over real TCP: greeting, names, history, broadcasts, no echo, empty messages, join and leave notices, connection limit, profile line, flood warning, rename, chat log, and flushing output on half-close.
- `client`: unreachable server, send and receive, server hang-up, and a full round trip with the real server.
- `main`: argument parsing for both modes.

Allowed packages only: `io`, `log`, `os`, `fmt`, `net`, `sync`, `time`, `bufio`, `errors`, `strings` (plus `testing` in tests).
