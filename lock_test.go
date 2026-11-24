package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScreenLockLocked(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "locked")

	if (&screenLock{}).locked() {
		t.Error("without -lockfile the deck is locked")
	}

	l := &screenLock{path: path}
	if l.locked() {
		t.Error("locked while the lock file doesn't exist")
	}
	if l.ignored("key 0") {
		t.Error("input ignored while the lock file doesn't exist")
	}

	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if !l.locked() {
		t.Error("not locked while the lock file exists")
	}
	if !l.ignored("key 0") {
		t.Error("input let through while the lock file exists")
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if l.locked() {
		t.Error("still locked after the lock file is gone")
	}
}

// An error other than "doesn't exist" must not unlock the deck.
func TestScreenLockStatErrorIsLocked(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can stat anything")
	}
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "locked"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sub, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sub, 0o700) })

	l := &screenLock{path: filepath.Join(sub, "locked")}
	if !l.locked() {
		t.Error("a lock file that can't be checked counts as unlocked")
	}
}

func TestScreenLockSetLockFile(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		in, want string
		fails    bool
	}{
		{in: "", want: ""},
		{in: "/tmp/locked", want: "/tmp/locked"},
		{in: "/tmp//x/../locked", want: "/tmp/locked"},
		{in: "~/locked", want: filepath.Join(home, "locked")},
		{in: "locked", fails: true},
		{in: "./locked", fails: true},
		{in: "~other/locked", fails: true},
	} {
		l := &screenLock{}
		err := l.setLockFile(tc.in)
		if tc.fails {
			if err == nil {
				t.Errorf("%q: accepted as %q", tc.in, l.path)
			}
			continue
		}
		if err != nil || l.path != tc.want {
			t.Errorf("%q: got %q, %v; want %q", tc.in, l.path, err, tc.want)
		}
	}
}

func TestScreenLockBrightness(t *testing.T) {
	defer func(b, lb uint) { *brightness, *lockBrightness = b, lb }(*brightness, *lockBrightness)

	for _, tc := range []struct {
		normal, lock     uint
		unlocked, locked uint8
	}{
		{80, 10, 80, 10},
		{5, 10, 5, 5},  // never brighter than unlocked
		{80, 0, 80, 1}, // never 0
		{150, 200, 100, 100},
	} {
		*brightness, *lockBrightness = tc.normal, tc.lock
		l := &screenLock{}
		if got := l.brightness(false); got != tc.unlocked {
			t.Errorf("%d/%d unlocked: got %d, want %d", tc.normal, tc.lock, got, tc.unlocked)
		}
		if got := l.brightness(true); got != tc.locked {
			t.Errorf("%d/%d locked: got %d, want %d", tc.normal, tc.lock, got, tc.locked)
		}
	}
}

// A press that began while locked must not trigger when it's released after
// unlocking; one that began unlocked still does.
func TestPressTrackerPressBegunWhileLocked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "locked")
	defer func(old *screenLock) { lock = old }(lock)
	lock = &screenLock{path: path}

	var triggered []uint8
	p := newPressTracker("key", make(chan func(), 8), func(index uint8, _ bool) {
		triggered = append(triggered, index)
	})

	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	p.handle(1, true)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	p.handle(1, false)

	p.handle(2, true)
	p.handle(2, false)

	if len(triggered) != 1 || triggered[0] != 2 {
		t.Errorf("triggered %v, want only key 2", triggered)
	}
}
