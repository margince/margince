// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// colleagueCtx is the colleague of meetingCtx's rep, who reads only what an
// audience lets them.
func colleagueCtx(e *sendEnv) context.Context {
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.other.String(), UserID: e.other,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true}, "contact": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// sweepTranscripts runs one pass of the sweep as the system, as its job does.
func sweepTranscripts(t *testing.T, e *sendEnv) int {
	t.Helper()
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.ws), "system:transcript-audience")
	var page DriftedPage
	if err := database.WithWorkspaceTx(ctx, e.pool, func(tx pgx.Tx) error {
		var err error
		page, err = FollowDriftedTranscriptsTx(ctx, tx, ids.ActivityID{}, 200)
		return err
	}); err != nil {
		t.Fatalf("sweeping the transcripts: %v", err)
	}
	return page.Changed
}

func colleagueReads(t *testing.T, e *sendEnv, id ids.ActivityID) bool {
	t.Helper()
	_, err := meetingStore(e).GetActivityContent(colleagueCtx(e), id, storekit.LiveOnly)
	if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("reading as the colleague: %v", err)
	}
	return err == nil
}

// A transcript is read by whoever reads its meeting, as the meeting changes.
// A narrowing by hand reaches it at once. A guest any writer adds or takes off
// reaches it on the next sweep.
func TestATranscriptKeepsTheReadersOfTheMeetingItRecords(t *testing.T) {
	e := setupSend(t)
	customer, deal := seedCustomer(t, e), e.seedDeal(t, "Renewal")
	booked := string(crmcontracts.ActivityMeetingStatusBooked)
	meeting := customerMeeting(t, e, customer, "Quarterly review", clocktest.Now(t).Add(-2*time.Hour), &booked)
	created, err := transcriptOf(t, e, meeting, customer, deal)
	if err != nil {
		t.Fatalf("logging the transcript: %v", err)
	}
	transcript := ids.From[ids.ActivityKind](ids.UUID(created.Id))

	if _, err := meetingStore(e).SetAudience(meetingCtx(e), meeting,
		SetAudienceInput{Audience: string(crmcontracts.ActivityAudienceParticipants)}); err != nil {
		t.Fatalf("limiting the meeting to its guests: %v", err)
	}
	if colleagueReads(t, e, transcript) {
		t.Fatal("the colleague reads the transcript of a meeting limited to guests they are not one of")
	}
	var refused *InvalidAudienceError
	if _, err := meetingStore(e).SetAudience(meetingCtx(e), transcript,
		SetAudienceInput{Audience: string(crmcontracts.ActivityAudienceWorkspace)}); !errors.As(err, &refused) {
		t.Fatalf("widening the transcript alone answered %v, want the audience refused", err)
	}

	if _, err := e.owner.Exec(context.Background(), `INSERT INTO activity_participant (activity_id, user_id, role)
		VALUES ($1, $2, 'attendee')`, meeting, e.other); err != nil {
		t.Fatalf("adding the colleague as a guest: %v", err)
	}
	if n := sweepTranscripts(t, e); n != 1 {
		t.Fatalf("the sweep changed %d transcript(s) after a guest was added, want 1", n)
	}
	if !colleagueReads(t, e, transcript) {
		t.Fatal("a guest added to the meeting cannot read its transcript")
	}

	if _, err := e.owner.Exec(context.Background(),
		`DELETE FROM activity_participant WHERE activity_id = $1 AND user_id = $2`, meeting, e.other); err != nil {
		t.Fatalf("taking the colleague off the meeting: %v", err)
	}
	if n := sweepTranscripts(t, e); n != 1 {
		t.Fatalf("the sweep changed %d transcript(s) after a guest left, want 1", n)
	}
	if colleagueReads(t, e, transcript) {
		t.Fatal("a guest taken off the meeting still reads its transcript")
	}
	if n := sweepTranscripts(t, e); n != 0 {
		t.Errorf("a second sweep changed %d transcript(s), want 0", n)
	}
	var announced int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM event_outbox
		WHERE envelope->>'type' = 'activity.updated' AND envelope->'entity'->>'id' = $1
		  AND envelope->'payload'->'changed_fields' ? 'audience'`, transcript.String()).Scan(&announced); err != nil {
		t.Fatalf("counting the transcript's audience events: %v", err)
	}
	if announced != 3 {
		t.Errorf("the transcript announced %d audience change(s), want 3", announced)
	}
}
