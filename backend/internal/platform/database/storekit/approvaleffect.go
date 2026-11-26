// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// Linking the writes an approval released to the approval itself.
//
// An approved effect writes through other modules' stores, each with its own
// audit row, and none of those rows otherwise says which decision released it.
// A receipt for the decision then cannot show what the decision changed. The
// link rides the context for the reason WithBatch does: every effect writer
// would otherwise need a parameter it has to remember.

import (
	"context"
	"maps"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// EvidenceKeyReleasedBy names the approval whose effect wrote the audit row.
const EvidenceKeyReleasedBy = "released_by_approval_id"

type releasedByKey struct{}

// WithReleasingApproval marks every audit row written under ctx as the work
// that approval released. The approval's own rows are left alone: they are the
// decision, not its effect.
func WithReleasingApproval(ctx context.Context, approvalID ids.UUID) context.Context {
	return context.WithValue(ctx, releasedByKey{}, approvalID)
}

// withReleasedByLink adds the link to a row the marked effect wrote.
//
//craft:ignore naked-any the audit evidence seam is jsonb; its members are each writer's own shape
func withReleasedByLink(ctx context.Context, entityID ids.UUID, evidence map[string]any) map[string]any {
	approval, ok := ctx.Value(releasedByKey{}).(ids.UUID)
	if !ok || approval.IsZero() || approval == entityID {
		return evidence
	}
	linked := make(map[string]any, len(evidence)+1)
	maps.Copy(linked, evidence)
	linked[EvidenceKeyReleasedBy] = approval.String()
	return linked
}

// EvidenceKeyConfirmedInConversation names the one-time code whose yes in the
// conversation released the work that wrote the audit row: the conversation's
// counterpart of EvidenceKeyReleasedBy. One code is good once, so two effects
// carrying one id are a replay.
const EvidenceKeyConfirmedInConversation = "confirmed_in_conversation"

type confirmedByKey struct{}

// WithConfirmingCode marks every audit row written under ctx as work its user
// confirmed in the conversation with the code named by once.
func WithConfirmingCode(ctx context.Context, once string) context.Context {
	return context.WithValue(ctx, confirmedByKey{}, once)
}

// withConfirmedByLink adds the code's id to a row the confirmed work wrote.
//
//craft:ignore naked-any the audit evidence seam is jsonb; its members are each writer's own shape
func withConfirmedByLink(ctx context.Context, evidence map[string]any) map[string]any {
	once, ok := ctx.Value(confirmedByKey{}).(string)
	if !ok || once == "" {
		return evidence
	}
	linked := make(map[string]any, len(evidence)+1)
	maps.Copy(linked, evidence)
	linked[EvidenceKeyConfirmedInConversation] = once
	return linked
}
