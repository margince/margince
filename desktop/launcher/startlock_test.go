// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// The single-writer question this launcher could not answer: is another one
// already working on this folder?
//
// Without it two launchers on a fresh installation both decide the database
// needs creating and both build it in the same staging directory — one's
// RemoveAll deleting what the other is writing, and the rename that follows
// landing a half-built cluster where the finished one belongs.

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

// A launcher holding the folder refuses the next one, and says what to do
// rather than letting it fail later on a symptom — an initdb error, or a
// cluster that will not start.
//
// The holder is the PARENT process rather than this one: a real second
// launcher is a different process, and lockInstallation reclaims its own id on
// purpose (a crash inside one run, or a lock copied with the folder). Writing
// this process's id would exercise that branch instead of the one under test.
func TestASecondLauncherIsRefusedWhileAnotherHoldsTheFolder(t *testing.T) {
	l := newTestLayout(t)
	holder := os.Getppid()
	if !processAlive(holder) {
		t.Skip("the parent process is gone, so it cannot stand in for a live launcher")
	}
	if err := os.WriteFile(l.startLockPath(), []byte(strconv.Itoa(holder)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := lockInstallation(l)
	if err == nil {
		t.Fatal("a second launcher claimed a folder another is starting in; both would create the " +
			"database in the same staging directory")
	}
	if !strings.Contains(err.Error(), "already starting") {
		t.Errorf("the refusal reads %q, want it to say the folder is already starting", err)
	}
	if !strings.Contains(err.Error(), strconv.Itoa(holder)) {
		t.Errorf("the refusal reads %q, want it to name the holder — a user with two windows open "+
			"needs to know which one to close", err)
	}
}

// Claiming and releasing leaves the folder free. A lock nothing can drop would
// brick the installation on its second run.
func TestClaimingAndReleasingFreesTheFolder(t *testing.T) {
	l := newTestLayout(t)

	held, err := lockInstallation(l)
	if err != nil {
		t.Fatalf("the first launcher could not claim the folder: %v", err)
	}
	if _, err := os.Stat(l.startLockPath()); err != nil {
		t.Fatalf("the claim wrote no lock: %v", err)
	}
	if err := held.release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, err := os.Stat(l.startLockPath()); !os.IsNotExist(err) {
		t.Fatalf("the lock survived release: %v", err)
	}
	next, err := lockInstallation(l)
	if err != nil {
		t.Fatalf("the folder stayed locked after release: %v", err)
	}
	if err := next.release(); err != nil {
		t.Fatal(err)
	}
}

// A launcher killed mid-start leaves its lock behind. The next one must reclaim
// it: a lock only a graceful exit can drop turns one crash into an
// installation nobody can start.
func TestAnAbandonedLockIsReclaimed(t *testing.T) {
	l := newTestLayout(t)

	// A pid that is certainly not running. 0 and negatives are refused by the
	// parse; this is a live-looking id with nothing behind it.
	if err := os.WriteFile(l.startLockPath(), []byte(strconv.Itoa(deadPID(t))+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lock, err := lockInstallation(l)
	if err != nil {
		t.Fatalf("a lock left by a dead launcher was not reclaimed: %v — one crash would brick the "+
			"installation", err)
	}
	// The file is this process's now, not the corpse's.
	recorded, err := os.ReadFile(l.startLockPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(recorded)) != strconv.Itoa(os.Getpid()) {
		t.Errorf("the reclaimed lock records %q, want this process", strings.TrimSpace(string(recorded)))
	}
	if err := lock.release(); err != nil {
		t.Fatal(err)
	}
}

// A lock whose contents cannot be read as a process id is treated as HELD.
//
// The safe direction is the one that refuses: the cost of refusing a folder
// nobody holds is a message a user can act on, and the cost of the other answer
// is the corrupted cluster this lock exists to prevent.
func TestAnUnreadableLockIsTreatedAsHeld(t *testing.T) {
	l := newTestLayout(t)
	for _, contents := range []string{"", "   ", "not a pid", "-1", "0"} {
		if err := os.WriteFile(l.startLockPath(), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := lockInstallation(l); err == nil {
			t.Errorf("a lock containing %q was reclaimed; an unreadable lock was written by "+
				"something, and guessing otherwise is the guess that costs a database", contents)
		}
	}
}

// This process's own id in the lock means a crash inside one run, or a lock
// copied along with an installation folder. There is no other launcher to
// collide with, so reclaiming is correct.
func TestALockThisProcessLeftIsReclaimed(t *testing.T) {
	l := newTestLayout(t)
	if err := os.WriteFile(l.startLockPath(), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	lock, err := lockInstallation(l)
	if err != nil {
		t.Fatalf("a lock this process itself left was not reclaimed: %v", err)
	}
	if err := lock.release(); err != nil {
		t.Fatal(err)
	}
}

// Releasing a lock somebody already removed is not an error: the run is over
// either way, and reporting it would replace the outcome of the launch with a
// housekeeping complaint.
func TestReleasingAnAlreadyRemovedLockIsQuiet(t *testing.T) {
	l := newTestLayout(t)
	lock, err := lockInstallation(l)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(l.startLockPath()); err != nil {
		t.Fatal(err)
	}
	if err := lock.release(); err != nil {
		t.Errorf("release complained about a lock that was already gone: %v", err)
	}
}

// This process is alive and pid 1 exists on every unix; the liveness check has
// to agree with both, or every lock reads stale and the whole file is inert.
func TestProcessAliveAnswersForARunningProcess(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Error("this process reads as gone, so every lock would be reclaimed and nothing serialised")
	}
}

// deadPID finds an id nothing is using, so the stale-lock case is about a dead
// holder rather than about a parse failure.
func deadPID(t *testing.T) int {
	t.Helper()
	// Downward from a high id: a freshly booted machine has few processes, and
	// the top of the range is where the gaps are.
	for pid := 4_000_000; pid > 100_000; pid -= 7919 {
		if !processAlive(pid) {
			return pid
		}
	}
	t.Skip("no unused process id could be found on this machine")
	return 0
}
