// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// The ATTACH direction of the row-visibility question.
//
// Reading a shared record and hanging a child row onto it are different acts,
// and record_grant has always carried the two levels that tell them apart. This
// file is the narrower one: the same predicate rowscope.go renders, with the
// share arm admitting a `write` grant alone. linkscope.go's EnsureAttachTarget
// carries the test a call site applies to decide which direction it is in.

import "github.com/margince/margince/backend/internal/shared/kernel/principal"

// shareLevel selects which record_grant rows the share arm admits.
//
// A grant carries `read` or `write`, and the schema has always said write
// satisfies read. READING a shared record takes any live grant — that is what a
// share is for. ATTACHING a child row onto one takes a write grant, because the
// attached row is something a reader of THAT record will now see, and a share
// the sharing screen called read-only must not confer it.
//
// It narrows the share arm and nothing else. The own/team arm is untouched, so
// this is not write authority: filing work against a record another team owns
// stays permitted, which is how this product works on purpose.
// linkscope.go's EnsureAttachTarget carries the test a call site applies.
type shareLevel bool

const (
	anyShare   shareLevel = false
	writeShare shareLevel = true
)

// AttachPredicate is VisiblePredicate with one arm narrowed: a share admits the
// row only at `write`. It is what an ATTACH asks — see EnsureAttachTarget for
// which direction a call site is in — and it differs from the write-authority
// predicate next door in exactly the way the ruling behind it says: the own and
// team arms are untouched, so a rep may still file work onto a record another
// team owns.
func AttachPredicate(p principal.Principal, table string, arg func(any) int) func(alias string) string {
	return predicateFor(p, table, arg, withCapturePrivacy, asClassified, writeShare)
}
