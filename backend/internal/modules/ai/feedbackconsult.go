// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Consulting the ledger for MANY records at once, and the scan both reads
// share.
//
// Its own file because feedback.go sits at its length ceiling; the per-record
// read stays there, beside the write it answers for, rather than moving house
// to make room.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// VerdictsForManyTx returns every verdict recorded about each of several
// records of one kind, keyed by subject and then by "<claim_kind>:<claim_key>".
//
// One read for the whole batch. A sweep asks about every record it is about to
// judge, and asking per record would be a query per row of a job that exists to
// process many — the same argument VerdictsForTx makes per claim, one level up.
// The ruling itself is not restated here: both land in Verdict, and a caller
// decides through Verdict.AsOf exactly as a page does.
func (s *FeedbackStore) VerdictsForManyTx(ctx context.Context, tx pgx.Tx,
	subjectType string, subjectIDs []ids.UUID,
) (map[ids.UUID]map[string]Verdict, error) {
	if err := admitSubject(subjectType); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, subjectType, principal.ActionRead); err != nil {
		return nil, err
	}
	out := map[ids.UUID]map[string]Verdict{}
	if len(subjectIDs) == 0 {
		return out, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT subject_id, claim_kind, claim_key, verdict, corrected_value, note,
		       updated_at, value_captured_at, value_shown
		  FROM ai_feedback
		 WHERE subject_type = $1 AND subject_id = ANY($2)`, subjectType, subjectIDs)
	if err != nil {
		return nil, fmt.Errorf("ai: reading the verdicts recorded about several records: %w", err)
	}
	defer rows.Close()

	if err := scanVerdicts(rows, withSubject, func(subject ids.UUID, v Verdict) {
		byClaim, ok := out[subject]
		if !ok {
			byClaim = map[string]Verdict{}
			out[subject] = byClaim
		}
		byClaim[VerdictLookupKey(v.ClaimKind, v.ClaimKey)] = v
	}); err != nil {
		return nil, err
	}
	return out, nil
}

// admitSubject refuses a subject kind the ledger does not keep claims about.
// Named because three entry points ask it — the write and both reads — and a
// caller sending an unknown kind should be told the same thing by each.
func admitSubject(subjectType string) error {
	if feedbackSubjects[subjectType] {
		return nil
	}
	return &values.ParseError{
		Field: fieldSubjectType, Code: "invalid_subject_type",
		Message: "a claim is about a company, contact, deal or lead",
	}
}

// Whether a verdict query selected subject_id — stated by the caller rather
// than inferred from the column count, which would make adding a column to
// either query a silent mis-scan.
const (
	withoutSubject = false
	withSubject    = true
)

// scanVerdicts reads the ledger's columns into Verdict values. Both reads go
// through it, which keeps the column list and the meaning of each column beside
// each other rather than beside each query. The subject is handed to the sink
// only when the query selected it; a per-record read already knows whose it is
// and passes the zero id.
func scanVerdicts(rows pgx.Rows, selectsSubject bool, keep func(subject ids.UUID, v Verdict)) error {
	for rows.Next() {
		var subject ids.UUID
		var claimKind, claimKey, verdict string
		var correctedValue, note *string
		var recordedAt time.Time
		var valueCapturedAt *time.Time
		var valueShown *string
		dest := []any{
			&claimKind, &claimKey, &verdict, &correctedValue, &note,
			&recordedAt, &valueCapturedAt, &valueShown,
		}
		if selectsSubject {
			dest = append([]any{&subject}, dest...)
		}
		if err := rows.Scan(dest...); err != nil {
			return fmt.Errorf("ai: reading a recorded verdict: %w", err)
		}
		keep(subject, NewVerdict(
			claimKind, claimKey, verdict, correctedValue, note, recordedAt, valueCapturedAt, valueShown,
		))
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("ai: reading the recorded verdicts: %w", err)
	}
	return nil
}
