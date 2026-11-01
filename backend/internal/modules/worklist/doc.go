// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package worklist keeps each member's pushed work items: which record asks
// for work, whose list it is on, and when it is due. It stores no display text.
//
// PROOF OF CONCEPT. The table is created by the PoC test, not by a migration,
// and no route reads it yet. A producer writes the items of one kind for one
// holder in the background; a read takes the holder's open items and keeps
// only those whose record the reader may read now, so a change of rights takes
// effect at the next read without any event.
package worklist
