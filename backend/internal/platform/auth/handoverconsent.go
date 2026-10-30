// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type handOverConsentKey struct{}

// WithHandOverConsent admits one colleague as the receiver of records the
// caller hands over, when that colleague agreed to take them. EnsureAssignee
// then asks only the seat half of AssigneeEligibleSQL for that colleague: the
// receiver's consent stands in for the caller's own write scope. It widens no
// other receiver, no read and no write authority over the records themselves.
func WithHandOverConsent(ctx context.Context, receiver ids.UUID) context.Context {
	return context.WithValue(ctx, handOverConsentKey{}, receiver)
}

func handOverConsented(ctx context.Context, dest ids.UUID) bool {
	receiver, ok := ctx.Value(handOverConsentKey{}).(ids.UUID)
	return ok && !dest.IsZero() && receiver == dest
}

// EnsureHandOverRecipient answers whether dest may receive a record they agreed
// to take: a live human seat that can write, inside the caller's scope or not.
// It is EnsureAssignee under WithHandOverConsent, so the seat SQL is shared.
func EnsureHandOverRecipient(ctx context.Context, tx pgx.Tx, dest ids.UUID) error {
	// Held to commit: EnsureAssignee's FOR KEY SHARE lets a suspension land
	// first. Ordinary assignments keep the lighter lock.
	if _, err := tx.Exec(ctx, `SELECT 1 FROM app_user WHERE id = $1 FOR NO KEY UPDATE`, dest); err != nil {
		return fmt.Errorf("auth: holding the receiver of a handover: %w", err)
	}
	return EnsureAssignee(WithHandOverConsent(ctx, dest), tx, dest)
}

// consentedAssigner is the caller with an unbounded row scope, which makes
// AssigneeEligibleSQL's scope half read TRUE and leaves its seat half.
func consentedAssigner(p principal.Principal) principal.Principal {
	p.Permissions.RowScope = principal.RowScopeAll
	return p
}
