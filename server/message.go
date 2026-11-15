package server

import (
	"strings"
	"time"
)

const (
	// timeLayout matches the subject: [2020-01-20 15:48:41]
	timeLayout = "2006-01-02 15:04:05"
	systemName = "System"

	// clearLine moves the cursor to the line start and erases it, so an
	// incoming message replaces the recipient's empty "[time][name]:" prompt
	// instead of being glued to it. nc passes it straight to the terminal.
	clearLine = "\r\033[K"
)

func timestamp(t time.Time) string {
	return "[" + t.Format(timeLayout) + "]"
}

// formatMessage builds a chat line: [2020-01-20 15:48:41][name]:text
func formatMessage(t time.Time, name, text string) string {
	return timestamp(t) + "[" + name + "]:" + text
}

// formatSystem builds a server notice: [2020-01-20 15:48:41][System]: text
func formatSystem(t time.Time, text string) string {
	return timestamp(t) + "[" + systemName + "]: " + text
}

// prompt is what a client sees while typing: [2020-01-20 15:48:41][name]:
func prompt(t time.Time, name string) string {
	return timestamp(t) + "[" + name + "]:"
}

// sanitize removes control characters (including ESC) so that a client
// cannot inject terminal escape sequences into other users' screens.
// Tabs are kept because they are harmless.
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return r
		}
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, s)
}
