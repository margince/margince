// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The approvals edge of capture: a dedupe collision becomes a 🟡 merge proposal
// a human answers, never an auto-merge.
//
// It lives apart from the registry wiring because it is the one place capture
// meets the approvals engine, and what it has to get right is a question about
// approvals rather than about capture — which repeats of one collision are the
// same question, so that a rep who says "these are not the same person" is
// answered once and not on every sync.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// mergeStager adapts the approvals engine to capture's dedupe seam.
type mergeStager struct {
	svc *approvals.Service
}

// mergeIdentityAddress is the payload key the merge identity is drawn from. The
// dedupe payload is a marshalled capture.LeadFields, which declares no JSON
// tags, so the key is the Go field name.
const mergeIdentityAddress = "Email"

func (m mergeStager) StageMerge(ctx context.Context, in capture.MergeProposal) (ids.UUID, error) {
	digest := sha256.Sum256(in.ProposedChange)
	// A connector re-syncing the same upstream record hits the same collision
	// every cycle, and a rep who says "these are not the same person" must be
	// answered once rather than every cycle after.
	//
	// The identity is the ADDRESS the collision was found on, against a target
	// that already names the incumbent lead: those two together are the whole of
	// what a rep answers. Not the whole payload, which is what a diff hash would
	// key on — a re-sync carrying a corrected title or a newly populated company
	// name is the same question about the same two records, and a memory keyed on
	// the payload forgets the refusal the first time any field moves.
	identity, err := json.Marshal(map[string]string{mergeIdentityAddress: capture.MergeAddress(in.ProposedChange)})
	if err != nil {
		return ids.Nil, fmt.Errorf("compose: marshal merge identity: %w", err)
	}
	id, _, err := m.svc.StageUnlessDeclined(ctx, approvals.StageInput{
		Kind:           captureCollisionKind,
		ProposedChange: in.ProposedChange,
		DiffHash:       hex.EncodeToString(digest[:]),
		Identity:       identity,
		TargetType:     in.TargetType,
		TargetID:       in.TargetID,
		Summary:        in.Summary,
		// JoinPending absorbs the re-sync while a proposal is still waiting,
		// which is what the pending check here used to do on its own. Identity
		// staging requires it, and it is the same behaviour by a stronger route:
		// the join runs under the identity's row lock rather than as a separate
		// read that a concurrent decision can land between.
		JoinPending: true,
	})
	return id.UUID, err
}
