// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package evidencereread queues the stage evidence reading again for the mail
// a removed own domain sent.
//
// While a domain counts as ours, the ledger refuses a buyer milestone read as
// met in its mail, and no row is written. Removing the domain cannot correct
// a row that was never written, so the text is read again.
package evidencereread

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// actor is the principal a page is read as. Nobody asked for the page.
const actor = "system:stage-evidence-reread"

// DefaultPage bounds the activities one job run judges and queues.
const DefaultPage = 200

// Job is the args of the stage_evidence_reread job, which compose declares.
type Job interface {
	jobs.WorkspaceScoped
	// SentFrom is the removed domain.
	SentFrom() string
	// PageAfter is the last activity the run before this one judged.
	PageAfter() ids.UUID
}

// Enqueuer inserts a job in the transaction that read the page.
type Enqueuer interface {
	EnqueueTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) error
}

// Worker judges one page of the domain's mail and queues a reading for each
// activity a buyer wrote. A full page queues the run for the page after it.
//
// The page, its readings and the next run commit together. A run that River
// repeats queues the same args, and the inserts coalesce.
type Worker[T Job] struct {
	Bind func(context.Context, jobs.WorkspaceScoped) (context.Context, error)
	Pool *pgxpool.Pool
	Own  deals.OwnDomainReader
	// Reads is false on a role with no model lane, which queues nothing.
	Reads bool
	// Queue answers the inserter, or why this role has none.
	Queue func() (Enqueuer, error)
	// Reading builds the queued reading of one activity for this removal.
	Reading func(job T, deal ids.DealID, activity ids.UUID) (river.JobArgs, *river.InsertOpts)
	// Next builds the run that continues after the given activity.
	Next func(job T, after ids.UUID) (river.JobArgs, *river.InsertOpts)
	// Page is DefaultPage unless a test sets a smaller one.
	Page int
	Log  *slog.Logger
}

// Work implements River's worker for one page.
func (w *Worker[T]) Work(ctx context.Context, job *river.Job[T]) error {
	bound, err := w.Bind(ctx, job.Args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	if !w.Reads {
		return nil
	}
	queue, err := w.Queue()
	if err != nil {
		return jobs.FaultContext(bound, err)
	}
	bound = principal.SystemActing(bound, actor)
	judged, queued, err := w.readPage(bound, queue, job.Args)
	if err != nil {
		return jobs.FaultContext(bound, err)
	}
	w.Log.InfoContext(bound, "stage evidence reread: page queued", "judged", judged, "queued", queued)
	return nil
}

// readPage answers how many activities the page held and how many readings it
// queued.
func (w *Worker[T]) readPage(ctx context.Context, queue Enqueuer, job T) (int, int, error) {
	size := w.Page
	if size <= 0 {
		size = DefaultPage
	}
	var judged, queued int
	err := database.WithTx(ctx, w.Pool, func(tx pgx.Tx) error {
		judged, queued = 0, 0
		sent, err := deals.DealMailSentFrom(ctx, tx, job.SentFrom(), job.PageAfter(), size)
		if err != nil {
			return err
		}
		judged = len(sent)
		for _, activity := range sent {
			reads, err := w.queueReading(ctx, tx, queue, job, activity)
			if err != nil {
				return err
			}
			if reads {
				queued++
			}
		}
		if len(sent) < size {
			return nil
		}
		args, opts := w.Next(job, sent[len(sent)-1])
		return queue.EnqueueTx(ctx, tx, args, opts)
	})
	return judged, queued, err
}

// queueReading queues the reading of one activity when a buyer wrote it.
//
// The activity is judged the way a first reading judges it. One the system
// may not read, or one no longer on a deal, queues nothing. An author still on
// our side would be refused again, so that reading is not spent.
func (w *Worker[T]) queueReading(
	ctx context.Context, tx pgx.Tx, queue Enqueuer, job T, activity ids.UUID,
) (bool, error) {
	facts, err := deals.ReadActivityAuthorship(ctx, tx, activity, w.Own)
	if errors.Is(err, apperrors.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if facts.AuthorSide != deals.AuthorBuyer {
		return false, nil
	}
	args, opts := w.Reading(job, facts.DealID, activity)
	return true, queue.EnqueueTx(ctx, tx, args, opts)
}
