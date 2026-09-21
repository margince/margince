// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import "syscall"

// processAlive reports whether a process with this id exists.
//
// Windows has no signal 0, and os.FindProcess succeeds for any id there, so it
// answers nothing. Opening a handle is the question: OpenProcess fails when no
// process carries the id.
//
// PROCESS_QUERY_LIMITED_INFORMATION rather than PROCESS_QUERY_INFORMATION,
// because the shared-machine case is the one the lock is for: the wider right
// is refused across a user boundary, and a refusal read as "gone" would let a
// second user's launcher reclaim a folder the first is initialising. The
// limited right answers existence across that boundary, which is all this asks.
//
// A handle that opens is CLOSED immediately — leaking one would hold the
// process's exit code alive in the kernel for as long as the launcher runs.
//
// A still-running process and one that has exited but whose id nothing has
// reused are told apart by the exit code: STILL_ACTIVE means running. That
// distinction matters because a handle can outlive its process here, unlike a
// pid on unix.
func processAlive(pid int) bool {
	const (
		processQueryLimitedInformation = 0x1000
		// STILL_ACTIVE, spelled here because the standard library's windows
		// syscall package does not export it. It is 259 (STATUS_PENDING) and
		// has been since NT — the value a process's exit code carries while it
		// is running.
		stillActive = 259
	)
	handle, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() {
		//craft:ignore swallowed-errors closing a query handle cannot change the answer already read, and reporting it would replace a liveness answer with a housekeeping one
		_ = syscall.CloseHandle(handle)
	}()
	var code uint32
	if err := syscall.GetExitCodeProcess(handle, &code); err != nil {
		// The handle opened, so something carries this id. Reporting it alive
		// is the safe direction: the cost is a readable refusal, and the cost
		// of the other answer is a corrupted cluster.
		return true
	}
	return code == stillActive
}
