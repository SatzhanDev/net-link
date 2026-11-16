package main

import "testing"

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    config
		wantErr bool
	}{
		{"default port", nil, config{mode: serverMode, port: "8989"}, false},
		{"custom port", []string{"2525"}, config{mode: serverMode, port: "2525"}, false},
		{"max port", []string{"65535"}, config{mode: serverMode, port: "65535"}, false},
		{"client mode", []string{"-c", "localhost", "2525"}, config{mode: clientMode, host: "localhost", port: "2525"}, false},
		{"client with ip", []string{"-c", "127.0.0.1", "8989"}, config{mode: clientMode, host: "127.0.0.1", port: "8989"}, false},

		{"port and host (subject example)", []string{"2525", "localhost"}, config{}, true},
		{"not a number", []string{"abc"}, config{}, true},
		{"zero", []string{"0"}, config{}, true},
		{"out of range", []string{"65536"}, config{}, true},
		{"negative", []string{"-1"}, config{}, true},
		{"empty", []string{""}, config{}, true},
		{"client without port", []string{"-c", "localhost"}, config{}, true},
		{"client with bad port", []string{"-c", "localhost", "abc"}, config{}, true},
		{"client with empty host", []string{"-c", "", "2525"}, config{}, true},
		{"unknown flag", []string{"-x", "localhost", "2525"}, config{}, true},
		{"too many args", []string{"-c", "localhost", "2525", "extra"}, config{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseArgs(%q) error = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseArgs(%q) = %+v, want %+v", tt.args, got, tt.want)
			}
		})
	}
}
