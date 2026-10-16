package main

import (
	"bytes"
	"github.com/zelport/zelport-sdk-go/protocol"
	"os"
	"path/filepath"
	"testing"
)

func TestTokenExclusiveAndPrivate(t *testing.T) {
	old := effectiveUID
	effectiveUID = func() int { return 501 }
	defer func() { effectiveUID = old }()
	path := filepath.Join(t.TempDir(), "token")
	var output bytes.Buffer
	if e := run([]string{"token", "--out", path}, &output); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(path)
	if _, e := protocol.DecodeKey(string(bytes.TrimSpace(b))); e != nil {
		t.Fatal(e)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	if e := run([]string{"token", "--out", path}, &output); e == nil {
		t.Fatal("overwrote token")
	}
	if output.Len() != 0 {
		t.Fatal("token printed")
	}
}
func TestRequiredToken(t *testing.T) {
	if e := run(nil, &bytes.Buffer{}); e == nil {
		t.Fatal("missing access token accepted")
	}
}
