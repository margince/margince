// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

// Whether the last reading still describes the records it read.
//
// A change bumps its subject's generation from one sequence. A run records the
// highest generation it could see BEFORE it read the deals. A change is unread
// when its generation is higher than every run's record. The marker commits
// only after the domain write it reports, so any generation a run saw belongs
// to a change that committed before that run read the deals. A change during a
// pass, or a pass that fails, therefore leaves the change unread. A redelivered
// event only bumps the generation again, so it can make a subject out of date
// but never current.

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ChangedSubject is one subject whose inputs moved: a deal, or the workspace
// as a whole under the nil id.
type ChangedSubject struct {
	Scope InputScope
	ID    ids.UUID
}

// DealChanged names one deal whose inputs moved.
func DealChanged(id ids.UUID) ChangedSubject { return ChangedSubject{Scope: ScopeDeal, ID: id} }

// WorkspaceChanged names a change that reaches every deal.
func WorkspaceChanged() ChangedSubject { return ChangedSubject{Scope: ScopeWorkspace, ID: ids.Nil} }

// readThroughSQL is the highest generation any run read its deals past. Zero
// before the first such run, so every change is unread.
const readThroughSQL = `(SELECT coalesce(max(read_through_generation), 0) FROM assurance_run)`

// MarkInputsChanged records that each subject's inputs moved, in one
// statement however many subjects there are. A subject named twice is one row.
func (s *Store) MarkInputsChanged(ctx context.Context, tx pgx.Tx, subjects []ChangedSubject) error {
	if err := auth.Require(ctx, "forecast", principal.ActionCreate); err != nil {
		return err
	}
	kinds, subjectIDs := distinctSubjects(subjects)
	if len(kinds) == 0 {
		return nil
	}
	// GREATEST so a row never moves to a lower generation, whatever order two
	// markers commit in.
	if _, err := tx.Exec(ctx, `
		INSERT INTO assurance_input_change (subject_kind, subject_id, generation)
		SELECT kind, subject_id, nextval('assurance_input_generation_seq')
		  FROM unnest($1::text[], $2::uuid[]) AS changed(kind, subject_id)
		ON CONFLICT (subject_kind, subject_id) DO UPDATE
		SET generation = GREATEST(assurance_input_change.generation, EXCLUDED.generation),
		    changed_at = now(),
		    updated_at = now()`, kinds, subjectIDs); err != nil {
		return fmt.Errorf("assurance: recording which inputs changed: %w", err)
	}
	return nil
}

// distinctSubjects is the subjects as two parallel columns, each once and in
// one order. The order keeps two consumers marking the same deals from
// deadlocking on each other's rows.
func distinctSubjects(subjects []ChangedSubject) ([]string, []ids.UUID) {
	sorted := slices.Clone(subjects)
	slices.SortFunc(sorted, func(a, b ChangedSubject) int {
		return cmp.Or(cmp.Compare(a.Scope, b.Scope), cmp.Compare(a.ID.String(), b.ID.String()))
	})
	sorted = slices.Compact(sorted)
	kinds := make([]string, 0, len(sorted))
	subjectIDs := make([]ids.UUID, 0, len(sorted))
	for _, subject := range sorted {
		kinds = append(kinds, string(subject.Scope))
		subjectIDs = append(subjectIDs, subject.ID)
	}
	return kinds, subjectIDs
}

// inputGeneration is the highest generation visible now. Scan reads it before
// the deals, which is what makes it safe to clear up to.
func (s *Store) inputGeneration(ctx context.Context, tx pgx.Tx) (int64, error) {
	var generation int64
	if err := tx.QueryRow(ctx,
		`SELECT coalesce(max(generation), 0) FROM assurance_input_change`).Scan(&generation); err != nil {
		return 0, fmt.Errorf("assurance: reading the input generation: %w", err)
	}
	return generation, nil
}

// recordReadThrough says the run read its deals past generation. Only a run
// that read them calls it; a failed read clears nothing.
func (s *Store) recordReadThrough(ctx context.Context, tx pgx.Tx, runID ids.UUID, generation int64) error {
	if _, err := tx.Exec(ctx, `
		UPDATE assurance_run SET read_through_generation = $2 WHERE id = $1`, runID, generation); err != nil {
		return fmt.Errorf("assurance: recording what the run read: %w", err)
	}
	return nil
}

// Freshness is how much of the last reading the records have moved past.
type Freshness struct {
	// ChangedDeals counts deals whose inputs changed after the reading.
	ChangedDeals int
	// WorkspaceChanged is a change that reaches every deal.
	WorkspaceChanged bool
	// FirstChangedAt is the oldest unread change, nil when there is none.
	FirstChangedAt *time.Time
}

// OutOfDate reports whether any input moved after the reading.
func (f Freshness) OutOfDate() bool { return f.ChangedDeals > 0 || f.WorkspaceChanged }

func readFreshness(ctx context.Context, tx pgx.Tx) (Freshness, error) {
	var out Freshness
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE subject_kind = 'deal'),
		       coalesce(bool_or(subject_kind = 'workspace'), false),
		       min(changed_at)
		  FROM assurance_input_change
		 WHERE generation > `+readThroughSQL).
		Scan(&out.ChangedDeals, &out.WorkspaceChanged, &out.FirstChangedAt); err != nil {
		return Freshness{}, fmt.Errorf("assurance: reading what changed since the check: %w", err)
	}
	return out, nil
}

// MarkOutOfDate sets InputsChanged on each finding whose deal, or the
// workspace, changed after the reading, in one read for the whole list.
//
// It takes findings its caller already read under the caller's own scope, and
// answers only about those, so it adds no gate of its own.
func MarkOutOfDate(ctx context.Context, tx pgx.Tx, found []Exception) error {
	if len(found) == 0 {
		return nil
	}
	dealIDs := make([]ids.UUID, 0, len(found))
	for _, e := range found {
		if e.SubjectKind == subjectKindDeal {
			dealIDs = append(dealIDs, e.SubjectID)
		}
	}
	rows, err := tx.Query(ctx, `
		SELECT subject_kind, subject_id
		  FROM assurance_input_change
		 WHERE generation > `+readThroughSQL+`
		   AND (subject_kind = 'workspace' OR (subject_kind = 'deal' AND subject_id = ANY($1)))`, dealIDs)
	if err != nil {
		return fmt.Errorf("assurance: reading which findings are out of date: %w", err)
	}
	changed := map[ChangedSubject]bool{}
	var kind string
	var subjectID ids.UUID
	if _, err := pgx.ForEachRow(rows, []any{&kind, &subjectID}, func() error {
		changed[ChangedSubject{Scope: InputScope(kind), ID: subjectID}] = true
		return nil
	}); err != nil {
		return fmt.Errorf("assurance: reading which findings are out of date: %w", err)
	}
	everything := changed[WorkspaceChanged()]
	for i := range found {
		found[i].InputsChanged = everything ||
			(found[i].SubjectKind == subjectKindDeal && changed[DealChanged(found[i].SubjectID)])
	}
	return nil
}
