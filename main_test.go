package main

import "testing"

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"default port", nil, "8989", false},
		{"custom port", []string{"2525"}, "2525", false},
		{"max port", []string{"65535"}, "65535", false},
		{"too many args", []string{"2525", "localhost"}, "", true},
		{"not a number", []string{"abc"}, "", true},
		{"zero", []string{"0"}, "", true},
		{"out of range", []string{"65536"}, "", true},
		{"negative", []string{"-1"}, "", true},
		{"empty", []string{""}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseArgs(%q) error = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseArgs(%q) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}
