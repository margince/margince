// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import "github.com/margince/margince/backend/internal/platform/config"

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
}, config.Store{RBACObject: "installation_settings", AuditVerb: settingsAuditVerb})
