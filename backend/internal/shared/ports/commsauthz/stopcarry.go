// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

// The seam a merge crosses to keep a subject's stops.
//
// contacts owns the merge and lead promotion; consent owns
// communication_suppression. Neither may import the other, and the types they
// have to agree on are two: which subject is retiring, and which survives.
// They live here, in the port both already depend on, so the interface contacts
// declares and the method consent implements are talking about one shape.
//
// WHY THE SEAM EXISTS AT ALL. Until it did, a merge carried the grants and
// left the stops on a record nothing evaluates any more. The stop was not
// deleted — it was orphaned, which is worse, because an export still shows it
// while the send path no longer sees it. Marketing resumed against somebody
// who had refused it, and nothing in the audit said so.

import "github.com/margince/margince/backend/internal/shared/kernel/ids"

// StopSubject names one side of a carry: a contact, or a lead.
//
// Exactly one of the two ids is set. A merge names two contacts; a promotion
// names the lead and the contact it became.
type StopSubject struct {
	ContactID ids.ContactID
	LeadID    ids.LeadID
}

// ContactStopSubject names a contact, so a caller cannot pass a half-filled
// struct by accident.
func ContactStopSubject(id ids.ContactID) StopSubject { return StopSubject{ContactID: id} }

// LeadStopSubject names a lead, the other half of the same guard.
func LeadStopSubject(id ids.LeadID) StopSubject { return StopSubject{LeadID: id} }

// IsZero reports a subject naming nobody, which is a caller bug rather than an
// empty case: every carry has two real sides.
func (s StopSubject) IsZero() bool {
	return s.ContactID.IsZero() && s.LeadID.IsZero()
}
