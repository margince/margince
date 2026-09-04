// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Finishing a system-minted follow-up, whichever arm asked for it.
//
// Its own file because the WRITE is one concept and the workflow arms beside
// it are another: followupresolve.go decides when a loop has closed, and this
// decides what "the system's own task" means and completes it. The two doors
// below select different sets and share every other rule, which is the whole
// reason they are one file rather than one function.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CompleteOpenSystemTasksForLead completes every open system-minted task
// linked to the lead — see completeOpenSystemTasksLinkedBy for the write
// shape, the version-skew handling and what "system-minted" means.
func (s *Store) CompleteOpenSystemTasksForLead(ctx context.Context, leadID ids.LeadID) (int, error) {
	return s.completeOpenSystemTasksLinkedBy(ctx, leadLinkColumn, leadID.UUID)
}

// CompleteOpenSystemTasksForPerson is CompleteOpenSystemTasksForLead's
// sibling for a lead that PROMOTED: carryLeadActivities (people/promote.go)
// re-points a follow-up task's link from the lead onto the person it became,
// in the same transaction that emits lead.promoted, so a lead id can no
// longer find it — only the person id it was carried to can.
//
// Completion is bounded to the tasks that existed when the person did. This
// arm's caller runs asynchronously off the outbox, so "the person is fresh
// and cannot yet carry anything else" is only true up to the moment the
// promotion committed — a sibling automation (no_activity_reminder,
// check_in_cadence) can anchor its own system task on the same person before
// this handler runs, and completing every open system task on the person
// would claim that task too.
//
// The bound is the person's OWN created_at, and it takes no argument on
// purpose. A promotion mints the person in the same transaction that carries
// the tasks onto them, so that row's creation IS the promotion instant — and
// unlike a timestamp threaded in from the caller it is written by the same
// clock as the activity.created_at it is compared against. The event's
// app-stamped OccurredAt used to fill this role, and could not: a host clock
// trailing the database's by more than the gap between a follow-up task's
// creation and the promotion put the carried task on the wrong side of the
// bound, which returns completed == 0 with no error — a loop left open
// forever and nothing to notice it by.
func (s *Store) CompleteOpenSystemTasksForPerson(ctx context.Context, personID ids.PersonID) (int, error) {
	return s.completeOpenSystemTasksLinkedBy(ctx, personLinkColumn, personID.UUID)
}

