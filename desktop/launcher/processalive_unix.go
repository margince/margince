// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !windows

package main

import (
	"errors"
	"syscall"
)

// processAlive reports whether a process with this id exists.
//
// Signal 0 is the POSIX question rather than an action: the kernel runs its
// permission and existence checks and delivers nothing. That is what makes it
// safe to ask about a process this launcher does not own.
//
// EPERM means ALIVE, and getting that backwards is the bug worth naming. It is
// the answer when the process exists and belongs to another user — which is
// exactly the shared-machine case the lock matters most on — so reading it as
// "gone" would let a second user's launcher reclaim a folder the first is
// initialising.
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	return !errors.Is(err, syscall.ESRCH)
}
