package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRelayHelp(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"token", "--help"}} {
		var out bytes.Buffer
		if err := run(args, &out); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if !strings.Contains(out.String(), "Usage:") {
			t.Fatal(out.String())
		}
	}
}

func TestRelayOptions(t *testing.T) {
	token := strings.Repeat("A", 43)
	file := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(file, []byte(token+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		args    []string
		env     map[string]string
		wantErr string
		public  bool
		listen  string
		max     int
		level   string
	}{
		{name: "missing", wantErr: "access token is required"},
		{name: "file", args: []string{"--access-token-file", file}},
		{name: "env token", env: map[string]string{"ACCESS_TOKEN": token}},
		{name: "env file", env: map[string]string{"ACCESS_TOKEN_FILE": file}},
		{name: "public", args: []string{"--allow-public-access"}, public: true},
		{name: "env public", env: map[string]string{"ALLOW_PUBLIC_ACCESS": "true"}, public: true},
		{name: "explicit false wins", args: []string{"--allow-public-access=false"}, env: map[string]string{"ALLOW_PUBLIC_ACCESS": "true", "ACCESS_TOKEN": token}},
		{name: "empty token", env: map[string]string{"ACCESS_TOKEN": ""}, wantErr: "32-byte"},
		{name: "invalid token", env: map[string]string{"ACCESS_TOKEN": "PRIVATE_SENTINEL"}, wantErr: "32-byte"},
		{name: "empty file", args: []string{"--access-token-file="}, wantErr: "must not be empty"},
		{name: "conflict", args: []string{"--access-token-file", file}, env: map[string]string{"ACCESS_TOKEN": token}, wantErr: "cannot be combined"},
		{name: "public with token", args: []string{"--allow-public-access"}, env: map[string]string{"ACCESS_TOKEN": token}, wantErr: "cannot be combined"},
		{name: "public with empty token", args: []string{"--allow-public-access"}, env: map[string]string{"ACCESS_TOKEN": ""}, wantErr: "cannot be combined"},
		{name: "invalid public", env: map[string]string{"ALLOW_PUBLIC_ACCESS": "1"}, wantErr: "true or false"},
		{name: "env options", env: map[string]string{"ALLOW_PUBLIC_ACCESS": "true", "LISTEN": "[::1]:9090", "MAX_CONNECTIONS": "16", "LOG_LEVEL": "error"}, public: true, listen: "[::1]:9090", max: 16, level: "error"},
		{name: "CLI wins", args: []string{"--allow-public-access", "--listen", ":8091", "--max-connections", "8", "--log-level", "info"}, env: map[string]string{"ALLOW_PUBLIC_ACCESS": "invalid", "LISTEN": "bad", "MAX_CONNECTIONS": "bad", "LOG_LEVEL": "bad"}, public: true, listen: ":8091", max: 8},
		{name: "CLI file wins", args: []string{"--access-token-file", file}, env: map[string]string{"ACCESS_TOKEN_FILE": "/missing"}},
		{name: "invalid max", args: []string{"--allow-public-access", "--max-connections", "0"}, wantErr: "positive integer"},
		{name: "invalid level", args: []string{"--allow-public-access", "--log-level", "debug"}, wantErr: "info or error"},
		{name: "invalid listen", args: []string{"--allow-public-access", "--listen", ":0"}, wantErr: "port"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookup := func(k string) (string, bool) { v, ok := tt.env[strings.TrimPrefix(k, "ZELPORT_RELAY_")]; return v, ok }
			var out bytes.Buffer
			got, done, err := parseOptions(tt.args, &out, lookup)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("want %q, got %v", tt.wantErr, err)
				}
				if strings.Contains(err.Error(), "PRIVATE_SENTINEL") {
					t.Fatal("secret leaked")
				}
				return
			}
			if err != nil || done {
				t.Fatalf("done=%v err=%v", done, err)
			}
			if tt.listen == "" {
				tt.listen = "127.0.0.1:8090"
			}
			if tt.max == 0 {
				tt.max = 128
			}
			if tt.level == "" {
				tt.level = "info"
			}
			if got.listen != tt.listen || got.max != tt.max || got.level != tt.level || got.public != tt.public {
				t.Fatal("unexpected options")
			}
			if !tt.public && len(got.token) != 32 {
				t.Fatal("missing token")
			}
		})
	}
}
