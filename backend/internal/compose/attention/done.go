// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Done on any row: the reader's "nothing more to do here, take it off my list".
//
// A row built from an activity keeps its Done on the activity, and its lane
// leaves it out. Every other row is named in the reader's own Done set. This
// file decides when such a row comes back: once its subject moves after the press.

import (
	"context"
	"errors"
	"fmt"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// doneInPlace are the sources whose row offers Done. Each one is otherwise left
// only by working the record elsewhere, or by waiting until its lane stops
// reporting it. A source absent here leaves through a verb of its own, and
// TestEverySourceOffersAWayOffTheList holds that each one does.
var doneInPlace = map[crmcontracts.WorklistItemSource]bool{
	crmcontracts.WorklistItemSourceAwaitingReply:       true,
	crmcontracts.WorklistItemSourceDomainQuestion:      true,
	crmcontracts.WorklistItemSourceMeetingFollowUp:     true,
	crmcontracts.WorklistItemSourceDealAtRisk:          true,
	crmcontracts.WorklistItemSourceMeeting:             true,
	crmcontracts.WorklistItemSourceLeadResponse:        true,
	crmcontracts.WorklistItemSourceIntroductionRequest: true,
	crmcontracts.WorklistItemSourceBatch:               true,
	crmcontracts.WorklistItemSourceAutomationRun:       true,
	crmcontracts.WorklistItemSourceAiWorkHealth:        true,
	crmcontracts.WorklistItemSourceCaptureHealth:       true,
	crmcontracts.WorklistItemSourceBounce:              true,
	crmcontracts.WorklistItemSourceUndelivered:         true,
	crmcontracts.WorklistItemSourceDsr:                 true,
	crmcontracts.WorklistItemSourceNoticeCase:          true,
}

// DoneStore keeps the reader's own Done set. OPTIONAL: unbound, no row is ever
// hidden as done and every Done is refused as not found.
type DoneStore interface {
	// DoneRows answers when this reader marked each of these rows done, and
	// every group row they marked.
	DoneRows(ctx context.Context, among []RowRef) (map[RowRef]time.Time, error)
	MarkDone(ctx context.Context, rows []RowRef) error
	ReopenDone(ctx context.Context, rows []RowRef) error
}

// doneState is the Done store, and on one request's copy what its reader
// marked and the zone a close date is judged in. probe, set on a copy, takes
// the rows a page could draw in place of the page.
type doneState struct {
	store DoneStore
	at    map[RowRef]time.Time
	zone  *time.Location
	probe func([]ranked)
}

// offersDone is whether a source's rows offer Done. dispositionsFor answers
// the same for a row, and always leads with Done where it offers any.
func offersDone(source crmcontracts.WorklistItemSource) bool {
	return source == sourceWaiting || doneInPlace[source]
}

// dispositionsFor is what a row offers a reader who is putting it down.
func dispositionsFor(row ranked, reader ids.UUID) []crmcontracts.WorklistItemDispositions {
	if domainMaintenanceBatch(row) {
		return nil
	}
	if row.item.Source == sourceWaiting {
		return waitingDispositions(row, reader)
	}
	if doneInPlace[row.item.Source] {
		return []crmcontracts.WorklistItemDispositions{disposeHandled}
	}
	return nil
}

// MarkRowsDone takes rows off this reader's own Worklist. Each row must offer
// Done and be one the reader is shown now. That is judged by assembling their
// day with every lane's own grant and row scope. A row on no scope the reader holds is
// not found.
func (s *Service) MarkRowsDone(ctx context.Context, rows []RowRef) error {
	if actor, ok := principal.Actor(ctx); !ok || actor.Type != principal.PrincipalHuman {
		return apperrors.ErrPermissionDenied
	}
	if s.marked.store == nil {
		return apperrors.ErrNotFound
	}
	missing := make(map[RowRef]bool, len(rows))
	for _, row := range rows {
		if !offersDone(crmcontracts.WorklistItemSource(row.Source)) {
			return httperr.Validation("rows", "not_doneable", fmt.Sprintf("a %q row offers no Done", row.Source))
		}
		missing[row] = true
	}
	if s.domainQuestions != nil {
		for row := range missing {
			if row.Source != string(sourceDomainQuestion) {
				continue
			}
			visible, err := s.domainQuestions.OwnsDomainQuestion(ctx, row.RowID)
			if err != nil {
				return err
			}
			if visible {
				delete(missing, row)
			}
		}
	}
	if len(missing) == 0 {
		return s.marked.store.MarkDone(ctx, rows)
	}
	for _, scope := range scopeOptionsFor(ctx) {
		if err := s.dropShown(ctx, scope, missing); err != nil {
			return err
		}
		if len(missing) == 0 {
			return s.marked.store.MarkDone(ctx, rows)
		}
	}
	return apperrors.ErrNotFound
}

// ReopenRows puts rows back on this reader's own Worklist. It only removes the
// reader's own Done, so it needs no look at the day.
func (s *Service) ReopenRows(ctx context.Context, rows []RowRef) error {
	if s.marked.store == nil {
		return nil
	}
	return s.marked.store.ReopenDone(ctx, rows)
}

// dropShown removes from missing every row this reader's day draws under one
// scope, its group members included, with nothing hidden as done.
func (s *Service) dropShown(ctx context.Context, scope string, missing map[RowRef]bool) error {
	probe := *s
	probe.marked.probe = func(candidates []ranked) {
		for _, row := range candidates {
			if !domainMaintenanceBatch(row) {
				delete(missing, refOf(row))
			}
			for _, member := range row.members {
				delete(missing, RowRef{Source: member.Source, RowID: member.RowID})
			}
		}
	}
	_, err := probe.worklistIn(ctx, scope, "", ids.UUID{}, worklistMaxPage, "")
	return err
}

// readingDone returns a copy carrying what this reader marked done among the
// rows the day can draw, and the zone close dates are judged in. A refused
// read is no Done, as a refused pin read is no pins.
func (s *Service) readingDone(ctx context.Context, day crmcontracts.Attention) (*Service, error) {
	if s.marked.store == nil || s.marked.probe != nil {
		return s, nil
	}
	// The refused rows are reported by worklistFrom, which admits the same day.
	candidates, _ := admitBySource(ctx, classifyDay(day, day.AsOf, s.money))
	among := make([]RowRef, 0, len(candidates))
	for _, row := range candidates {
		among = append(among, refOf(row))
	}
	at, err := s.marked.store.DoneRows(ctx, among)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	_, zone, err := s.endOfDay(ctx, day.AsOf)
	if err != nil {
		return nil, err
	}
	reading := *s
	reading.marked.at, reading.marked.zone = at, zone
	return &reading, nil
}

// dropDone leaves out every row this reader marked done that has not come
// back. Run once over the lanes' rows and once over the groups the fold mints,
// because a group's id did not exist before the fold.
func dropDone(rows []ranked, done doneState, asOf time.Time) []ranked {
	if len(done.at) == 0 {
		return rows
	}
	kept := rows[:0:0]
	for _, row := range rows {
		at, marked := done.at[refOf(row)]
		if marked && !cameBack(row, at, asOf, done.zone) {
			continue
		}
		kept = append(kept, row)
	}
	return kept
}

// cameBack is whether the row reports something on its subject after the
// press. That is its own event, the subject moving, a newer group member, or a
// deadline passing. A deal's close date passes by the overdue rule itself.
//
// A meeting's start is no event on it. A meeting marked done stays off the
// list until the reader undoes it or the meeting leaves the lane.
func cameBack(row ranked, doneAt, asOf time.Time, zone *time.Location) bool {
	if row.item.Source == crmcontracts.WorklistItemSourceMeeting {
		return false
	}
	after := func(moment *time.Time) bool {
		return moment != nil && moment.After(doneAt) && !moment.After(asOf)
	}
	if row.item.Source == crmcontracts.WorklistItemSourceDomainQuestion {
		return after(&row.subjectMovedAt)
	}
	if after(row.item.OccurredAt) || after(&row.subjectMovedAt) || after(&row.newestMember) {
		return true
	}
	due := row.item.DueAt
	if due == nil {
		return false
	}
	if row.item.Source == crmcontracts.WorklistItemSourceDealAtRisk {
		return !deals.CloseIsOverdue(*due, doneAt, zone) && deals.CloseIsOverdue(*due, asOf, zone)
	}
	return after(due)
}

// A bounded maintenance group cannot dismiss unseen members; review defers each question.
func domainMaintenanceBatch(row ranked) bool {
	return row.item.Batch != nil && row.item.Batch.Key == domainQuestionsBatchKey
}
