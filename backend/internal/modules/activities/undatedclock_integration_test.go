// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// An activity logged without a date has happened for every reader at once,
// whichever clock the reader judges by. The store's clock plays the app
// host's, an hour from the database's now() in either direction.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// databaseClockPlus is the database's own clock moved by shift, read on every
// call, so the skew is exact and no reading depends on this process's clock.
func databaseClockPlus(t *testing.T, e *sendEnv, shift time.Duration) func() time.Time {
	t.Helper()
	return func() time.Time {
		var at time.Time
		if err := e.owner.QueryRow(context.Background(), `SELECT clock_timestamp() + $1::interval`, shift.String()).Scan(&at); err != nil {
			t.Fatalf("reading the database clock: %v", err)
		}
		return at
	}
}

func logUndated(t *testing.T, e *sendEnv, store *Store, in LogActivityInput) ids.ActivityID {
	t.Helper()
	in.Source = "manual"
	out, _, err := store.LogActivity(meetingCtx(e), in)
	if err != nil {
		t.Fatalf("logging the undated %s: %v", in.Kind, err)
	}
	return ids.From[ids.ActivityKind](ids.UUID(out.Id))
}

// The transcript reader admits a promise only from a meeting that took place,
// judged at the reader's clock. Behind the database's, a meeting stamped with
// now() alone was refused as not yet held.
func TestAnUndatedMeetingHasTakenPlaceForAReaderWhoseClockIsBehind(t *testing.T) {
	e := setupSend(t)
	behind := databaseClockPlus(t, e, -time.Hour)
	meeting := logUndated(t, e, meetingStore(e).WithClock(behind), LogActivityInput{
		Kind: string(crmcontracts.ActivityKindMeeting), Body: new("Priya: I will send the pricing."),
	})
	var evidence CommitmentAdmissionEvidence
	if err := database.WithWorkspaceTx(meetingCtx(e), e.pool, func(tx pgx.Tx) error {
		var err error
		evidence, err = CommitmentAdmissionEvidenceTx(meetingCtx(e), tx, CommitmentAdmissionInput{Preview: true, SourceID: meeting})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if reason := AdmitAutomaticCommitment(evidence, behind(), time.UTC, nil); reason != CommitmentCurrent {
		t.Fatalf("an undated meeting read by a clock behind the database's is %q, want admitted", reason)
	}
}

// An undated inbound message is on the waiting list the moment it is logged.
func TestAnUndatedInboundMessageWaitsForAReplyAtOnceWhenTheStoreClockIsBehind(t *testing.T) {
	e := setupSend(t)
	customer := seedCustomer(t, e)
	store := meetingStore(e).WithClock(databaseClockPlus(t, e, -time.Hour)).WithOwnDomains(fixedDomains(nil))
	mail := logUndated(t, e, store, LogActivityInput{
		Kind: string(crmcontracts.ActivityKindEmail), Direction: new("inbound"),
		Subject: new("Contract?"), Body: new("Where is the contract"),
		Links: []ActivityLinkInput{{EntityType: "contact", EntityID: customer}},
	})
	entity := "contact"
	rows, _, err := store.ListActivities(meetingCtx(e), ListActivitiesInput{WaitingReply: true, EntityType: &entity, EntityID: &customer})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if ids.UUID(row.Id) == mail.UUID {
			return
		}
	}
	t.Fatalf("an undated inbound message is missing from the waiting list (%d rows)", len(rows))
}

// The last-activity clock counts only occurred_at <= now(). Stamped with a
// store clock ahead of the database's, an undated activity would not count.
func TestAnUndatedActivityIsTheLastActivityAtOnceWhenTheStoreClockIsAhead(t *testing.T) {
	e := setupSend(t)
	customer := seedCustomer(t, e)
	note := logUndated(t, e, meetingStore(e).WithClock(databaseClockPlus(t, e, time.Hour)), LogActivityInput{
		Kind: string(crmcontracts.ActivityKindNote), Subject: new("Call notes"),
		Links: []ActivityLinkInput{{EntityType: "contact", EntityID: customer}},
	})
	var occurred time.Time
	var last *time.Time
	if err := e.owner.QueryRow(context.Background(), `SELECT a.occurred_at, c.last_activity_at
		 FROM activity a, contact c WHERE a.id = $1 AND c.id = $2`, note.UUID, customer).Scan(&occurred, &last); err != nil {
		t.Fatal(err)
	}
	if last == nil || !last.Equal(occurred) {
		t.Fatalf("the contact's last activity is %v, want the undated note at %v", last, occurred)
	}
}
