// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

import "github.com/margince/margince/backend/internal/shared/kernel/ids"

// Target is the record a notice is about, when it is about one.
//
// Both halves or neither, as the table's own constraint holds. A type with no
// id names a kind of thing and cannot be opened. An id with no type cannot be
// routed to a screen. The zero value is a notice about no record, such as a
// capture backlog or a coach's word, and most notices are.
type Target struct {
	Type string
	ID   ids.UUID
}

// Named reports whether this notice points at a record.
func (t Target) Named() bool { return t.Type != "" && !t.ID.IsZero() }
