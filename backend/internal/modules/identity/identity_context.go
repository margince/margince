// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type identityKey struct{}

func withIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

func identityFrom(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey{}).(Identity)
	return id, ok
}

// selfActorCtx binds the account owner as the storekit actor for a mutation
// their own act triggered with no session or resolved Identity in the room —
// a redeemed reset token, a federated sign-in whose groups grant a role. It
// carries only what the audit trail and the outbox events need: which human
// it was. Shared by both callers so a self-triggered write is attributed the
// same way wherever it happens rather than each building its own principal.
func selfActorCtx(ctx context.Context, userID ids.UserID) context.Context {
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: principal.HumanIDPrefix + userID.String(), UserID: userID.UUID,
	})
}
