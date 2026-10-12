// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

// How a pass CLOSES what it no longer owes an answer for.
//
// Two sweeps, and the distinction between them is the whole of this file: one
// says the condition went, the other says the record did. Folding them into a
// single "not open any more" would make the two indistinguishable forever,
// which is the thing an audit of a forecast cannot afford.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CloseCleared closes every open finding of the named types, on the named
// subjects, that this pass no longer observes. Both bounds carry meaning: the
// types are the rules whose required sources were actually read tonight — for
// the others "not observed" means "not looked" — and the subjects are the
// deals this pass actually evaluated, because absence is only a fact about
// what was walked. A deal that left the eligible set is deliberately NOT
// cleared here; its findings are a different question with a different answer.
//
// A finding under a LIVE deferral is left alone: "remind me next week" is an
// answer about when, and a condition that dips out for one night — a push
// count rolling off its window does this on its own — must not eat the
// reminder. If the condition is truly gone next week, the reminder fires over
// a clean record and clears then.
//
// No per-finding resolution row is written: assurance_resolution attributes an
// answer to a contact (actor_id is NOT NULL because an answer is BY somebody),
// and the scan is not one. The night's clearing is recorded once, on the run's
// own audit row, where FinishRun carries the cleared count.
func (s *Store) CloseCleared(ctx context.Context, tx pgx.Tx, types []string, subjects, seen []string) (int64, error) {
	if err := auth.Require(ctx, "forecast", principal.ActionUpdate); err != nil {
		return 0, err
	}
	if len(types) == 0 || len(subjects) == 0 {
		return 0, nil
	}
	if seen == nil {
		// A pass that found NOTHING has an empty seen set, not an absent one.
		// pgx encodes a nil slice as SQL NULL, and `NOT (x = ANY(NULL))` is
		// NULL — which silently filters every row and clears nothing, on
		// exactly the night everything should clear.
		seen = []string{}
	}
	// The deferral carve-out compares against OutcomeRemindLater, spelled from
	// the constant rather than written into the SQL: a literal here agrees with
	// the writer of the outcome only until one of them is renamed, and the
	// clause that stops matching clears every deferred finding silently.
	tag, err := tx.Exec(ctx, `
		UPDATE assurance_exception
		SET status = $1, updated_at = now()
		WHERE status = 'open'
		  AND subject_kind = 'deal'
		  AND subject_id = ANY($2::uuid[])
		  AND type = ANY($3)
		  AND NOT (logical_key = ANY($4))
		  AND NOT EXISTS (SELECT 1 FROM assurance_resolution r
		                   WHERE r.exception_id = assurance_exception.id
		                     AND r.outcome = $5
		                     AND r.remind_at > now())`,
		ExceptionConditionCleared, subjects, types, seen, OutcomeRemindLater)
	if err != nil {
		return 0, fmt.Errorf("assurance: closing cleared findings: %w", err)
	}
	return tag.RowsAffected(), nil
}

// CloseDeparted closes every open finding whose subject is no longer in the
// eligible set — a deal that was won, lost or archived since it was raised.
//
// A THIRD fact, and the two words already here both say something false about
// it. `resolved` claims somebody answered it; `condition_cleared` claims the
// condition stopped being true, which is exactly what CloseCleared refuses to
// say about a deal it never walked. The finding was never fixed: the record
// stopped being checkable. Left without a word of its own, those findings stay
// open forever, so the queue carries rows nobody can act on and its count is
// wrong by however many deals closed this quarter.
//
// It asks the DEAL what it is, rather than taking the complement of the set the
// pass walked. The complement is the same answer only for a pass that read the
// whole pipeline, and it fails open in the worst direction: a read that
// returned nothing, or half, makes every finding outside it look departed and
// closes the queue in one statement. Naming the condition — not live, or no
// longer open — says what is meant and is true of a partial pass too.
//
// The join is a READ of another module's table, which table ownership permits;
// what this module writes is still only its own.
//
// NO DEFERRAL CARVE-OUT, unlike CloseCleared. "Remind me next week" is an
// answer about when to look again, and there is nothing to look at: the
// reminder would fire over a deal that is won, lost or gone. Departure
// outranks it.
//
// No resolution row, for CloseCleared's reason: assurance_resolution attributes
// an answer to a contact, and no contact answered this.
func (s *Store) CloseDeparted(ctx context.Context, tx pgx.Tx) (int64, error) {
	if err := auth.Require(ctx, "forecast", principal.ActionUpdate); err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE assurance_exception
		SET status = $1, updated_at = now()
		WHERE status = 'open'
		  AND subject_kind = 'deal'
		  AND EXISTS (SELECT 1 FROM deal d
		               WHERE d.id = assurance_exception.subject_id
		                 AND (d.archived_at IS NOT NULL OR d.status <> 'open'))`,
		ExceptionSubjectDeparted)
	if err != nil {
		return 0, fmt.Errorf("assurance: closing findings whose subject departed: %w", err)
	}
	return tag.RowsAffected(), nil
}
