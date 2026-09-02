// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import "github.com/margince/margince/backend/internal/platform/mailrole"

// refusesToNameAPerson answers the question T1's evidence cannot: this address
// is one the workspace demonstrably writes to, but is there a PERSON to record?
//
// A role mailbox is correspondence-positive exactly like a customer — a mailbox
// owner writes to `billing@` and `support@` all the time — so T1's evidence is
// true and its conclusion was still wrong. Creating a contact for one put
// records called "Billing" and "support" in a founder's CRM, each with a
// human's shape and nobody behind it.
//
// It is deliberately NOT part of recordWorthy, which asks whether a person can
// be REACHED at the address. Somebody does answer `info@`, and that function's
// own tests say so. Reachability and identity are two questions, and the second
// is this one.
//
// REPEATED correspondence outranks it, exactly as T1 outranks the T2 registry
// and a stale terminal answer, and for the same reason: a shared mailbox the
// workspace keeps writing to is a working relationship, and `sales@` at a
// supplier answered twice is somebody worth holding. One send to `billing@` is
// an errand.
//
// The caller refuses only the CREATE on a true answer. The address still
// travels the ladder's ordinary path, so a first sighting defers and opens the
// ledger question the verdict answers as `role_mailbox` — returning early would
// skip that write and leave the queue unjudged, re-asked on every later message.
func refusesToNameAPerson(email string, correspondedRepeatedly bool) bool {
	if correspondedRepeatedly {
		return false
	}
	_, role := mailrole.Match(email)
	return role
}
