// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A pending card of a retired kind is refused on approval, because nothing
// would carry out the yes. Declining it still closes it.
func TestAPendingRetiredCardIsRefusedOnApprovalAndMayBeDeclined(t *testing.T) {
	for _, kind := range []string{kindTranscriptProposal, kindCommunicationReview} {
		t.Run(kind, func(t *testing.T) {
			e := setupStaging(t)
			seat := seatFor(e.ws, e.rep, objectActivity, objectCommunicationException)
			stage := func(hash string) ids.ApprovalID {
				t.Helper()
				var id ids.UUID
				if err := e.owner.QueryRow(context.Background(), `
					INSERT INTO approval (kind, proposed_by, on_behalf_of, proposed_change, diff_hash, expires_at,
					                      requested_decider_id, requested_decider_source)
					VALUES ($1, 'human:'||$2::text, $2::uuid, '{}', $3, now() + interval '1 day', $2::uuid, 'on_behalf_of')
					RETURNING id`, kind, e.rep, hash).Scan(&id); err != nil {
					t.Fatalf("seeding a pending %s card: %v", kind, err)
				}
				return ids.From[ids.ApprovalKind](id)
			}

			if _, err := e.svc.Decide(seat, stage("approve"), true, nil); !errors.Is(err, apperrors.ErrConflict) {
				t.Errorf("approving a retired %s card: err = %v, want a conflict", kind, err)
			}
			declined, err := e.svc.Decide(seat, stage("decline"), false, nil)
			if err != nil {
				t.Fatalf("declining a retired %s card: %v", kind, err)
			}
			if declined.Status != approvalStatusRejected {
				t.Errorf("a declined retired card reads %s, want %s", declined.Status, approvalStatusRejected)
			}
		})
	}
}
