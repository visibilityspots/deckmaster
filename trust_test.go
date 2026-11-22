package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckTrusted(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.deck")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	chmod := func(path string, mode os.FileMode) {
		t.Helper()
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}

	if err := checkTrusted(file); err != nil {
		t.Errorf("own 0644 file in a 0700 directory: %v", err)
	}

	chmod(file, 0o666)
	if err := checkTrusted(file); err == nil || !strings.Contains(err.Error(), "writable by others") {
		t.Errorf("world-writable file: error = %v", err)
	}
	chmod(file, 0o664)
	if err := checkTrusted(file); err == nil {
		t.Error("group-writable file: no error")
	}
	chmod(file, 0o644)

	chmod(dir, 0o777)
	if err := checkTrusted(file); err == nil {
		t.Error("file in a world-writable directory: no error")
	}
	chmod(dir, os.ModeSticky|0o777)
	if err := checkTrusted(file); err != nil {
		t.Errorf("file in a sticky world-writable directory: %v", err)
	}
	chmod(dir, 0o700)

	// root-owned system programs are fine
	if err := checkTrusted("/bin/sh"); err != nil {
		t.Errorf("/bin/sh: %v", err)
	}
	if err := checkTrusted(filepath.Join(dir, "missing")); err == nil {
		t.Error("missing file: no error")
	}
}

func TestCheckCommandTrusted(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "status.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho ok\n"), 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(script, 0o777); err != nil { // umask may have masked it
		t.Fatal(err)
	}

	if err := checkCommandTrusted("playerctl"); err != nil {
		t.Errorf("program from $PATH: %v", err)
	}
	if err := checkCommandTrusted(script); err == nil {
		t.Error("world-writable script: no error")
	}
	if _, err := runCommand(script + " --flag | head -1"); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Errorf("runCommand of a world-writable script: error = %v", err)
	}

	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := runCommand(script + " --flag | head -1"); err != nil || out != "ok" {
		t.Errorf("runCommand of a safe script = %q, %v", out, err)
	}
}

func TestLoadConfigRefusesWritableDeck(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.deck")
	if err := os.WriteFile(path, []byte("[[keys]]\n  index = 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "refusing to load deck") {
		t.Errorf("LoadConfig() of a world-writable deck: error = %v", err)
	}
}

func TestShellProgram(t *testing.T) {
	for in, want := range map[string]string{
		"":                           "",
		"  ~/bin/a.sh --x | head -1": "~/bin/a.sh",
		"echo hi":                    "echo",
	} {
		if got := shellProgram(in); got != want {
			t.Errorf("shellProgram(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCheckTrustedSymlinkIntoUnsafeDir(t *testing.T) {
	safe, unsafe := t.TempDir(), t.TempDir()
	if err := os.Chmod(unsafe, 0o777); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(unsafe, "script.sh")
	if err := os.WriteFile(target, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(safe, "script.sh")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := checkTrusted(link); err == nil || !strings.Contains(err.Error(), "writable by others") {
		t.Errorf("symlink into a world-writable directory: error = %v", err)
	}
}
