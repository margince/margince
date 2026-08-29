// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The lanes an installation may not serve at all.
//
// Meetings, deal risk and promises are each bound by compose or left nil, and a
// nil reader means the feed sends NO lane rather than an empty one — "this
// installation does not look here" is a different answer from "there is nothing
// here", and only the first is honest about what was never read.
//
// They live together because they share one shape: read, render, and either
// fill the lane or name it as withheld. Three copies of that in Assemble is
// what pushed it past the length a reader can hold at once.

import (
	"context"
	"errors"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// optionalLane is one such lane: whether it is bound, how to read it, and where
// its items and count belong on the answer.
type optionalLane struct {
	name  string
	bound bool
	read  func() ([]crmcontracts.AttentionItem, error)
	into  **[]crmcontracts.AttentionItem
	count **int
}

// collect runs the lane and files its result, returning the omitted list the
// caller carries. An unbound lane is skipped entirely, which is what leaves the
// field absent on the wire.
//
// A refusal NAMES the lane; any other failure is returned, because a lane that
// is broken rather than withheld must not read as a quiet one.
//
// ErrModeNotOverlay is unbound-at-read: whether an installation runs in
// overlay mode is a fact only the read can answer, and a workspace that is
// not simply does not have the lane — absent like an unbound one, never
// withheld, because nothing was hidden from this reader.
func (l optionalLane) collect(
	omitted []crmcontracts.AttentionLanesOmitted,
) ([]crmcontracts.AttentionLanesOmitted, error) {
	if !l.bound {
		return omitted, nil
	}
	items, err := l.read()
	switch {
	case errors.Is(err, apperrors.ErrModeNotOverlay):
		return omitted, nil
	case errors.Is(err, apperrors.ErrPermissionDenied):
		return append(omitted, crmcontracts.AttentionLanesOmitted(l.name)), nil
	case err != nil:
		return omitted, err
	default:
		*l.into = &items
		count := len(items)
		*l.count = &count
		return omitted, nil
	}
}

// optionalLanes describes the optional lanes, in the order they are read.
//
// The meetings window opens at NOW rather than at midnight: a meeting that
// began an hour ago cannot be prepared for, and one that ended would be plainly
// wrong on a lane headed "still ahead".
func (s *Service) optionalLanes(
	ctx context.Context, asOf time.Time, out *crmcontracts.Attention,
) []optionalLane {
	lanes := []optionalLane{
		{
			name: "meetings", bound: s.meetings != nil,
			read: func() ([]crmcontracts.AttentionItem, error) {
				booked, err := s.meetings.Today(ctx, asOf, endOfDay(asOf), plannedCap)
				return renderEach(booked, meetingItem), err
			},
			into: &out.Meetings, count: &out.Counts.Meetings,
		},
		{
			name: "at_risk", bound: s.atRisk != nil,
			read: func() ([]crmcontracts.AttentionItem, error) {
				risky, err := s.atRisk.Quiet(ctx)
				return renderEach(risky, riskItem), err
			},
			into: &out.AtRisk, count: &out.Counts.AtRisk,
		},
		{
			name: "commitments", bound: s.commitments != nil,
			read: func() ([]crmcontracts.AttentionItem, error) {
				due, err := s.commitments.DueBy(ctx, endOfDay(asOf), plannedCap)
				return renderEach(due, func(promise Commitment) crmcontracts.AttentionItem {
					return commitmentItem(promise, asOf)
				}), err
			},
			into: &out.Commitments, count: &out.Counts.Commitments,
		},
		{
			name: "did_not_run", bound: s.failed != nil,
			read: func() ([]crmcontracts.AttentionItem, error) {
				undone, err := s.failed.Failed(ctx, doneCap)
				return renderEach(undone, failedItem), err
			},
			into: &out.DidNotRun, count: &out.Counts.DidNotRun,
		},
		{
			name: "dsr", bound: s.dsrs != nil,
			read: func() ([]crmcontracts.AttentionItem, error) {
				owed, err := s.dsrs.OpenDueSoonest(ctx, doneCap)
				return renderEach(owed, func(request DSRCase) crmcontracts.AttentionItem {
					return dsrItem(request, asOf)
				}), err
			},
			into: &out.Dsr, count: &out.Counts.Dsr,
		},
		{
			name: "relationship_decay", bound: s.decay != nil,
			read: func() ([]crmcontracts.AttentionItem, error) {
				lapsed, err := s.decay.Lapsed(ctx)
				return renderEach(lapsed, lapsedItem), err
			},
			into: &out.RelationshipDecay, count: &out.Counts.RelationshipDecay,
		},
	}
	return append(lanes, s.operationalLanes(ctx, asOf, out)...)
}

// operationalLanes is the second half of the list: what is broken between
// this reader and the world — the sync, their mailboxes, their delegated AI
// work, their sends. Split from optionalLanes on the function-length
// ceiling; the shared collect loop walks both halves as one list.
func (s *Service) operationalLanes(
	ctx context.Context, asOf time.Time, out *crmcontracts.Attention,
) []optionalLane {
	return []optionalLane{
		{
			name: "sync_health", bound: s.syncHealth != nil,
			read: func() ([]crmcontracts.AttentionItem, error) {
				concerns, err := s.syncHealth.Concerns(ctx)
				return renderEach(concerns, syncItem), err
			},
			into: &out.SyncHealth, count: &out.Counts.SyncHealth,
		},
		{
			name: "capture_health", bound: s.captureHealth != nil,
			read: func() ([]crmcontracts.AttentionItem, error) {
				concerns, err := s.captureHealth.CaptureConcerns(ctx)
				return renderEach(concerns, captureItem), err
			},
			into: &out.CaptureHealth, count: &out.Counts.CaptureHealth,
		},
		{
			name: "ai_work_health", bound: s.aiWork != nil,
			read: func() ([]crmcontracts.AttentionItem, error) {
				// Failures age out with the window; a day is the widest a
				// reader still acts on, and the stalled arm ignores it.
				troubled, err := s.aiWork.Troubled(ctx, asOf.Add(-24*time.Hour), doneCap)
				return renderEach(troubled, aiWorkItem), err
			},
			into: &out.AiWorkHealth, count: &out.Counts.AiWorkHealth,
		},
		{
			name: "automation_health", bound: s.automations != nil,
			read: func() ([]crmcontracts.AttentionItem, error) {
				// The same week the bounce lane keeps: a broken rule needs
				// fixing whenever its owner next sits down.
				troubled, err := s.automations.TroubledRuns(ctx, asOf.Add(-7*24*time.Hour), doneCap)
				return renderEach(troubled, automationItem), err
			},
			into: &out.AutomationHealth, count: &out.Counts.AutomationHealth,
		},
		{
			name: "bounces", bound: s.bounces != nil,
			read: func() ([]crmcontracts.AttentionItem, error) {
				// A week, not a day: a dead address needs fixing whenever the
				// rep next sits down, and a bounce that ages out unseen is
				// the invisible failure this lane exists to end.
				undelivered, err := s.bounces.HardBounces(ctx, asOf.Add(-7*24*time.Hour), doneCap)
				return renderEach(undelivered, bounceItem), err
			},
			into: &out.Bounces, count: &out.Counts.Bounces,
		},
	}
}

// renderEach turns one producer's rows into wire items — the loop the lanes
// share, so they differ only in what they read and how a row is drawn.
//
// Never nil: the empty slice rather than a null the contract promised was a list.
func renderEach[T any](rows []T, draw func(T) crmcontracts.AttentionItem) []crmcontracts.AttentionItem {
	items := make([]crmcontracts.AttentionItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, draw(row))
	}
	return items
}

// fill files one REQUIRED lane's result: a refusal names the lane, any other
// error is handed back, and success runs the assignment the caller supplied.
//
// The four required lanes each spelled that switch out. They differ only in
// which fields they set, which is what the closure carries — and a lane that is
// broken rather than withheld still fails the whole feed rather than reading as
// a quiet day.
func fill(
	omitted []crmcontracts.AttentionLanesOmitted, name string, err error, assign func(),
) ([]crmcontracts.AttentionLanesOmitted, error) {
	switch {
	case errors.Is(err, apperrors.ErrPermissionDenied):
		return append(omitted, crmcontracts.AttentionLanesOmitted(name)), nil
	case err != nil:
		return omitted, err
	default:
		assign()
		return omitted, nil
	}
}
