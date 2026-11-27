// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"errors"
	"net/http"
	"slices"
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

func (d *doneStoreSpy) AlreadyDone(_ context.Context, rows []RowRef) (map[RowRef]bool, error) {
	out := map[RowRef]bool{}
	for _, row := range rows {
		_, out[row] = d.already[row]
	}
	return out, nil
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

// A second Done on a row its lane now hides answers success. The row is
// already off the reader's list, so only the rows still drawn are written.
func TestASecondDoneOnARowItsLaneHidesSucceedsWithoutAWrite(t *testing.T) {
	t.Parallel()
	meeting := ids.NewV7()
	shown := RowRef{Source: sourceMeeting, RowID: meeting.String()}
	putDown := RowRef{Source: string(sourceAwaitingReply), RowID: ids.NewV7().String()}
	cases := []struct {
		name  string
		rows  []RowRef
		wrote []RowRef
	}{
		{"the hidden row alone", []RowRef{putDown}, nil},
		{"beside a row still drawn", []RowRef{shown, putDown}, []RowRef{shown}},
	}
	for _, tc := range cases {
		spy := &doneStoreSpy{already: map[RowRef]time.Time{putDown: rankInstant.Add(-time.Hour)}}
		kickOff := Meeting{ID: meeting, Subject: "Kick-off", StartsAt: readInstant.Add(30 * time.Minute), Preparation: ownPreparation(true)}
		err := meetingPrepService([]Meeting{kickOff}).WithDone(spy).MarkRowsDone(meetingPrepReader(), tc.rows)
		if err != nil {
			t.Errorf("%s: answered %v, want success", tc.name, err)
		}
		if got := slices.Concat(spy.marked...); !slices.Equal(got, tc.wrote) {
			t.Errorf("%s: wrote %v, want %v", tc.name, got, tc.wrote)
		}
	}
}

// A row the reader put down does not carry a foreign row past the check. The
// batch is not found and nothing of it is written.
func TestARowAlreadyDoneDoesNotAdmitAForeignRowBesideIt(t *testing.T) {
	t.Parallel()
	putDown := RowRef{Source: string(sourceAwaitingReply), RowID: ids.NewV7().String()}
	foreign := RowRef{Source: string(sourceAwaitingReply), RowID: ids.NewV7().String()}
	spy := &doneStoreSpy{already: map[RowRef]time.Time{putDown: rankInstant.Add(-time.Hour)}}
	err := meetingPrepService(nil).WithDone(spy).MarkRowsDone(meetingPrepReader(), []RowRef{putDown, foreign})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("answered %v, want not found", err)
	}
	if len(spy.marked) != 0 {
		t.Errorf("wrote %v for a refused batch", spy.marked)
	}
}

// A task row keeps its own lifecycle, so Done must not change it.
func TestDoneOnALaneThatNeverOffersItIsAValidationRefusal(t *testing.T) {
	t.Parallel()
	meeting := ids.NewV7()
	spy := &doneStoreSpy{}
	kickOff := Meeting{ID: meeting, Subject: "Kick-off", StartsAt: readInstant.Add(30 * time.Minute), Preparation: ownPreparation(true)}
	svc := meetingPrepService([]Meeting{kickOff}).WithDone(spy)

	err := svc.MarkRowsDone(meetingPrepReader(), []RowRef{{Source: sourceTask, RowID: meeting.String()}})

	fault, ok := httperr.Classify(err)
	if !ok || fault.Status != http.StatusUnprocessableEntity || len(fault.Fields) != 1 || fault.Fields[0].Field != "rows[0].source" {
		t.Fatalf("answered %+v (classified %v), want a 422 naming rows[0].source", fault, ok)
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
