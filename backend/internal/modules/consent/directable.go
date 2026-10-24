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
// This is the exception's one eligibility test. DirectSend asks it before it
// records an instruction. The refusal's offered actions ask it before they list
// "direct_send", so the offer and the door give one answer. It needs the
// exception grant and, for each refusal, a declared remedy the caller may take.
// Any holder of the grant may take the plain exception; only an admin may take
// the admin exception. A refusal that declares neither cannot be directed past.
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
