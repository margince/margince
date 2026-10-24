// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package correspondence carries the serialization key for "does this
// workspace write to this address", so every module that takes it takes the
// same one.
//
// Whether the workspace corresponds with an address is a T1 fact that calls off
// the noise effects — a verdict may not hide or destroy the mail of somebody we
// write to. The check reads attested OUTBOUND rows, so an attested outbound
// insert is the only write that can turn the answer from no to yes, and the two
// have to serialize: a verdict that reads `no`, then archives while that insert
// commits underneath it, hides the mail of a counterparty the workspace had
// just written to.
//
// The reader is compose's verdict engine, and the writers are capture's sink
// and the activities store. A module never imports a sibling, so without this
// package each would hand-spell the key — and two writers of one lock key that
// disagree by a character take two different locks and serialize nothing, while
// looking exactly like code that does.
//
// Tier 0 rather than storekit, for the reason contactaddress beside it gives:
// storekit owns no domain, and who the workspace corresponds with is a domain
// rule. stdlib only, which the tier requires.
package correspondence

import "strings"

// LockEntity names the lock's subject for storekit.LockWriteIdentity.
const LockEntity = "capture_correspondence"

// LockIdentity is the key itself: the folded address, so a caller cannot take a
// lock that differs from another's by the sender's capitalisation.
func LockIdentity(address string) string {
	return Fold(address)
}

// Fold is how an address is compared, matching activity.counterparty_email and
// contact_email so a lookup is index-backed without a runtime case fold.
func Fold(address string) string {
	return strings.ToLower(strings.TrimSpace(address))
}

// LocalPartBase is a local part without its plus-addressing tag: the mailbox
// owner appends `+tag` to route mail, so `anna+crm` and `anna` are one mailbox.
// A local part with nothing in front of the `+` has no base and is kept whole.
func LocalPartBase(local string) string {
	if base, _, found := strings.Cut(local, "+"); found && base != "" {
		return base
	}
	return local
}

// BaseAddress is the folded address with its local part cut to LocalPartBase,
// and reports whether a tag was removed. The local part ends at the first `@`,
// which is where the duplicate check's SQL expression ends it too.
func BaseAddress(address string) (string, bool) {
	folded := Fold(address)
	local, domain, found := strings.Cut(folded, "@")
	if !found {
		return folded, false
	}
	base := LocalPartBase(local)
	return base + "@" + domain, base != local
}
