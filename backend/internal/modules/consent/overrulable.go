// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// OverrulableRecipient is one refused contact a rep may vouch for, and the
// category their vouch would cover (POST /contacts/{id}/consent/allow).
type OverrulableRecipient struct {
	ContactID ids.UUID
	Category  string
}

// Overrulable is what the rep may overrule on this review: every refused
// recipient, or nothing. One recipient the rep may not overrule keeps the send
// refused, so the other vouches would change nothing.
func (r Review) Overrulable() []OverrulableRecipient {
	if len(r.Refusals) == 0 {
		return nil
	}
	out := make([]OverrulableRecipient, 0, len(r.Refusals))
	for _, refusal := range r.Refusals {
		decision := commsauthz.Decision{Verdict: commsauthz.VerdictDeny, ReasonCode: refusal.ReasonCode}
		if refusal.SubjectKind != entityContact || !decision.CanBeOverruledByCategory() ||
			!commsauthz.Category(refusal.Category).KnownForOverride() {
			return nil
		}
		contact, err := ids.Parse(refusal.SubjectID)
		if err != nil {
			return nil
		}
		out = append(out, OverrulableRecipient{ContactID: contact, Category: refusal.Category})
	}
	return out
}

// OverrulableBy is what this caller may overrule on the review: Overrulable,
// narrowed by the same authority the allow door asks of the caller. A caller who
// could not write one of the contacts is offered nothing, because the vouch for
// that contact would be refused.
func (g *Gate) OverrulableBy(ctx context.Context, review Review) ([]OverrulableRecipient, error) {
	overrulable := review.Overrulable()
	if len(overrulable) == 0 || !mayUpdateContacts(ctx) {
		return nil, nil
	}
	writable := true
	err := g.store.db.Tx(ctx, func(tx pgx.Tx) error {
		for _, o := range overrulable {
			ok, err := contactWritable(ctx, tx, o.ContactID)
			if err != nil || !ok {
				writable = ok
				return err
			}
		}
		return nil
	})
	if err != nil || !writable {
		return nil, err
	}
	return overrulable, nil
}

func mayUpdateContacts(ctx context.Context) bool {
	return auth.Require(ctx, entityContact, principal.ActionUpdate) == nil
}

// contactWritable separates a refusal (the caller may not write this contact,
// or may not see it) from a failure, which the caller must hear about.
func contactWritable(ctx context.Context, tx pgx.Tx, contact ids.UUID) (bool, error) {
	err := auth.EnsureWritable(ctx, tx, entityContact, contact)
	if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}
