// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package worklistsnap holds one reader's walk through their worklist still
// while they page it.
//
// The queue's cursor is an offset into a ranking rebuilt on every read. That is
// honest — the contract said outright that a row crossing the page boundary
// between two reads is served twice or not at all — and it is still a bad walk:
// a rep paging their morning watched the count above the queue move under them,
// met the same customer twice, and had no way to tell which of the two had
// happened.
//
// So the first page freezes the ORDER and the MEMBERSHIP, and later pages
// resume into it.
//
// IDENTITY AND ORDER ONLY. The snapshot stores which rows the walk covers and
// in what sequence. It stores no titles, no subjects, no message excerpts, no
// names, no evidence. Every page re-reads the live rows under the caller's own
// gates and renders only what they may see at that moment; the snapshot decides
// which rows and in what order, never what they say. A snapshot that froze
// display text would be a second copy of records whose visibility can change
// underneath it, and re-serving that copy after a revocation would disclose
// precisely what the revocation withdrew.
//
// MEMBERSHIP MOVES ONE WAY. New work waits for a refresh, so the walk a reader
// started is the walk they finish. Work that was resolved, deleted or is no
// longer visible LEAVES immediately, and the response says how many rows went.
// Freezing the headline over work the reader can no longer see or act on would
// be a steadier number and a false one.
//
// PER READER, and never shared. A walk is one person's position in one
// question, and the fingerprint beside it is that question — scope, filter,
// owner — so a token carried onto a different one is refused rather than
// resumed into an answer nobody asked for.
//
// It owns `worklist_snapshot`, and writes no audit row and no event: this is
// per-reader derived state, the shape org_brief, person_brief and
// deal_status_card already have. An assembly generated FOR one person and never
// served to another has no record history to write, and a trail over it would
// record reading rather than changing.
package worklistsnap
