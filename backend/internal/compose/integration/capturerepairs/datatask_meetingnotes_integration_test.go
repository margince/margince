// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capturerepairs

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func storedAutomaticNotes(t *testing.T, e *integration.Env, status string) (ids.UUID, ids.UUID) {
	t.Helper()
	meeting, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "meeting", Source: "manual", MeetingStatus: &status})
	if err != nil {
		t.Fatal(err)
	}
	actor := principal.Principal{Type: principal.PrincipalConnector, ID: "connector:gdocs", UserID: e.Rep1, OnBehalfOf: e.Rep1, Permissions: integration.AdminPerms}
	ctx := principal.WithActor(e.Admin(), actor)
	source, url, body := "transcript", "https://docs.google.com/document/d/"+ids.NewV7().String(), "Ada will send the offer."
	raw := map[string]any{"google_doc": map[string]any{"meeting_id": ids.UUID(meeting.Id).String(), "revision_id": "stored-revision"}}
	// The old automatic writer left the meeting only in its source document metadata.
	transcript, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{Kind: "meeting", Source: "meeting_notes", SourceSystem: &source, SourceID: &url, Body: &body, Raw: &raw})
	if err != nil {
		t.Fatal(err)
	}
	return ids.UUID(meeting.Id), ids.UUID(transcript.Id)
}

func TestStoredAutomaticNotesRecordTheMeetingWithoutReplacingAnOutcome(t *testing.T) {
	for _, status := range []string{"booked", "held", "canceled", "no_show"} {
		t.Run(status, func(t *testing.T) {
			e := integration.Setup(t)
			ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
			meeting, transcript := storedAutomaticNotes(t, e, status)
			before := e.WsScalar(t, `SELECT version::text FROM activity WHERE id=$1`, meeting)
			task := activities.MeetingNotesDataTask(e.Activities)
			task.Steps[0].Limit, task.Steps[0].MinLimit = 1, 1
			e.RunDataTask(ctx, t, task)
			rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.SchedulerPerms)
			if _, err := e.Activities.GetActivity(rep, ids.From[ids.ActivityKind](transcript), storekit.LiveOnly); err != nil {
				t.Fatalf("notes lost their bounded reader: %v", err)
			}
			if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND source_activity_id=$2`, transcript, meeting); got != 1 {
				t.Fatal("stored notes were not connected")
			}
			want := status
			if status == "booked" {
				want = "held"
			}
			if got := e.WsScalar(t, `SELECT meeting_status FROM activity WHERE id=$1`, meeting); got != want {
				t.Fatalf("outcome = %q, want %q", got, want)
			}
			if status != "booked" && e.WsScalar(t, `SELECT version::text FROM activity WHERE id=$1`, meeting) != before {
				t.Error("an existing outcome was edited")
			}
			held := e.WsCount(t, `SELECT count(*) FROM activity WHERE id=ANY($1) AND meeting_status='held'`, []ids.UUID{meeting, transcript})
			if (want == "held" && held != 1) || (want != "held" && held != 0) {
				t.Fatalf("held pair count = %d", held)
			}
			if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id=$1 AND action='update' AND after ? 'source_activity_id'`, transcript); got != 1 {
				t.Fatalf("connection audits = %d, want 1", got)
			}
			// The notes take the meeting's readers in an audit of their own.
			if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id=$1 AND action='update' AND after ? 'participant_users'`, transcript); got != 1 {
				t.Fatalf("reader audits = %d, want 1", got)
			}
			if err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
				out, err := task.Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 100})
				if out.Changed != 0 {
					t.Errorf("replay changed %d notes", out.Changed)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestStoredNotesRepairRollsBackAndSkipsUntrustedOrImmutableRecords(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	meeting, transcript := storedAutomaticNotes(t, e, "booked")
	_, restricted := storedAutomaticNotes(t, e, "booked")
	e.RestrictActivity(t, restricted)
	_, malformed := storedAutomaticNotes(t, e, "booked")
	e.WsExec(t, `UPDATE activity SET raw=jsonb_set(raw, '{google_doc,meeting_id}', '"invalid"') WHERE id=$1`, malformed)
	_, human := storedAutomaticNotes(t, e, "booked")
	e.WsExec(t, `UPDATE activity SET captured_by=$1 WHERE id=$2`, "human:"+e.Rep1.String(), human)
	member := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)
	for _, repair := range []datatask.Task{activities.CaptureLabelsDataTask(), activities.MeetingNotesDataTask(e.Activities)} {
		err := database.WithTx(member, e.Pool, func(tx pgx.Tx) error {
			_, err := repair.Steps[0].Batch(member, tx, datatask.BatchIn{Limit: 10})
			return err
		})
		if !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Fatalf("member invoked system-only repair: %v", err)
		}
	}
	task := activities.MeetingNotesDataTask(e.Activities)
	original := task.Steps[0].Batch
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := original(ctx, tx, in)
		if err != nil {
			return out, err
		}
		return out, errors.New("interrupt after connecting notes")
	}
	e.RunDataTask(ctx, t, task)
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND source_activity_id IS NULL`, transcript); got != 1 {
		t.Fatal("connection escaped rollback")
	}
	if got := e.WsScalar(t, `SELECT meeting_status FROM activity WHERE id=$1`, meeting); got != "booked" {
		t.Fatal("outcome escaped rollback")
	}
	task.Steps[0].Batch = original
	e.WsExec(t, `UPDATE data_task_run SET next_attempt_at=NULL WHERE task_id=$1`, task.ID)
	e.RunDataTask(ctx, t, task)
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND source_activity_id=$2`, transcript, meeting); got != 1 {
		t.Fatal("repair did not resume")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id=ANY($1) AND source_activity_id IS NULL`, []ids.UUID{restricted, malformed, human}); got != 3 {
		t.Fatalf("ineligible notes were changed: %d unchanged", got)
	}
}
