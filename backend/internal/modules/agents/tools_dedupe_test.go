// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// stubDuplicateQueue answers the seam without a store, and records which verb
// the tool chose for the candidate it was handed.
type stubDuplicateQueue struct {
	dismissed, reopened *ids.UUID
	refuse              error
}

func (s stubDuplicateQueue) Dismiss(_ context.Context, id ids.UUID) (DuplicateVerdict, error) {
	if s.dismissed != nil {
		*s.dismissed = id
	}
	return DuplicateVerdict{CandidateID: id.String(), Disposition: "not_a_duplicate"}, s.refuse
}

func (s stubDuplicateQueue) Reopen(_ context.Context, id ids.UUID) (DuplicateVerdict, error) {
	if s.reopened != nil {
		*s.reopened = id
	}
	return DuplicateVerdict{CandidateID: id.String(), Disposition: "open"}, s.refuse
}

func decide(t *testing.T, queue DuplicateQueue, args string) (json.RawMessage, error) {
	t.Helper()
	return decideDuplicate{queue: queue}.Handle(context.Background(), json.RawMessage(args))
}

func TestDecideDuplicateRoutesEachDecisionToItsOwnVerb(t *testing.T) {
	id := ids.NewV7()
	var dismissed, reopened ids.UUID
	queue := stubDuplicateQueue{dismissed: &dismissed, reopened: &reopened}

	out, err := decide(t, queue, `{"candidate_id":"`+id.String()+`","decision":"not_the_same"}`)
	if err != nil || dismissed != id || !reopened.IsZero() {
		t.Fatalf("not_the_same: err=%v dismissed=%v reopened=%v, want only the dismissal for %v", err, dismissed, reopened, id)
	}
	var verdict DuplicateVerdict
	if err := json.Unmarshal(out, &verdict); err != nil || verdict.Disposition != "not_a_duplicate" {
		t.Fatalf("verdict = %s (%v), want not_a_duplicate", out, err)
	}

	dismissed = ids.UUID{}
	if _, err := decide(t, queue, `{"candidate_id":"`+id.String()+`","decision":"reopen"}`); err != nil ||
		reopened != id || !dismissed.IsZero() {
		t.Fatalf("reopen: err=%v dismissed=%v reopened=%v, want only the re-open for %v", err, dismissed, reopened, id)
	}
}

func TestDecideDuplicateRefusesADecisionItDoesNotKnow(t *testing.T) {
	id := ids.NewV7()
	var dismissed ids.UUID
	_, err := decide(t, stubDuplicateQueue{dismissed: &dismissed},
		`{"candidate_id":"`+id.String()+`","decision":"merge"}`)
	var bad *BadArgsError
	if !errors.As(err, &bad) {
		t.Fatalf("decision merge = %v, want BadArgsError: merging is merge_records, not this verb", err)
	}
	if !dismissed.IsZero() {
		t.Fatal("an unknown decision still reached the queue")
	}
}

func TestDecideDuplicatePassesTheQueuesRefusalThrough(t *testing.T) {
	_, err := decide(t, stubDuplicateQueue{refuse: apperrors.ErrConflict},
		`{"candidate_id":"`+ids.NewV7().String()+`","decision":"reopen"}`)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a refused re-open = %v, want the queue's own conflict, unmasked", err)
	}
}
