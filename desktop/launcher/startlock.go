// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// One launcher per installation folder.
//
// Two launchers started at once on a FRESH installation both decide the
// database needs creating, and then both create it in the same staging
// directory: one process's RemoveAll deletes what the other is midway through
// writing, and the rename that follows can land a half-built cluster where the
// finished one belongs. Double-clicking the start script twice is enough.
//
// It is first-boot only — on a later launch the postmaster's own lock file and
// the fixed UI port refuse the second process, for reasons a user can read — so
// what is missing is not a general mutex but an answer to "is another launcher
// working on this folder", asked before anything is created.
//
// HELD FOR THE WHOLE RUN rather than around initdb alone. A second launcher
// that waited for the cluster and then bound ports and ran migrations against
// it is the same collision arriving later, so the lock is taken before the
// stack starts and released after it stops.
//
// WHY A PID FILE AND NOT flock. `flock` is the better primitive and this module
// cannot reach it: it is deliberately stdlib-only and outside the workspace,
// and Windows's LockFileEx lives in golang.org/x/sys. A PID file with a
// liveness check is what both platforms can answer from the standard library,
// and its weakness is stated where it bites — see staleLock.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// startLockName sits in data/ beside the other per-installation state rather
// than in the installation root, so a user browsing the folder they installed
// into sees the program and their own settings, not a lock file.
const startLockName = "starting.lock"

func (l layout) startLockPath() string { return filepath.Join(l.data(), startLockName) }

// startLock is a held installation lock. Release is the only way it ends.
type startLock struct{ path string }

// lockInstallation claims this folder for this launcher.
//
// THE CREATE IS THE CHECK, the shape the rest of this launcher uses for a
// single-writer question: O_EXCL answers "does it exist" and "claim it" in one
// syscall, so two launchers racing cannot both be told the folder is free.
func lockInstallation(l layout) (*startLock, error) {
	if err := os.MkdirAll(l.data(), 0o700); err != nil {
		return nil, fmt.Errorf("create the data directory: %w", err)
	}
	path := l.startLockPath()
	if err := writeStartLock(path); err == nil {
		return &startLock{path: path}, nil
	} else if !errors.Is(err, os.ErrExist) {
		return nil, err
	}

	// It exists. Either another launcher is running, or one died holding it.
	stale, holder := staleLock(path)
	if !stale {
		// The capital is the product name, and this error is printed verbatim to
		// the contact who just double-clicked the second icon.
		return nil, fmt.Errorf( //nolint:staticcheck // ST1005: leading word is a proper noun
			"Margince is already starting in this folder (process %s) — wait for it to finish, "+
				"or close that one and start again", holder)
	}
	// The holder is gone. Removing and re-claiming is safe because the lock
	// grants nothing on its own: whatever that run left half-done is cleaned by
	// the step that owns it — initCluster clears its own staging directory, and
	// nothing was ever moved into place from one.
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("clear the abandoned start lock %s: %w", path, err)
	}
	// Once, not in a loop. A second failure here means a launcher claimed the
	// folder between the remove and this write, which is the answer this
	// function exists to give.
	if err := writeStartLock(path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, errors.New("another Margince launcher claimed this folder while this one was starting")
		}
		return nil, err
	}
	return &startLock{path: path}, nil
}

// release drops the claim. A launcher that exits without reaching this leaves a
// lock the next start reads as stale, which is why the staleness check is not
// optional.
func (s *startLock) release() error {
	if s == nil {
		return nil
	}
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("release the start lock %s: %w", s.path, err)
	}
	return nil
}

// writeStartLock records this process's id, and refuses an existing file.
func writeStartLock(path string) error {
	// #nosec G304 -- path is layout.data()'s lock file, derived from the installation directory and never from input
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|openNoFollow, 0o600)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(file, "%d\n", os.Getpid()); err != nil {
		if closeErr := file.Close(); closeErr != nil {
			return fmt.Errorf("write %s: %w (and closing it failed: %v)", path, err, closeErr)
		}
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}

// staleLock reports whether the recorded holder is gone, and names it either
// way so the refusal can say who has the folder.
//
// WHAT THIS CANNOT ANSWER, said here because it decides what a user sees. A
// process id is reused, so a lock left by a crashed launcher whose id has since
// been taken by something unrelated reads as live. The installation then
// refuses to start with a message naming a process that is not Margince — which
// is why that message says what to do about it rather than only what is wrong.
// The alternative, treating an unreadable or doubtful lock as stale, trades a
// readable refusal for the corrupted cluster this whole file exists to prevent.
//
// An unreadable or malformed lock is treated as HELD for the same reason: it
// was written by something, and guessing otherwise is the guess that costs a
// database.
func staleLock(path string) (stale bool, holder string) {
	// #nosec G304 -- see writeStartLock
	recorded, err := os.ReadFile(path)
	if err != nil {
		return false, "unknown"
	}
	text := strings.TrimSpace(string(recorded))
	pid, err := strconv.Atoi(text)
	if err != nil || pid <= 0 {
		return false, "unknown"
	}
	if pid == os.Getpid() {
		// This process wrote it and did not release it — a crash inside one
		// run, or a lock file left in a copied installation folder. Reclaiming
		// is correct: there is no other launcher to collide with.
		return true, text
	}
	return !processAlive(pid), text
}
