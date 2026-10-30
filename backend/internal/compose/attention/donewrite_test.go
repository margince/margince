// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// doneStoreSpy records what the attention service writes, and answers the
// rows it was told are already done.
type doneStoreSpy struct {
	already map[RowRef]time.Time
	marked  [][]RowRef
}

func (d *doneStoreSpy) DoneRows(context.Context, []RowRef) (map[RowRef]time.Time, error) {
	return d.already, nil
}

func (d *doneStoreSpy) MarkDone(_ context.Context, rows []RowRef) error {
	d.marked = append(d.marked, rows)
	return nil
}

func (d *doneStoreSpy) ReopenDone(context.Context, []RowRef) error { return nil }

// Done is written only for a row the reader's own day draws and that offers
// Done. Any other row is not found and nothing of the batch is written.
func TestDoneIsWrittenOnlyForARowTheReadersDayDraws(t *testing.T) {
	t.Parallel()
	meeting := ids.NewV7()
	shown := RowRef{Source: sourceMeeting, RowID: meeting.String()}
	unseen := RowRef{Source: sourceMeeting, RowID: ids.NewV7().String()}
	unseenDeal := RowRef{Source: sourceAtRisk, RowID: ids.NewV7().String()}
	alreadyDone := map[RowRef]time.Time{shown: rankInstant.Add(-time.Hour)}
	cases := []struct {
		name    string
		rows    []RowRef
		want    error
		already map[RowRef]time.Time
	}{
		{"a row on the day", []RowRef{shown}, nil, nil},
		{"a row already marked done is still the reader's", []RowRef{shown}, nil, alreadyDone},
		{"a row on no day the reader can see", []RowRef{shown, unseen}, apperrors.ErrNotFound, nil},
		{"a deal the reader is not shown", []RowRef{unseenDeal}, apperrors.ErrNotFound, nil},
	}
	for _, tc := range cases {
		spy := &doneStoreSpy{already: tc.already}
		kickOff := Meeting{ID: meeting, Subject: "Kick-off", StartsAt: readInstant.Add(30 * time.Minute), Preparation: ownPreparation(true)}
		svc := meetingPrepService([]Meeting{kickOff}).WithDone(spy)
		err := svc.MarkRowsDone(meetingPrepReader(), tc.rows)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: answered %v, want %v", tc.name, err, tc.want)
		}
		if wrote := len(spy.marked) > 0; wrote != (tc.want == nil) {
			t.Errorf("%s: wrote = %v, want %v", tc.name, wrote, tc.want == nil)
		}
	}
}

// A row in a lane that never offers Done is a fault in the body, answered as
// the contract's 422 naming `rows`, and nothing is written.
func TestDoneOnALaneThatNeverOffersItIsAValidationRefusal(t *testing.T) {
	t.Parallel()
	meeting := ids.NewV7()
	spy := &doneStoreSpy{}
	kickOff := Meeting{ID: meeting, Subject: "Kick-off", StartsAt: readInstant.Add(30 * time.Minute), Preparation: ownPreparation(true)}
	svc := meetingPrepService([]Meeting{kickOff}).WithDone(spy)

	err := svc.MarkRowsDone(meetingPrepReader(), []RowRef{{Source: sourceTask, RowID: meeting.String()}})

	fault, ok := httperr.Classify(err)
	if !ok || fault.Status != http.StatusUnprocessableEntity || len(fault.Fields) != 1 || fault.Fields[0].Field != "rows" {
		t.Fatalf("answered %+v (classified %v), want a 422 naming rows", fault, ok)
	}
	if len(spy.marked) > 0 {
		t.Fatalf("wrote %v for a refused batch", spy.marked)
	}
}

// An agent has no list of its own to clear.
func TestDoneIsRefusedToAnAgent(t *testing.T) {
	t.Parallel()
	spy := &doneStoreSpy{}
	agent := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalAgent, UserID: ids.NewV7(),
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
	err := meetingPrepService(nil).WithDone(spy).MarkRowsDone(agent,
		[]RowRef{{Source: sourceMeeting, RowID: ids.NewV7().String()}})
	if !errors.Is(err, apperrors.ErrPermissionDenied) || len(spy.marked) != 0 {
		t.Errorf("an agent's Done answered %v and wrote %d batches", err, len(spy.marked))
	}
}