// CompleteCarriedSystemTasks completes the open system-minted tasks among a
// NAMED set of activities — the ones a promotion moved from the lead onto the
// person, which the lead.promoted payload carries.
//
// Named ids rather than a link column, which is what makes it exact for a
// MERGE. The person-keyed reading answers "every open system task on this
// person", and on a survivor that includes reminders the promotion never
// touched; this answers "the tasks this promotion carried", which is a fact
// about the promotion. The same system-minted predicate applies — source AND
// captured_by together — so a task a caller planted is no more completable
// through this door than through the other.
func (s *Store) CompleteCarriedSystemTasks(ctx context.Context, activityIDs []ids.UUID) (int, error) {
	if len(activityIDs) == 0 {
		return 0, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	// No link join: the ids ARE the selection. Which record they hang off is
	// what the promotion just changed, so filtering on it would ask the
	// question this door exists to stop asking.
	where := storekit.SQLf("a.id = ANY($%d)", arg(activityIDs))
	return s.completeOpenSystemTasks(ctx, where, arg, &args)
}

// completeOpenSystemTasksLinkedBy completes every open system-minted task the
// given column and value select, each through UpdateActivity — the module's
// own write path — so every completion carries the write shape (audit row,
// activity.updated event) exactly like a human ticking the box. A bulk UPDATE
// would be invisible history. Replays are harmless: a completed task no
// longer matches the open filter.
//
// The selection and the completions are separate transactions, so two
// firings — an activity.captured racing a lead.promoted, or two firings that
// both resolve through this same helper — can both select the same open
// task. Each completion therefore carries the version the selection read,
// and a task somebody else finished first answers version skew and is
// SKIPPED. Without it the loser writes is_done = true over a row that
// already says so, and the task's history shows two identical completions of
// the same thing.
//
// The count is what THIS call completed, which is also why skew is not an
// error: another firing completing the task is the outcome this one wanted.
//
// "System-minted" is decided by source AND captured_by together: source
// rides the client create wire verbatim (any caller can spell "system" —
// see systemSource's doc), while captured_by is stamped from the authenticated
// principal — a planted source alone hands nothing to this path. It
// answers a COUNT rather than rows, so nothing about which records exist
// leaves a call that takes no read gate of its own; each completion's
// write is gated inside UpdateActivity.
//
// `column` is unexported and passed only by the two callers above, each a
// compile-time literal ("lead_id" / "person_id") — never a value off a
// request body. Its placeholder is spelled right here, beside the argument
// that fills it, rather than split across a call boundary.
//
// The created_at bound is derived from `column` rather than asked for
// alongside it, because the two are one fact: the bound reads the PERSON row
// $1 names, so it means something only when $1 is a person id. A separate
// flag would let a caller pair it with leadLinkColumn, and that pairing does
// not fail — it looks a lead id up in `person`, finds nothing, and completes
// nothing at all. leadLinkColumn asks the original, unbounded question: a
// lead's own follow-up cannot be confused with a sibling automation's task
// the way a shared person id can.
func (s *Store) completeOpenSystemTasksLinkedBy(ctx context.Context, column string, linkValue ids.UUID) (int, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	linkPos := arg(linkValue)
	where := storekit.SQLf(`EXISTS (SELECT 1 FROM activity_link l
			WHERE l.activity_id = a.id AND l.%s = $%d)`, column, linkPos)
	if column == personLinkColumn {
		// Both sides are Postgres's clock: person.created_at and
		// activity.created_at are each DEFAULT now(), and reading them in one
		// statement leaves no second clock for a caller to introduce. A person
		// that has since been deleted makes the subquery NULL, so nothing
		// matches and nothing is completed — the safe direction.
		where += storekit.SQLf(
			" AND a.created_at <= (SELECT p.created_at FROM person p WHERE p.id = $%d)", linkPos)
	}
	return s.completeOpenSystemTasks(ctx, where, arg, &args)
}

// completeOpenSystemTasks selects the open SYSTEM-MINTED tasks a caller's own
// predicate names, and completes each through UpdateActivity.
//
// The predicate is the caller's; everything else — what "system-minted" means,
// the open filter, the ordering, the version-skew handling — is shared, so the
// two doors above cannot come to disagree about which tasks are the system's
// to finish. `arg` and `args` are the caller's own argument slice, so every
// placeholder is derived from the value that fills it rather than counted by
// hand across a call boundary.
func (s *Store) completeOpenSystemTasks(
	ctx context.Context, where string, arg func(any) int, args *[]any,
) (int, error) {
	type openTask struct {
		id      ids.ActivityID
		version int64
	}
	var open []openTask
	err := s.tx(ctx, func(tx pgx.Tx) error {
		query := storekit.SQLf(`
			SELECT a.id, a.version FROM activity a
			WHERE %s AND a.kind = $%d AND a.source = $%d
			  AND (a.captured_by = $%d OR a.captured_by LIKE $%d)
			  AND a.is_done = false AND a.archived_at IS NULL
			ORDER BY a.id`,
			where,
			arg(string(crmcontracts.ActivityKindTask)), arg(systemSource),
			arg(systemCapturedBy), arg(systemCapturedByPattern))
		rows, err := tx.Query(ctx, query, (*args)...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var task openTask
			if err := rows.Scan(&task.id, &task.version); err != nil {
				return err
			}
			open = append(open, task)
		}
		return rows.Err()
	})
	if err != nil {
		return 0, err
	}
	completed := 0
	for _, task := range open {
		flipped, err := s.completeSystemTask(ctx, task.id, task.version)
		if err != nil {
			return 0, fmt.Errorf("completing system task %s: %w", task.id, err)
		}
		if flipped {
			completed++
		}
	}
	return completed, nil
}

// The two activity_link columns this file resolves follow-ups through, named
// because completeOpenSystemTasksLinkedBy derives its created_at bound from
// which one it was handed. Both are compile-time literals reaching SQL as
// identifiers, never a value off a request body.
const (
	leadLinkColumn   = "lead_id"
	personLinkColumn = "person_id"
)

// completionAttempts bounds the re-read below. Each attempt is a lost race
// with a DIFFERENT writer, so more than a couple means a row somebody is
// editing continuously; failing then is honest, and the workflow's own retry
// is the right place for the wait.
const completionAttempts = 3

// completeSystemTask completes one selected task, answering whether THIS call
// is what flipped it.
//
// The version is the one the selection read, so a row that moved underneath
// answers skew rather than writing. What happens next depends on why it moved,
// and the two reasons are not alike:
//
//   - a sibling firing completed it — there is nothing left to do, and writing
//     anyway would put a second identical completion in the task's history;
//   - anything else touched it — the task is still open and still has to be
//     completed, because the loop that opened it has closed and no later
//     firing is promised.
//
// So skew is a re-read, not a skip. Skipping unconditionally trades a noisy
// history for a follow-up task that can stay open forever, which is the worse
// of the two.
func (s *Store) completeSystemTask(ctx context.Context, id ids.ActivityID, version int64) (bool, error) {
	done := true
	for attempt := 0; attempt < completionAttempts; attempt++ {
		_, err := s.UpdateActivity(ctx, id, UpdateActivityInput{IsDone: &done, IfVersion: &version})
		if err == nil {
			return true, nil
		}
		if errors.Is(err, apperrors.ErrNotFound) {
			// Archived between the selection and the write: the row lock is
			// taken live, so a task that has gone answers not-found. There is
			// no task to finish and nothing here went wrong — failing the
			// firing would strand every OTHER task it selected.
			return false, nil
		}
		if !errors.Is(err, apperrors.ErrVersionSkew) {
			return false, err
		}
		current, err := s.GetActivity(ctx, id, storekit.LiveOnly)
		if errors.Is(err, apperrors.ErrNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if current.IsDone != nil && *current.IsDone {
			return false, nil
		}
		if current.Version == nil {
			return false, fmt.Errorf("task %s reports no version to retry against", id)
		}
		version = int64(*current.Version)
	}
	return false, fmt.Errorf("task %s was edited under every one of %d completion attempts", id, completionAttempts)
}
