package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckDecks(t *testing.T) {
	loadTestFonts(t)
	dir := t.TempDir()
	write := func(name, content string, mode os.FileMode) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}

	write("evil.sh", "#!/bin/sh\n", 0o777)
	write("main.deck", `
[[keys]]
  index = 0
  [keys.widget]
    id = "button"
  [keys.action]
    deck = "sub.deck"
[[keys]]
  index = 1
  [keys.widget]
    id = "button"
  [keys.action]
    deck = "missing.deck"
[[keys]]
  index = 2
  [keys.widget]
    id = "button"
  [keys.action]
    exec = "`+filepath.Join(dir, "evil.sh")+` --now"
[swipe]
  [swipe.left]
    deck = "dial.deck"
`, 0o644)
	write("sub.deck", `
[[keys]]
  index = 0
  [keys.widget]
    id = "nosuchwidget"
  [keys.action]
    deck = "main.deck"
  [keys.acton]
    exec = "true"
`, 0o644)
	write("dial.deck", `
[[dials]]
  index = 0
  [dials.action_right]
    deck = "main.deck"
`, 0o644)

	var out bytes.Buffer
	n := checkDecks(plusDevice(), filepath.Join(dir, "main.deck"), &out)
	got := out.String()

	for _, want := range []string{
		"main.deck", "sub.deck", "dial.deck", // all reachable decks get checked
		"missing.deck: no such file",
		"evil.sh is writable by others",
		"unknown widget with ID nosuchwidget",
		"unknown keys: keys.acton",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
	// main.deck isn't checked twice even though two decks lead back to it
	if c := strings.Count(got, filepath.Join(dir, "main.deck")+"\n"); c != 1 {
		t.Errorf("main.deck checked %d times, want once:\n%s", c, got)
	}
	if n != 4 {
		t.Errorf("checkDecks() = %d problems, want 4:\n%s", n, got)
	}
}
