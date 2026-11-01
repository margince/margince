// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

import "github.com/margince/margince/backend/internal/shared/kernel/ids"

// Target is the record a notice is about, when it is about one.
//
// Both halves or neither, which the table's own constraint holds: a type with
// no id names a KIND of thing and cannot be opened, and an id with no type
// cannot be routed to a screen. The zero value is a notice about no record —
// a capture backlog, a coach's word — and those are the majority.
type Target struct {
	Type string
	ID   ids.UUID
}

// Named reports whether this notice points at a record.
func (t Target) Named() bool { return t.Type != "" && !t.ID.IsZero() }
