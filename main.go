// Command net-link is a NetCat-like TCP group chat server.
//
// Usage:
//
//	go run .          # listens on the default port 8989
//	go run . 2525     # listens on port 2525
package main

import (
	"errors"
	"fmt"
	"net"
	"os"

	"net-link/server"
)

const (
	defaultPort  = "8989"
	usageMessage = "[USAGE]: ./TCPChat $port"
)

var errUsage = errors.New("invalid arguments")

func main() {
	port, err := parseArgs(os.Args[1:])
	if err != nil {
		// The subject shows the usage line followed by a normal prompt,
		// so we print it and exit cleanly instead of with a failure code.
		fmt.Println(usageMessage)
		return
	}

	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to listen on port %s: %v\n", port, err)
		os.Exit(1)
	}
	fmt.Printf("Listening on the port :%s\n", port)

	srv := server.New(server.DefaultMaxConnections)
	if err := srv.Serve(ln); err != nil && !errors.Is(err, server.ErrServerClosed) {
		fmt.Fprintf(os.Stderr, "server stopped: %v\n", err)
		os.Exit(1)
	}
}

// parseArgs returns the port to listen on.
// No arguments -> default port; exactly one valid port -> that port;
// anything else is a usage error.
func parseArgs(args []string) (string, error) {
	switch len(args) {
	case 0:
		return defaultPort, nil
	case 1:
		if !isValidPort(args[0]) {
			return "", errUsage
		}
		return args[0], nil
	default:
		return "", errUsage
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
