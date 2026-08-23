// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

// A buyer cannot record a decision on a document, whatever their seat says.
//
// The buttons were removed from the buyer's screen, and a removed button is not
// a removed authority: a reviewer holds a live credential and can call the
// endpoint directly. This is the test that says the refusal lives in the store
// rather than in one client.

import (
	"context"
	"errors"
	"testing"

	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

func TestNoSeatCanRecordADocumentDecision(t *testing.T) {
	store := &Store{}
	for _, capability := range []string{capabilityView, capabilityComment, capabilityReviewer} {
		session := Session{
			ID:            ids.NewV7(),
			ParticipantID: ids.From[ids.DealRoomParticipantKind](ids.NewV7()),
			RoomID:        ids.From[ids.DealRoomKind](ids.NewV7()),
			Capability:    capability,
		}
		_, err := store.DecideAsBuyer(context.Background(), session, ids.NewV7(), decisionConfirmVersion, nil)
		if !errors.Is(err, ErrDecisionsRetired) {
			t.Errorf("a %s seat confirming a document answered %v, want ErrDecisionsRetired", capability, err)
		}
	}
}

// The refusal must not be reached before the session check: an unauthenticated
// caller is refused for being unauthenticated, which is the answer that does
// not tell them what the endpoint would otherwise have done.
func TestAnUnauthenticatedDecisionIsRefusedAsUnauthenticated(t *testing.T) {
	store := &Store{}
	_, err := store.DecideAsBuyer(context.Background(), Session{}, ids.NewV7(), decisionConfirmVersion, nil)
	if errors.Is(err, ErrDecisionsRetired) {
		t.Fatal("a caller with no session was told why the feature is retired, rather than refused")
	}
	if err == nil {
		t.Fatal("a caller with no session was not refused at all")
	}
}
