package server

import (
	"testing"
	"time"
)

var fixedTime = time.Date(2020, 1, 20, 15, 48, 41, 0, time.UTC)

func TestFormatMessage(t *testing.T) {
	got := formatMessage(fixedTime, "Yenlik", "hello")
	want := "[2020-01-20 15:48:41][Yenlik]:hello"
	if got != want {
		t.Errorf("formatMessage() = %q, want %q", got, want)
	}
}

func TestFormatSystem(t *testing.T) {
	got := formatSystem(fixedTime, "Lee has joined our chat.")
	want := "[2020-01-20 15:48:41][System]: Lee has joined our chat."
	if got != want {
		t.Errorf("formatSystem() = %q, want %q", got, want)
	}
}

func TestPrompt(t *testing.T) {
	got := prompt(fixedTime, "Lee")
	want := "[2020-01-20 15:48:41][Lee]:"
	if got != want {
		t.Errorf("prompt() = %q, want %q", got, want)
	}
}

func TestSanitize(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"hello", "hello"},
		{"tab\there", "tab\there"},
		{"\033[2Jclear", "[2Jclear"},
		{"bell\a", "bell"},
		{"del\x7f", "del"},
		{"привет", "привет"},
	}
	for _, tt := range tests {
		if got := sanitize(tt.in); got != tt.want {
			t.Errorf("sanitize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
