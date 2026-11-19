// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// InlineSendReview is the pilot switch for the composer's one-step Send: the
// server decides at Send and a refusal shows inline with its review. Off keeps
// the check while writing and the confirmation dialog. Default off until the
// pilot has passed.
var InlineSendReview = config.DefineStored(config.Spec[bool]{
	Key:     "activities.inline_send_review",
	Env:     "MARGINCE_ACTIVITIES_INLINE_SEND_REVIEW",
	Type:    config.Bool,
	Default: config.Some(false),
	Doc:     "Decide a composed email at Send and show a refusal inline, instead of checking while it is written.",
}, config.Store{RBACObject: settingsObject, AuditVerb: settingsAuditVerb})

// InlineSendReviewFor reads the switch for the caller on the context, through
// the setting's own read gate. A seat that may not read installation settings
// is answered off, so the session that carries it still answers them.
func InlineSendReviewFor(cfg *config.Config) func(context.Context) (bool, error) {
	return func(ctx context.Context) (bool, error) {
		on, err := config.StoredVal(ctx, cfg, InlineSendReview)
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return false, nil
		}
		return on, err
	}
}
