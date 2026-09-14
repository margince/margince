// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ReadGranted answers the OBJECT half of RBAC as a boolean: may this caller
// read this object type at all.
//
// Named as a QUESTION and not as a refusal, deliberately. This package reserves
// Require/Ensure/Admit/Hold for the verbs that refuse a caller, and
// rbacgate_test.go holds that convention by failing a name under one of them
// whose body returns no error. A boolean called HoldsReadGrant would have read
// like a gate to every human who met it and behaved like a query.
//
// The same question Require asks, for the reads that must not refuse on the
// answer. Ordering a list by a company name is reading that name, so a seat
// holding project.read and no company.read may not be given the order — but
// failing the request would take their project list away over a column they
// never asked to sort by. Those reads DEGRADE: the order falls back, the count
// is omitted, the computed column is absent. Require cannot express that,
// because its answer is a 403.
//
// Use Require wherever refusing is the right answer. This is for the narrower
// case where the grant decides whether a FIELD appears, not whether the request
// is admitted — and a caller that inspects this and then serves the field
// anyway has written a gate that does nothing.
//
// It refuses a Deal Room buyer for refuseBuyer's reason, which is not the same
// as the empty-permissions accident that would refuse one today: a buyer holds
// no CRM authority, and stating it here means a constructor that starts minting
// buyers with permissions cannot quietly widen this.
//
// No query: the principal already carries its merged permissions, resolved once
// at authentication. The system principal is trusted by construction, mirroring
// Require's own carve-out, and a request with no actor bound fails closed.
func ReadGranted(ctx context.Context, object string) bool {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return false
	}
	if actor.Type == principal.PrincipalBuyer {
		return false
	}
	if actor.Type == principal.PrincipalSystem {
		return true
	}
	return actor.Permissions.Allows(object, principal.ActionRead)
}
