// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// moveAt is the request's History badge, read at instant.
func moveAt(t *testing.T, e *loadEnv, id ids.UUID, instant time.Time) crmcontracts.EmailSummaryMove {
	t.Helper()
	summaries, err := readingAt(storeKnowing(e), instant).EmailSummariesByID(e.as(), []ids.UUID{id})
	if err != nil {
		t.Fatal(err)
	}
	return summaries[id].Move
}

// A request nobody answered reads "needs reply" while it is current, and none
// once it is older than the waiting horizon, as the waiting lane already said.
// A request someone took stays "needs reply" past the horizon.
//
// Mutation: drop withinWaitingHorizonSQL from moveSQL and the old request still
// reads needs_reply.
func TestARequestPastTheWaitingHorizonNeedsNoReply(t *testing.T) {
	e := setupLoad(t)
	id := seedEmailRequest(t, e, "Please send the price list", "commitment", OwedVerdictAsksUs)
	current := requestInstant.AddDate(0, 0, 1)

	if got := moveAt(t, e, id, current); got != crmcontracts.EmailSummaryMoveNeedsReply {
		t.Fatalf("a day-old request reads %q, want needs_reply", got)
	}
	if !e.waitingAt(e.as(), t, current)[id] {
		t.Fatal("the waiting lane does not hold the day-old request, so the badge has nothing to agree with")
	}
	if got := moveAt(t, e, id, pastTheHorizon); got != crmcontracts.EmailSummaryMoveNone {
		t.Fatalf("a request past the waiting horizon reads %q, want none", got)
	}
	if e.waitingAt(e.as(), t, pastTheHorizon)[id] {
		t.Fatal("the waiting lane still holds the old request, so the two would not disagree")
	}

	request := LogActivityInput{Kind: "task", Source: "manual", RequestActivityID: &id}
	if _, _, err := storeKnowing(e).LogActivity(e.asSeat(e.rep), request); err != nil {
		t.Fatalf("taking the request: %v", err)
	}
	if got := moveAt(t, e, id, pastTheHorizon); got != crmcontracts.EmailSummaryMoveNeedsReply {
		t.Fatalf("a request someone took reads %q past the horizon, want needs_reply", got)
	}
}
