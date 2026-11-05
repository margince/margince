// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RequireRowScopeNarrowerThan admits a caller whose row scope is narrower than
// most, and answers refusal otherwise. It guards an act that only a seat short
// of most needs, such as asking a colleague for a record. An all-scope seat may
// already change every record, and refusal tells it so.
func RequireRowScopeNarrowerThan(ctx context.Context, most principal.RowScope, refusal error) error {
	p, err := rbacActor(ctx)
	if err != nil {
		return err
	}
	c := Check{NarrowerThan: most}
	noteCheck(ctx, c)
	if c.HeldBy(p) {
		return nil
	}
	return refuseAs(ctx, c, refusal)
}

// refuseAs refuses the caller c with the caller's own refusal, and notes it.
func refuseAs(ctx context.Context, c Check, refusal error) error {
	if recording(ctx) {
		noteRefusal(ctx, c.String())
	}
	return refusal
}
