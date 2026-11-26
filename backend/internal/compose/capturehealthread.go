// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The queries behind the capture-health page, held apart from the gate in front
// of them.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/capturehealth"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/jobs"
)

// readCaptureHealth answers what every mailbox is waiting on, and the sender
// queue as a whole.
//
// One transaction, so the per-mailbox rows and the classifier totals describe
// the same instant. Read separately they would drift by however long the first
// took, and an administrator comparing them would be told the difference is a
// backlog.
func readCaptureHealth(ctx context.Context, tx pgx.Tx, now time.Time) (crmcontracts.CaptureHealth, error) {
	out := crmcontracts.CaptureHealth{GeneratedAt: now.UTC(), ConnectionConcerns: []crmcontracts.CaptureConnectionConcern{}}
	concerns, err := capture.FleetConcernsTx(ctx, tx, now)
	if err != nil {
		return crmcontracts.CaptureHealth{}, err
	}
	shared := []crmcontracts.CaptureSharedMailboxConcern{}
	for _, concern := range concerns {
		v := concern.Connection
		if concern.Shared || concern.UserID == nil {
			shared = append(shared, crmcontracts.CaptureSharedMailboxConcern{
				ConnectionId: openapi_types.UUID(v.ID), Provider: v.Provider, AccountLabel: v.AccountLabel,
				Concern: crmcontracts.CaptureConcernKind(v.Concern), Since: v.ConcernSince,
				LastSuccessAt: v.LastSuccessAt, ErrorClass: v.LastErrorClass,
			})
			continue
		}
		out.ConnectionConcerns = append(out.ConnectionConcerns, crmcontracts.CaptureConnectionConcern{
			UserId: openapi_types.UUID(*concern.UserID), DisplayName: concern.DisplayName, ConnectionId: openapi_types.UUID(v.ID),
			Provider: v.Provider, AccountLabel: v.AccountLabel, Concern: crmcontracts.CaptureConcernKind(v.Concern),
			Since: v.ConcernSince, LastSuccessAt: v.LastSuccessAt, ErrorClass: v.LastErrorClass,
		})
	}
	out.SharedMailboxConcerns = &shared
	mailboxes, err := capturehealth.MailboxHealth(ctx, tx, now)
	if err != nil {
		return crmcontracts.CaptureHealth{}, err
	}
	out.Mailboxes = mailboxes
	owners, err := capturehealth.OwnerReviews(ctx, tx)
	if err != nil {
		return crmcontracts.CaptureHealth{}, err
	}
	out.OwnerReviews = &owners
	classifier, err := capturehealth.ClassifierHealth(ctx, tx, now)
	if err != nil {
		return crmcontracts.CaptureHealth{}, err
	}
	out.Classifier = classifier
	held, err := captureHeldMeetings(ctx, tx, now)
	if err != nil {
		return crmcontracts.CaptureHealth{}, err
	}
	out.HeldMeetings = held
	sweeps, err := captureSweepHealth(ctx, tx)
	if err != nil {
		return crmcontracts.CaptureHealth{}, err
	}
	out.Sweeps = sweeps
	categories, err := capture.ReadOperationalCategoriesTx(ctx, tx)
	if err != nil {
		return crmcontracts.CaptureHealth{}, err
	}
	out.Categories = &crmcontracts.CaptureHealthCategories{MachineThreads: categories.MachineThreads, UncertainThreads: categories.UncertainThreads, OwnerReviewMailboxes: categories.OwnerReviewMailboxes, UnreadMailboxes: categories.UnreadMailboxes}
	return out, nil
}

// captureHeldMeetings counts what the nightly pass has still to lift, by the
// pass's own selector and without its per-tick bound.
func captureHeldMeetings(
	ctx context.Context, tx pgx.Tx, now time.Time,
) (crmcontracts.CaptureHeldMeetings, error) {
	var (
		args   []any
		out    crmcontracts.CaptureHeldMeetings
		oldest *time.Time
	)
	arg := func(v any) int { args = append(args, v); return len(args) }
	query := `
		SELECT count(*), min(a.created_at)
		  FROM activity a
		 WHERE ` + filedMeetingHeldClause("a", arg)
	if err := tx.QueryRow(ctx, query, args...).Scan(&out.Count, &oldest); err != nil {
		return crmcontracts.CaptureHeldMeetings{}, fmt.Errorf(
			"compose: counting the filed meetings still held: %w", err)
	}
	out.OldestAgeSeconds = capturehealth.AgeSeconds(now, oldest)
	return out, nil
}

// sweepJobKinds names the job that runs each pass, for its declared cadence.
var sweepJobKinds = map[capture.Sweep]string{
	capture.SweepSettledThreadVerdicts: ConfidentialityVerdictArgs{}.Kind(),
	capture.SweepStrandedContacts:      LinkReconcileArgs{}.Kind(),
	capture.SweepFiledMeetingHolds:     LinkReconcileArgs{}.Kind(),
}

// captureSweepHealth reports every pass's latest receipt and last success.
func captureSweepHealth(ctx context.Context, tx pgx.Tx) ([]crmcontracts.CaptureSweepHealth, error) {
	states, err := capture.SweepStatesTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	out := make([]crmcontracts.CaptureSweepHealth, 0, len(states))
	for _, state := range states {
		row := crmcontracts.CaptureSweepHealth{
			Sweep:           crmcontracts.CaptureSweepHealthSweep(state.Sweep),
			LastSucceededAt: capturehealth.UTCOrNil(state.LastSucceededAt),
		}
		if spec, ok := jobs.SpecFor(sweepJobKinds[state.Sweep]); ok && spec.Cadence.Fixed > 0 {
			seconds := int(spec.Cadence.Fixed.Seconds())
			row.CadenceSeconds = &seconds
		}
		if last := state.Latest; last != nil {
			run := crmcontracts.CaptureSweepRun{
				Outcome:    crmcontracts.CaptureSweepRunOutcome(last.Outcome),
				FinishedAt: last.FinishedAt.UTC(),
				Processed:  last.Processed,
				CapHit:     last.CapHit,
			}
			if class := vettedSweepClass(last.ErrorClass); class != "" {
				run.ErrorClass = &class
			}
			row.LastRun = &run
		}
		out = append(out, row)
	}
	return out, nil
}
