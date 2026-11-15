package server

import (
	"errors"
	"strings"
)

const maxNameLen = 32

// Name validation errors. They are shown to the user verbatim.
var (
	ErrNameEmpty    = errors.New("name cannot be empty")
	ErrNameTooLong  = errors.New("name is too long (max 32 characters)")
	ErrNameInvalid  = errors.New("name cannot contain '[' or ']'")
	ErrNameReserved = errors.New("this name is reserved")
	ErrNameTaken    = errors.New("this name is already taken")
)

// validateName checks the format of a name. Uniqueness is checked by the
// server under its lock (see join), because it depends on who is connected.
// Assumptions: names are trimmed, limited in length, may not contain square
// brackets (they would break the [time][name] format and allow spoofing) and
// may not impersonate the System user.
func validateName(name string) error {
	switch {
	case name == "":
		return ErrNameEmpty
	case len([]rune(name)) > maxNameLen:
		return ErrNameTooLong
	case strings.ContainsAny(name, "[]"):
		return ErrNameInvalid
	case strings.EqualFold(name, systemName):
		return ErrNameReserved
	}
	return nil
}
