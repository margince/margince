// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The day's surface, wired to the modules that own what it shows.
//
// attention is a compose subpackage and approvals, people and activities are
// modules, so every edge between them is bound here like any other cross-module
// edge. What crosses is four READS. No verb does: a card's approve, complete or
// merge goes to the endpoint that already owns it, so this surface can never
// become a second place where a decision's rules live.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gradionhq/margince/backend/internal/compose/attention"
	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/modules/activities"
	"github.com/gradionhq/margince/backend/internal/modules/approvals"
	"github.com/gradionhq/margince/backend/internal/modules/people"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/deadline"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// attentionApprovals reads the staged queue through the engine every approval
// surface reads, so the feed shows what the inbox shows.
type attentionApprovals struct{ svc *approvals.Service }

func (a attentionApprovals) ListWire(ctx context.Context, in attention.ApprovalQuery) ([]crmcontracts.Approval, error) {
	status := in.Status
	rows, _, err := a.svc.ListWire(ctx, approvals.ListInput{Status: &status, Limit: in.Limit})
	return rows, err
}

// attentionDuplicates reads the dedupe queue through the people store, which
// applies the both-sides-visible rule to the page and the count alike.
type attentionDuplicates struct{ store *people.Store }

func (d attentionDuplicates) OpenCandidates(ctx context.Context, limit int) ([]attention.DuplicatePair, error) {
	rows, _, err := d.store.ListDedupeCandidates(ctx, people.DedupeQueueInput{Limit: limit})
	if err != nil {
		return nil, err
	}
	pairs := make([]attention.DuplicatePair, 0, len(rows))
	for _, row := range rows {
		pairs = append(pairs, attention.DuplicatePair{
			ID:         row.ID,
			EntityType: row.EntityType,
			Confidence: row.Confidence,
		})
	}
	return pairs, nil
}

func (d attentionDuplicates) CountOpen(ctx context.Context) (int, error) {
	return d.store.CountOpenDedupeCandidates(ctx)
}

// attentionTasks reads open tasks through the activities store. A task is an
// activity of kind `task`, so this is the same read the task queue makes.
type attentionTasks struct{ store *activities.Store }

func (t attentionTasks) OpenForViewer(ctx context.Context, until time.Time, limit int) ([]attention.Task, error) {
	kind := activityKindTask
	rows, _, err := t.store.ListActivities(ctx, activities.ListActivitiesInput{Kind: &kind, Limit: &limit})
	if err != nil {
		return nil, err
	}
	open := make([]attention.Task, 0, len(rows))
	for _, row := range rows {
		if row.IsDone != nil && *row.IsDone {
			continue
		}
		// A task with no due date is still work somebody agreed to, but it is
		// not work for TODAY, and a queue that promised today's list would be
		// lying if it carried the undated backlog too. `until` is the end of
		// the day, so anything not yet past it is still ahead of the reader.
		if row.DueAt == nil || !deadline.Passed(row.DueAt, until) {
			continue
		}
		due := *row.DueAt
		open = append(open, attention.Task{
			ID:      ids.UUID(row.Id),
			Subject: subjectOfActivity(row),
			DueAt:   &due,
		})
	}
	return open, nil
}

// subjectOfActivity is the line a task shows. An activity always carries one
// for a task — the create path refuses a blank subject — so the fallback is
// for a row that predates that rule rather than a routine case.
func subjectOfActivity(row crmcontracts.Activity) string {
	if row.Subject != nil && *row.Subject != "" {
		return *row.Subject
	}
	return "(untitled task)"
}

// approvalStatusApproved is the decided status a receipt is read from. Spelled
// once here because the receipt lane asks for it by name and a typo would
// quietly return an empty lane rather than an error.
const approvalStatusApproved = "approved"

// attentionReceipts reads what ran without asking: approvals a system actor
// decided, which is exactly the set a human never chose.
type attentionReceipts struct{ svc *approvals.Service }

func (r attentionReceipts) Recent(ctx context.Context, since time.Time, limit int) ([]attention.Receipt, error) {
	status := approvalStatusApproved
	rows, _, err := r.svc.ListWire(ctx, approvals.ListInput{Status: &status, Limit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]attention.Receipt, 0, len(rows))
	for _, row := range rows {
		// Inside the window, not before it: `since` is the receipt lane's own
		// horizon, and the same authority answers "is this behind that" here as
		// answers it for a task's due date.
		if row.DecidedAt == nil || deadline.Passed(row.DecidedAt, since) {
			continue
		}
		summary := ""
		if row.Summary != nil {
			summary = *row.Summary
		}
		out = append(out, attention.Receipt{
			ID:         ids.UUID(row.Id),
			Kind:       row.Kind,
			Summary:    summary,
			OccurredAt: *row.DecidedAt,
		})
	}
	return out, nil
}

// newAttentionHandlers assembles the surface for the API role.
func newAttentionHandlers(pool *pgxpool.Pool, svc *approvals.Service) attention.Handlers {
	db := InstallationDB(pool)
	return attention.NewHandlers(attention.NewService(
		attentionApprovals{svc: svc},
		attentionDuplicates{store: people.NewStore(db)},
		attentionTasks{store: activities.NewStore(db)},
		attentionReceipts{svc: svc},
		func() time.Time { return time.Now().UTC() },
	))
}
