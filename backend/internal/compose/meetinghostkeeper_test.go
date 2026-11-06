// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Handing a synced meeting to its organizer is a calendar fact, not an edit.
// The history offers no undo, because a replay cannot write a meeting's host.
func TestTheOrganizerHostMoveOffersNoUndo(t *testing.T) {
	row := AuditRow{
		EntityType: entityTypeActivity, Action: "update",
		Before:   []byte(`{"host_user_id":"0195e4a0-0000-7000-8000-000000000002"}`),
		After:    []byte(`{"host_user_id":"0195e4a0-0000-7000-8000-000000000001"}`),
		Evidence: []byte(`{"reason":"meeting_organizer_host"}`),
	}
	answer, err := Evaluator{}.Evaluate(context.Background(), nil, row, Advisory)
	if err != nil {
		t.Fatalf("judging the host move: %v", err)
	}
	if answer.Undoable || answer.Reason != ReasonNotRestorableByThisPath {
		t.Errorf("the host move reads %+v, want refused as not restorable by this path", answer)
	}
}

// hostTaskFailingTx is a database that fails every statement the task sends.
type hostTaskFailingTx struct {
	pgx.Tx
	err error
}

func (tx hostTaskFailingTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, tx.err
}

func (tx hostTaskFailingTx) QueryRow(context.Context, string, ...any) pgx.Row {
	return hostTaskFailedRow{err: tx.err}
}

type hostTaskFailedRow struct{ err error }

func (r hostTaskFailedRow) Scan(...any) error { return r.err }

func hostTaskActor(kind principal.PrincipalType) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{Type: kind, ID: "actor:test"})
}

// The repair writes colleagues' meetings, so only the system runs it, and the
// refusal comes before any statement.
func TestOnlyTheSystemRunsTheOrganizerHostTask(t *testing.T) {
	meetings := &activities.Store{}
	if _, err := claimOrganizerHostsBatch(hostTaskActor(principal.PrincipalHuman), refusingTx{}, meetings, datatask.BatchIn{}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a person's batch answered %v, want permission denied", err)
	}
	if _, _, err := estimateOrganizerHosts(hostTaskActor(principal.PrincipalHuman), refusingTx{}, 10); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a person's estimate answered %v, want permission denied", err)
	}
}

// A task resumes from its cursor, so one it cannot read stops it, and a failed
// read is reported rather than recorded as a finished batch with nothing to do.
func TestTheOrganizerHostTaskStopsOnWhatItCannotRead(t *testing.T) {
	ctx := hostTaskActor(principal.PrincipalSystem)
	meetings := &activities.Store{}

	_, err := claimOrganizerHostsBatch(ctx, refusingTx{}, meetings, datatask.BatchIn{Cursor: json.RawMessage(`"not-an-id"`)})
	if _, malformed := errors.AsType[*storekit.MalformedCursorError](err); !malformed {
		t.Errorf("a malformed cursor answered %v", err)
	}

	down := errors.New("the database went away")
	if _, err := claimOrganizerHostsBatch(ctx, hostTaskFailingTx{err: down}, meetings, datatask.BatchIn{Limit: 10}); !errors.Is(err, down) {
		t.Errorf("a failed list answered %v, want the failure", err)
	}
	if _, _, err := estimateOrganizerHosts(ctx, hostTaskFailingTx{err: down}, 10); !errors.Is(err, down) {
		t.Errorf("a failed estimate answered %v, want the failure", err)
	}
}
