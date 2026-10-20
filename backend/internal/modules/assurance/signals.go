// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const subjectKindSignal = "signal"

// TypeUnlinkedSignal names a buying request that still needs qualification or a deal.
const TypeUnlinkedSignal = "unlinked_signal"

// SignalSubject carries verified references, never a copy of protected source text.
type SignalSubject struct {
	SignalID      string
	Owner         string
	CompanyID     string
	SuggestionID  string
	OccurredAt    time.Time
	Historical    bool
	BuyerVerified bool
}

// SignalSubjectsFunc supplies eligibility facts without protected message text.
type SignalSubjectsFunc func(context.Context, pgx.Tx, time.Time) ([]SignalSubject, error)

// WithSignalSubjects shares signal admission between preview and scan.
func (s *Scanner) WithSignalSubjects(subjects SignalSubjectsFunc) *Scanner {
	wired := *s
	wired.signals = subjects
	return &wired
}

func (s *Scanner) askSignals(ctx context.Context, tx pgx.Tx, now time.Time, pass *walked) (int, error) {
	if s.signals == nil {
		return 0, nil
	}
	subjects, err := s.signals(ctx, tx, now)
	if err != nil {
		return 0, err
	}
	for _, subject := range subjects {
		finding := Finding{
			Type: TypeUnlinkedSignal, SubjectKind: subjectKindSignal, SubjectID: subject.SignalID,
			Severity: SeverityLow,
			Claim:    map[string]any{"company_id": subject.CompanyID},
			Observed: map[string]any{"occurred_at": subject.OccurredAt, "historical": subject.Historical, "buyer_verified": subject.BuyerVerified},
		}
		if subject.SuggestionID != "" {
			finding.Observed["suggestion_id"] = subject.SuggestionID
		}
		pass.raised = append(pass.raised, raised{finding: finding, owner: subject.Owner})
		pass.seen = append(pass.seen, LogicalKey(finding))
	}
	return len(subjects), nil
}

func ensureSignalAnswerable(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	if err := auth.Require(ctx, subjectKindSignal, principal.ActionUpdate); err != nil {
		return err
	}
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return err
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return err
	}
	if err := auth.EnsureSignalVisibleLive(ctx, tx, id); err != nil {
		return err
	}
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	content, err := auth.ActivityContentClause(ctx, "ca", arg)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(`SELECT NOT EXISTS (
  SELECT 1 FROM signal s CROSS JOIN LATERAL jsonb_array_elements(s.evidence) cite
  WHERE s.id = $%d AND cite->>'source_type' = 'activity'
  AND NOT EXISTS (SELECT 1 FROM activity ca WHERE ca.id = `+storekit.CitedActivityID("cite")+`
   AND ca.archived_at IS NULL AND `+content+`))`, arg(id))
	var visible bool
	if err := tx.QueryRow(ctx, query, args...).Scan(&visible); err != nil {
		return err
	}
	if !visible {
		return apperrors.ErrNotFound
	}
	return nil
}

func (s *Store) closeAbsentSignals(ctx context.Context, tx pgx.Tx, seen []string) (int64, error) {
	if err := auth.Require(ctx, "forecast", principal.ActionUpdate); err != nil {
		return 0, err
	}
	if seen == nil {
		seen = []string{}
	}
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	query := fmt.Sprintf(`UPDATE assurance_exception SET status = $%d, updated_at = now()
 WHERE subject_kind = 'signal' AND type = $%d AND status = 'open'
 AND NOT (logical_key = ANY($%d))`, arg(ExceptionConditionCleared), arg(TypeUnlinkedSignal), arg(seen))
	tag, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("assurance: clearing absent signal findings: %w", err)
	}
	return tag.RowsAffected(), nil
}
