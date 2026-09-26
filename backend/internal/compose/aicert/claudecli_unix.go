// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The build tag is explicit because only GOOS filename suffixes are implicit,
// and "unix" is not one of them.
//go:build !windows

package aicert

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// killProcessGroupOnCancel starts the CLI as the leader of its own process
// group and kills the whole group on cancellation, so a child the CLI spawned
// cannot outlive the call holding its pipes open.
func killProcessGroupOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err == nil || errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return fmt.Errorf("aicert: killing the claude_cli judge's process group: %w", err)
	}
}
