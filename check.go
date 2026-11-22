package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/nix-er/streamdeck"
)

// checkDevice returns the device decks are checked against: the configured
// (or first) connected device, without opening it, or else a Stream Deck+,
// which has keys, dials and a touch screen.
func checkDevice(w io.Writer) *streamdeck.Device {
	if devs, err := streamdeck.Devices(); err == nil {
		for i := range devs {
			if *device == "" || devs[i].Serial == *device {
				fmt.Fprintf(w, "checking against device %s (%d keys, %d dials)\n",
					devs[i].Serial, devs[i].Keys, devs[i].Dials)
				return &devs[i]
			}
		}
	}

	dev, ok := checkLayouts[*layout]
	if !ok {
		fmt.Fprintf(w, "unknown layout %q, using plus\n", *layout)
		*layout, dev = "plus", checkLayouts["plus"]
	}
	fmt.Fprintf(w, "no matching device connected, checking against the %s layout\n", *layout)
	return &dev
}

// checkLayouts are the device layouts -check can use when the device isn't
// connected, chosen with -layout.
var checkLayouts = map[string]streamdeck.Device{
	"mini": {
		Columns: 3, Rows: 2, Keys: 6, KeyWidth: 80, KeyHeight: 80, Pixels: 80, DPI: 138, Padding: 16,
	},
	"original": {
		Columns: 5, Rows: 3, Keys: 15, KeyWidth: 72, KeyHeight: 72, Pixels: 72, DPI: 124, Padding: 16,
	},
	"xl": {
		Columns: 8, Rows: 4, Keys: 32, KeyWidth: 96, KeyHeight: 96, Pixels: 96, DPI: 166, Padding: 32,
	},
	"plus": {
		Columns: 4, Rows: 2, Keys: 8, KeyWidth: 120, KeyHeight: 120, Pixels: 120, DPI: 166, Padding: 16,
		Dials: 4, ScreenWidth: 800, ScreenHeight: 100, ScreenSegments: 4, ScreenDPI: 166,
	},
}

// checkDecks loads the given deck and every deck reachable from it through
// deck actions, without touching the device, and reports all problems:
// syntax and type errors, unknown keys, unknown widgets, keys and dials the
// device doesn't have, icons that don't load, missing files, and deck files,
// exec programs and widget commands others can change. It returns the number
// of problems found.
func checkDecks(dev *streamdeck.Device, start string, w io.Writer) int {
	problems := 0
	report := func(format string, a ...interface{}) {
		problems++
		fmt.Fprintf(w, "  error: "+format+"\n", a...)
	}

	old := unknownKeys
	reportUnknown := func(filename string, keys []string) {
		report("%s: unknown keys: %s", filename, strings.Join(keys, ", "))
	}
	defer func() { unknownKeys = old }()

	first, err := expandPath(".", start)
	if err != nil {
		report("%s: %s", start, err)
		return problems
	}
	queue := []string{first}
	seen := map[string]bool{first: true}

	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]

		fmt.Fprintln(w, path)
		before := problems
		// the config alone, to report its unknown keys once and to follow its
		// links even if the deck doesn't load on the device
		unknownKeys = reportUnknown
		dc, err := LoadConfig(path)
		if err != nil {
			report("%s", err)
			continue
		}
		unknownKeys = func(string, []string) {}
		if _, err := LoadDeck(dev, ".", path); err != nil {
			report("%s", err)
		}

		checkLayout(dev, dc, filepath.Dir(path), report)

		for _, a := range dc.actions() {
			if a.Exec != "" {
				if args, err := splitCommand(a.Exec); err != nil {
					report("exec %q: %s", a.Exec, err)
				} else if len(args) > 0 {
					if err := checkCommandTrusted(args[0]); err != nil {
						report("exec %q: %s", a.Exec, err)
					}
				}
			}
			if a.Deck == "" {
				continue
			}
			next, err := expandPath(filepath.Dir(path), a.Deck)
			if err != nil {
				report("deck %s: %s", a.Deck, err)
				continue
			}
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}

		if problems == before {
			fmt.Fprintln(w, "  ok")
		}
	}

	return problems
}

// actions returns all actions configured in a deck: those of its keys,
// dials and swipes.
func (c DeckConfig) actions() []*ActionConfig {
	var all actionList
	for _, k := range c.Keys {
		all.add(k.Action, k.ActionHold)
	}
	for _, dc := range c.Dials {
		all.addDial(dc)
	}
	all.add(c.Swipe.Left, c.Swipe.Right)
	return all
}

type actionList []*ActionConfig

func (l *actionList) add(aa ...*ActionConfig) {
	for _, a := range aa {
		if a != nil {
			*l = append(*l, a)
		}
	}
}

func (l *actionList) addDial(dc DialConfig) {
	l.add(dc.Action, dc.ActionHold, dc.ActionLeft, dc.ActionRight, dc.ActionTouch, dc.ActionTouchHold)
}

// checkLayout reports keys and dials the device doesn't have, icons that
// don't load, and widget commands that won't run.
func checkLayout(dev *streamdeck.Device, dc DeckConfig, base string, report func(string, ...interface{})) {
	for _, k := range dc.Keys {
		where := fmt.Sprintf("key %d", k.Index)
		if k.Index >= dev.Keys+dev.TouchKeys {
			report("%s: the device has %d keys (0-%d)", where, dev.Keys+dev.TouchKeys, dev.Keys+dev.TouchKeys-1)
		}
		checkWidgetConfig(where, k.Widget, base, report)
	}
	for _, d := range dc.Dials {
		where := fmt.Sprintf("dial %d", d.Index)
		if d.Index >= dev.Dials {
			report("%s: the device has %d dials", where, dev.Dials)
		}
		checkWidgetConfig(where, d.Widget, base, report)
		if d.RingValue != "" {
			checkWidgetCommand(where+" ring_value", d.RingValue, report)
		}
	}
}

// checkWidgetConfig reports a widget's icon that doesn't load and commands
// that won't run.
func checkWidgetConfig(where string, wc WidgetConfig, base string, report func(string, ...interface{})) {
	var icon string
	if err := ConfigValue(wc.Config["icon"], &icon); err == nil && icon != "" {
		var path string
		var err error
		if isIconName(icon) {
			path, err = findIcon(icon)
		} else {
			path, err = expandPath(base, icon)
		}
		if err == nil {
			_, err = loadImage(path)
		}
		if err != nil {
			report("%s: icon %s: %s", where, icon, err)
		}
	}

	switch wc.ID {
	case "command":
		var commands []string
		if err := ConfigValue(wc.Config["command"], &commands); err == nil {
			for _, c := range commands {
				checkWidgetCommand(where, c, report)
			}
		}
	case "imageCommand":
		var command string
		if err := ConfigValue(wc.Config["command"], &command); err == nil {
			checkWidgetCommand(where, command, report)
		}
	}
}

// checkWidgetCommand reports a widget command whose program, given as a
// path, is missing or others can change it.
func checkWidgetCommand(where, command string, report func(string, ...interface{})) {
	if err := checkCommandTrusted(shellProgram(command)); err != nil {
		report("%s: command %q: %s", where, command, err)
	}
}
