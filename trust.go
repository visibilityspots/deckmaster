package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Deck files are code: their actions and widgets run commands as the user.
// Like OpenSSH's StrictModes, deckmaster refuses deck files and the programs
// they run when someone else could have changed them: when they, or the
// directory they're in, are writable by others, or owned by another user
// than the user running deckmaster or root.

// checkTrusted returns an error if the file at path, or the directory it's
// in, could have been changed by another user.
func checkTrusted(path string) error {
	if err := checkOwnedAndNotWritable(path, false); err != nil {
		return err
	}
	return checkOwnedAndNotWritable(filepath.Dir(path), true)
}

func checkOwnedAndNotWritable(path string, dir bool) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}

	if uid := uint32(os.Getuid()); st.Uid != uid && st.Uid != 0 { //nolint:gosec // uids are never negative
		return fmt.Errorf("%s is owned by uid %d, not by you or root", path, st.Uid)
	}

	mode := fi.Mode()
	if mode.Perm()&0o022 == 0 {
		return nil
	}
	// in a sticky directory like /tmp, others can't replace your files
	if dir && mode&os.ModeSticky != 0 {
		return nil
	}
	return fmt.Errorf("%s is writable by others (mode %04o)", path, mode.Perm())
}

// checkCommandTrusted checks the program a command line starts, if it's
// given as a path. Programs found through $PATH are not checked; neither are
// the arguments, e.g. the script in "setsid -f ~/script.sh".
func checkCommandTrusted(program string) error {
	if !strings.Contains(program, "/") {
		return nil
	}
	path, err := expandPath("", program)
	if err != nil {
		return err
	}
	return checkTrusted(path)
}

// shellProgram returns the first word of a shell command, e.g. the script in
// "~/bin/status.sh --short | head -1".
func shellProgram(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
