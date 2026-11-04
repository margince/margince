// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// createOnlyCtx is meetingCtx's rep without the activity update grant.
func createOnlyCtx(e *sendEnv) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"activity": {Read: true, Create: true},
				"contact":  {Read: true}, "deal": {Read: true},
				"company": {Read: true}, "project": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

// A log that replays a known source key and moves the meeting's status
// changes an existing row, so it needs the update grant a PATCH needs. The
// create grant alone is refused the same way, and the row keeps its status.
// A replay that moves nothing still answers with the row.
func TestAReplayThatMovesAMeetingNeedsTheUpdateGrant(t *testing.T) {
	e := setupSend(t)
	start := time.Now().Add(-48 * time.Hour)
	first := deliverMeeting(t, e, "evt-grant-1", "Northgate review", start,
		crmcontracts.ActivityMeetingStatusBooked)

	replay := func(status crmcontracts.ActivityMeetingStatus) (crmcontracts.Activity, error) {
		subject, system, key, s := "Northgate review", "calendar-test", "evt-grant-1", string(status)
		out, _, err := meetingStore(e).LogActivity(createOnlyCtx(e), LogActivityInput{
			Kind: string(crmcontracts.ActivityKindMeeting), Subject: &subject, OccurredAt: &start,
			MeetingStatus: &s, SourceSystem: &system, SourceID: &key, Source: "sync",
		})
		return out, err
	}

	if _, err := replay(crmcontracts.ActivityMeetingStatusHeld); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a create-only replay moving the status answered %v, want permission denied", err)
	}
	var stored string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT meeting_status FROM activity WHERE id = $1`, ids.UUID(first.Id)).Scan(&stored); err != nil {
		t.Fatalf("reading the meeting back: %v", err)
	}
	if stored != "booked" {
		t.Fatalf("the stored meeting_status is %q after a refused replay, want booked", stored)
	}

	same, err := replay(crmcontracts.ActivityMeetingStatusBooked)
	if err != nil || ids.UUID(same.Id) != ids.UUID(first.Id) {
		t.Fatalf("a create-only replay that moves nothing answered %v, %v; want the same row", same.Id, err)
	}
}
