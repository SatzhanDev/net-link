package server

import (
	"strings"
	"testing"
)

func TestWelcomeBanner(t *testing.T) {
	if !strings.HasPrefix(welcomeBanner, "Welcome to TCP-Chat!\n") {
		t.Errorf("banner must start with the welcome line, got %q", welcomeBanner)
	}
	if !strings.Contains(welcomeBanner, "_nnnn_") || !strings.Contains(welcomeBanner, "`-'       `--'") {
		t.Error("banner must contain the linux logo")
	}
}
