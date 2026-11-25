// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// The reason an agent gave for a change, kept on the audit rows the change
// wrote. It rides the context for the reason WithReleasingApproval does: every
// writer behind a tool would otherwise need a parameter it has to remember.

import (
	"context"
	"maps"
)

// EvidenceKeyStatedReason names the reason the actor gave for the write, in
// the actor's own words. Done for you shows it as the change's why.
const EvidenceKeyStatedReason = "stated_reason"

type statedReasonKey struct{}

// WithStatedReason marks every audit row written under ctx with why.
func WithStatedReason(ctx context.Context, why string) context.Context {
	return context.WithValue(ctx, statedReasonKey{}, why)
}

// stampStatedReason adds the reason to a row written under a marked context.
//
//craft:ignore naked-any the audit evidence seam is jsonb; its members are each writer's own shape
func stampStatedReason(ctx context.Context, evidence map[string]any) map[string]any {
	why, ok := ctx.Value(statedReasonKey{}).(string)
	if !ok || why == "" {
		return evidence
	}
	stated := make(map[string]any, len(evidence)+1)
	maps.Copy(stated, evidence)
	stated[EvidenceKeyStatedReason] = why
	return stated
}
