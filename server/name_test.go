package server

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	tests := []struct {
		name string
		want error
	}{
		{"Yenlik", nil},
		{"Ли", nil},
		{"", ErrNameEmpty},
		{strings.Repeat("a", maxNameLen), nil},
		{strings.Repeat("a", maxNameLen+1), ErrNameTooLong},
		{"[admin]", ErrNameInvalid},
		{"System", ErrNameReserved},
		{"system", ErrNameReserved},
	}
	for _, tt := range tests {
		if got := validateName(tt.name); !errors.Is(got, tt.want) {
			t.Errorf("validateName(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestParseNameCommand(t *testing.T) {
	tests := []struct {
		text     string
		wantName string
		wantOK   bool
	}{
		{"/name Lee", "Lee", true},
		{"  /name   Lee  ", "Lee", true},
		{"/name", "", true},
		{"/name   ", "", true},
		{"/name Lee Brown", "Lee Brown", true},
		{"/names", "", false},
		{"/nameLee", "", false},
		{"hello /name Lee", "", false},
		{"hello", "", false},
	}
	for _, tt := range tests {
		name, ok := parseNameCommand(tt.text)
		if name != tt.wantName || ok != tt.wantOK {
			t.Errorf("parseNameCommand(%q) = (%q, %v), want (%q, %v)", tt.text, name, ok, tt.wantName, tt.wantOK)
		}
	}
}
