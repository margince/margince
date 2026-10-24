// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// DirectableBy reports whether the caller may direct this refused message past
// every refusal on it.
//
// The exception's eligibility test: DirectSend asks it before it
// records an instruction and the refusal's offered actions ask it before they
// list "direct_send", so the offer and the door give one answer. It needs the
// exception grant and, for each refusal, a declared remedy the caller may take:
// the plain exception for any holder of the grant, the admin exception only for
// an admin. A refusal that declares neither (an undeclared reason) cannot be
// directed past by anyone.
func (r Review) DirectableBy(ctx context.Context) bool {
	return directableBy(ctx, r.Refusals)
}

func directableBy(ctx context.Context, refusals []RefusedRecipient) bool {
	if auth.Require(ctx, entityCommunicationException, principal.ActionCreate) != nil {
		return false
	}
	for _, refusal := range refusals {
		if commsauthz.HasRemedy(refusal.ReasonCode, commsauthz.RemedyException) {
			continue
		}
		if commsauthz.HasRemedy(refusal.ReasonCode, commsauthz.RemedyAdminException) &&
			auth.RequireAdmin(ctx) == nil {
			continue
		}
		return false
	}
	return true
}
