package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nix-er/streamdeck"
)

var (
	lockFile = flag.String("lockfile", "",
		"while this file exists, ignore all device input and dim the device, e.g. a file your screen locker creates")
	lockBrightness = flag.Uint("lockbrightness", 10, "brightness in percent while -lockfile exists, at least 1")
)

// screenLock tracks whether the -lockfile exists. The input handlers check it
// right before every action, so a press can't slip through between two polls;
// the event loop polls it to dim and restore the device.
type screenLock struct {
	path string

	// applied is the state the device's brightness was last set for; known is
	// false until it was set once, and again after the device reconnected
	applied, known bool
}

var lock = &screenLock{}

// setLockFile checks and sets the -lockfile path. A relative path is refused:
// systemd doesn't expand ~ and runs deckmaster in another directory than a
// shell, so such a path would silently never exist and never lock the deck.
func (l *screenLock) setLockFile(path string) error {
	if path == "" {
		return nil
	}
	path, err := expandHome(path)
	if err != nil {
		return fmt.Errorf("-lockfile: %w", err)
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("-lockfile must be an absolute path, not %q", path)
	}
	l.path = filepath.Clean(path)
	return nil
}

// locked reports whether the lock file exists. Any error other than "doesn't
// exist", e.g. a permission problem, counts as locked: input is better
// ignored than let through by mistake.
func (l *screenLock) locked() bool {
	if l.path == "" {
		return false
	}
	_, err := os.Stat(l.path)
	return !errors.Is(err, os.ErrNotExist)
}

// brightness returns the brightness the device should have now: -brightness,
// or -lockbrightness while locked. Never 0: the streamdeck library ignores a
// brightness of 0 while the device sleeps, and it would wake at full
// brightness while still locked.
func (l *screenLock) brightness(locked bool) uint8 {
	b := min(*brightness, 100)
	if locked {
		b = max(min(*lockBrightness, b), 1)
	}
	return uint8(b) //nolint:gosec // at most 100
}

// update dims the device when the lock file appeared, and restores the
// brightness when it's gone.
func (l *screenLock) update(dev *streamdeck.Device) {
	if l.path == "" {
		return
	}
	locked := l.locked()
	if l.known && locked == l.applied {
		return
	}

	if l.known || locked {
		if locked {
			fmt.Fprintln(os.Stderr, "Locked: ignoring input until", l.path, "is gone")
		} else {
			fmt.Fprintln(os.Stderr, "Unlocked")
		}
	}
	// recorded even when setting the brightness fails, so a failure isn't
	// retried and logged ten times a second; input is gated regardless
	l.applied, l.known = locked, true
	if err := dev.SetBrightness(l.brightness(locked)); err != nil {
		fmt.Fprintln(os.Stderr, "Can't set the brightness:", err)
	}
}

// reset makes the next update set the brightness and report the state again,
// e.g. after the device reconnected.
func (l *screenLock) reset() {
	l.known = false
}

// ignored reports whether input must be ignored because of the lock, and logs
// it in verbose mode.
func (l *screenLock) ignored(what string) bool {
	if !l.locked() {
		return false
	}
	verbosef("Locked, ignoring %s", what)
	return true
}
