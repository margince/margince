// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
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

func TestASeatsWaitingMailKeepsLegacyNullEvidence(t *testing.T) {
	e := setupLoad(t)
	id := seedEmailRequest(t, e, "Please send the figures", "commitment", OwedVerdictAsksUs)
	store := storeKnowing(e)
	listed := func(at time.Time) map[ids.UUID]bool {
		t.Helper()
		rows, err := store.WaitingRepliesForOwnerBefore(e.as(), at, WaitingPosition{}, e.rep)
		if err != nil {
			t.Fatal(err)
		}
		out := make(map[ids.UUID]bool, len(rows))
		for _, row := range rows {
			out[row.ActivityID] = true
		}
		return out
	}
	recent := requestInstant.AddDate(0, 0, 1)
	if !listed(recent)[id] {
		t.Fatal("recent mail addressed to the seat is missing")
	}
	testdb.WithFixtureTriggerDisabled(t, "activity_participant", "stamp_participant_domain_evidence", func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE activity_participant
 SET domain_evidence_at=NULL WHERE activity_id=$1 AND role='to'`, id)
		return err
	})
	if !listed(recent)[id] {
		t.Fatal("recent mail with legacy null evidence is missing")
	}
	if listed(pastTheHorizon)[id] {
		t.Fatal("old unheld mail remains on the seat's waiting lane")
	}
}

// filteredAt reports whether waiting_reply=true lists the message, read at
// instant on the same store clock the badge reads.
func filteredAt(t *testing.T, e *loadEnv, id ids.UUID, instant time.Time) bool {
	t.Helper()
	rows, _, err := readingAt(storeKnowing(e), instant).ListActivities(e.as(), ListActivitiesInput{
		WaitingReply: true, WaitingReplyAsOf: &instant,
	})
	if err != nil {
		t.Fatalf("reading waiting_reply: %v", err)
	}
	for _, row := range rows {
		if ids.UUID(row.Id) == id {
			return true
		}
	}
	return false
}

// At the horizon's edge the badge and the waiting lane agree, day by day, so
// the boundary is the Worklist's whichever side it falls on. Past it the badge
// reads none.
func TestTheBadgeAndTheLaneShareTheHorizonsEdge(t *testing.T) {
	e := setupLoad(t)
	id := seedEmailRequest(t, e, "Please confirm the delivery date", "commitment", OwedVerdictAsksUs)
	for days := 28; days <= 32; days++ {
		at := requestInstant.AddDate(0, 0, days)
		badge := moveAt(t, e, id, at) == crmcontracts.EmailSummaryMoveNeedsReply
		if lane := e.waitingAt(e.as(), t, at)[id]; badge != lane {
			t.Errorf("%d days after the request: badge needs_reply=%v, lane=%v", days, badge, lane)
		}
	}
	if got := moveAt(t, e, id, requestInstant.AddDate(0, 0, 32)); got != crmcontracts.EmailSummaryMoveNone {
		t.Errorf("32 days after the request the badge reads %q, want none", got)
	}
}

// A stored horizon is judged fresh at the reading's own instant, by the
// filter and the badge alike. Read on a day the measurement still governs, the
// filter must not fall back to the compiled horizon while the badge keeps the
// measured one.
//
// Mutation: read the filter's horizon at the store's own clock instead of
// WaitingReplyAsOf and the filter lists a mail its badge calls none.
func TestTheFilterAndTheBadgeReadOneStoredHorizon(t *testing.T) {
	e := setupLoad(t)
	id := seedEmailRequest(t, e, "Please send the signed order", "commitment", OwedVerdictAsksUs)
	at := requestInstant.AddDate(0, 0, 10)
	e.exec(t, `INSERT INTO activity_waiting_horizon (singleton, horizon_days, measured_at) VALUES (true, 5, $1)
		ON CONFLICT (singleton) DO UPDATE SET horizon_days = 5, measured_at = $1`, at.Add(-time.Hour))

	if got := moveAt(t, e, id, at); got != crmcontracts.EmailSummaryMoveNone {
		t.Fatalf("10 days after the request, under a 5-day horizon, the badge reads %q, want none", got)
	}
	if filteredAt(t, e, id, at) {
		t.Fatal("waiting_reply lists the request its badge calls none")
	}
	if !filteredAt(t, e, id, requestInstant.AddDate(0, 0, 1)) {
		t.Fatal("waiting_reply does not list the day-old request, so the case above proves nothing")
	}
}

// A chat message follows the mail rule through message_move: owed while
// current, none past the horizon.
func TestAChatMessagePastTheHorizonNeedsNoReply(t *testing.T) {
	e := setupLoad(t)
	direction, subject := "inbound", "Chat with the buyer"
	logged, _, err := storeKnowing(e).LogActivity(asClassifier(e), LogActivityInput{
		Kind: "message", Direction: &direction, Subject: &subject, OccurredAt: &requestInstant,
		Source: "manual", ChannelProvider: "whatsapp", ThreadKey: "chat-" + ids.NewV7().String(),
		Links: []ActivityLinkInput{{EntityType: "contact", EntityID: e.buyer(t)}},
	})
	if err != nil {
		t.Fatalf("logging the chat message: %v", err)
	}
	moveOf := func(instant time.Time) crmcontracts.ActivityMessageMove {
		t.Helper()
		read, err := readingAt(storeKnowing(e), instant).GetActivity(e.as(), ids.From[ids.ActivityKind](ids.UUID(logged.Id)), storekit.LiveOnly)
		if err != nil {
			t.Fatalf("reading the message: %v", err)
		}
		if read.MessageMove == nil {
			t.Fatal("the chat message carries no move")
		}
		return *read.MessageMove
	}
	if got := moveOf(requestInstant.AddDate(0, 0, 1)); got != crmcontracts.ActivityMessageMoveNeedsReply {
		t.Fatalf("a day-old chat message reads %q, want needs_reply", got)
	}
	if got := moveOf(pastTheHorizon); got != crmcontracts.ActivityMessageMoveNone {
		t.Fatalf("a chat message past the horizon reads %q, want none", got)
	}
}
