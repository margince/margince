// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import "os/exec"

// killProcessGroupOnCancel leaves exec's default kill of the CLI alone: Windows
// has no process group to signal, and WaitDelay still bounds the wait.
func killProcessGroupOnCancel(*exec.Cmd) {}
