package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootPolicy(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		uid                   int
		allow, denied, warned bool
	}{
		{"root rejected", 0, false, true, false}, {"root allowed", 0, true, false, true},
		{"regular user", 501, false, false, false}, {"unnecessary override", 501, true, false, false},
		{"unsupported UID", -1, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := checkRoot(tc.uid, tc.allow, &out)
			if (err != nil) != tc.denied {
				t.Fatalf("error: %v", err)
			}
			if err != nil && (!strings.Contains(err.Error(), "--allow-root") || !strings.Contains(err.Error(), allowRootEnv+"=1")) {
				t.Fatal(err)
			}
			if strings.Contains(out.String(), "WARNING:") != tc.warned {
				t.Fatalf("warning: %q", out.String())
			}
		})
	}
}

func TestRootOptions(t *testing.T) {
	for _, tc := range []struct {
		env  string
		args []string
		want bool
	}{
		{"", nil, false}, {"1", nil, true}, {"true", nil, false}, {" 1", nil, false},
		{"", []string{"--allow-root"}, true}, {"1", []string{"--allow-root=false"}, false},
	} {
		lookup := func(k string) (string, bool) {
			if k == allowRootEnv {
				return tc.env, true
			}
			return "", false
		}
		cfg, done, err := parseOptions(append([]string{"--allow-public-access"}, tc.args...), io.Discard, lookup)
		if err != nil || done || cfg.allowRoot != tc.want {
			t.Fatalf("%+v: cfg=%+v done=%v err=%v", tc, cfg, done, err)
		}
	}
}
func TestRootRunGuard(t *testing.T) {
	old := effectiveUID
	effectiveUID = func() int { return 0 }
	defer func() { effectiveUID = old }()
	t.Setenv(allowRootEnv, "")
	for _, args := range [][]string{{"--help"}, {"--version"}, {"token", "--help"}} {
		var out bytes.Buffer
		if err := run(args, &out); err != nil {
			t.Fatal(err)
		}
		if args[len(args)-1] == "--help" && (!strings.Contains(out.String(), "--allow-root") || !strings.Contains(out.String(), allowRootEnv)) {
			t.Fatal(out.String())
		}
	}
	if err := run([]string{"--allow-public-access", "--log-level=error"}, io.Discard); err == nil || !strings.Contains(err.Error(), "Refusing to run as root for security reasons.") {
		t.Fatalf("service root check: %v", err)
	}
	path := filepath.Join(t.TempDir(), "token")
	if err := run([]string{"token", "--out", path}, io.Discard); err == nil || !strings.Contains(err.Error(), "Refusing to run as root for security reasons.") {
		t.Fatalf("error: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("token created before root check: %v", err)
	}
	if err := run([]string{"token", "--out", path, "--allow-root"}, io.Discard); err != nil {
		t.Fatal(err)
	}
}
