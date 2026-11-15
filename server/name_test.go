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
