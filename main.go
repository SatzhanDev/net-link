// Command net-link is a NetCat-like TCP group chat.
//
// Usage:
//
//	go run .                    # server on the default port 8989
//	go run . 2525               # server on port 2525
//	go run . -c localhost 2525  # client connected to localhost:2525
//
// Any other combination of arguments prints the usage message.
package main

import (
	"errors"
	"fmt"
	"net"
	"os"

	"net-link/client"
	"net-link/server"
)

const (
	defaultPort  = "8989"
	usageMessage = "[USAGE]: ./TCPChat $port"
	clientFlag   = "-c"
	chatLogFile  = "chat.log" // created in the current directory
)

var errUsage = errors.New("invalid arguments")

type mode int

const (
	serverMode mode = iota
	clientMode
)

// config is the result of parsing the command line.
type config struct {
	mode mode
	host string // client mode only
	port string
}

func main() {
	cfg, err := parseArgs(os.Args[1:])
	if err != nil {
		// The subject shows the usage line followed by a normal prompt,
		// so we print it and exit cleanly instead of with a failure code.
		fmt.Println(usageMessage)
		return
	}

	switch cfg.mode {
	case clientMode:
		os.Exit(runClient(cfg.host, cfg.port))
	default:
		os.Exit(runServer(cfg.port))
	}
}

func runServer(port string) int {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to listen on port %s: %v\n", port, err)
		return 1
	}
	fmt.Printf("Listening on the port :%s\n", port)

	srv := server.New(server.DefaultMaxConnections)

	// Bonus: keep all chat events in a file. The chat still works if the
	// file cannot be opened, so this is a warning, not a fatal error.
	logFile, err := os.OpenFile(chatLogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: chat log disabled: %v\n", err)
	} else {
		defer logFile.Close()
		srv.SetChatLog(logFile)
	}

	if err := srv.Serve(ln); err != nil && !errors.Is(err, server.ErrServerClosed) {
		fmt.Fprintf(os.Stderr, "server stopped: %v\n", err)
		return 1
	}
	return 0
}

func runClient(host, port string) int {
	c, err := client.Dial(net.JoinHostPort(host, port), client.DefaultDialTimeout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	err = c.Run(os.Stdin, os.Stdout)
	switch {
	case err == nil:
		return 0
	case errors.Is(err, client.ErrConnectionClosed):
		fmt.Fprintln(os.Stderr, "\nConnection closed by server.")
		return 0
	default:
		fmt.Fprintf(os.Stderr, "\nconnection error: %v\n", err)
		return 1
	}
}

// parseArgs decides between server and client mode.
//
//	[]                 -> server on the default port
//	[port]             -> server on port
//	["-c", host, port] -> client
//
// Everything else is a usage error.
func parseArgs(args []string) (config, error) {
	switch {
	case len(args) == 0:
		return config{mode: serverMode, port: defaultPort}, nil
	case len(args) == 1 && isValidPort(args[0]):
		return config{mode: serverMode, port: args[0]}, nil
	case len(args) == 3 && args[0] == clientFlag && args[1] != "" && isValidPort(args[2]):
		return config{mode: clientMode, host: args[1], port: args[2]}, nil
	default:
		return config{}, errUsage
	}
}

// isValidPort reports whether s is a decimal number in the range 1..65535.
// strconv is not in the list of allowed packages, so the number is parsed by hand.
func isValidPort(s string) bool {
	if s == "" || len(s) > 5 {
		return false
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
		n = n*10 + int(r-'0')
	}
	return n >= 1 && n <= 65535
}
