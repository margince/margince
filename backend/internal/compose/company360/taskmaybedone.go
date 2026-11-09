// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

import "github.com/margince/margince/backend/internal/shared/kernel/ids"

// taskSource is where a task row came from: the message its promise was read
// out of, and the system that wrote it.
type taskSource struct {
	activity *ids.UUID
	system   string
}
