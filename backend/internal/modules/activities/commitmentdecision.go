// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RecordCommitmentAdmissionTx replaces one machine decision without changing the source or human work.
func RecordCommitmentAdmissionTx(ctx context.Context, tx pgx.Tx, source ids.ActivityID, decision crmcontracts.CommitmentAdmission) error {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return err
	}
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, source.UUID); err != nil {
		return err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	var before json.RawMessage
	if err := tx.QueryRow(ctx, `SELECT jsonb_build_object('commitment_admission',
  (SELECT jsonb_build_object('locator', d.locator, 'outcome', d.outcome, 'current_work', d.current_work,
 'rule_version', d.rule_version, 'evidence_ids', d.evidence_ids, 'evaluated_at', d.evaluated_at, 'renewed_by', d.renewed_by, 'renewed_at', d.renewed_at,
 'meaning_state', d.meaning->'state', 'deadline_precision', d.meaning->'deadline_precision')
 FROM activity_commitment_admission d WHERE d.source_activity_id = a.id AND d.locator = $2))
  FROM activity a WHERE a.id = $1 FOR UPDATE OF a`, source, decision.Locator).Scan(&before); err != nil {
		return fmt.Errorf("activities: locking prior commitment admission: %w", err)
	}
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	query := fmt.Sprintf(`INSERT INTO activity_commitment_admission AS prior
 (source_activity_id, locator, summary, source_quote, outcome, current_work, rule_version, evidence_ids, evaluated_at, captured_by, renewed_by, renewed_at, meaning)
 VALUES ($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)
 ON CONFLICT (source_activity_id, locator) DO UPDATE SET
 summary = EXCLUDED.summary, source_quote = EXCLUDED.source_quote, outcome = EXCLUDED.outcome, current_work = EXCLUDED.current_work,
 rule_version = EXCLUDED.rule_version, evidence_ids = EXCLUDED.evidence_ids,
 evaluated_at = EXCLUDED.evaluated_at, captured_by = EXCLUDED.captured_by,
 renewed_by = EXCLUDED.renewed_by, renewed_at = EXCLUDED.renewed_at, meaning = EXCLUDED.meaning
 WHERE prior.outcome <> $%d AND prior.evaluated_at <= EXCLUDED.evaluated_at AND
 (prior.summary, prior.source_quote, prior.outcome, prior.current_work, prior.rule_version, prior.evidence_ids, prior.evaluated_at, prior.renewed_by, prior.renewed_at, prior.meaning)
 IS DISTINCT FROM
 (EXCLUDED.summary, EXCLUDED.source_quote, EXCLUDED.outcome, EXCLUDED.current_work, EXCLUDED.rule_version, EXCLUDED.evidence_ids, EXCLUDED.evaluated_at, EXCLUDED.renewed_by, EXCLUDED.renewed_at, EXCLUDED.meaning)`,
		arg(source), arg(decision.Locator), arg(decision.Summary), arg(decision.SourceQuote), arg(decision.Outcome), arg(decision.CurrentWork),
		arg(decision.RuleVersion), arg(decision.EvidenceIds), arg(decision.EvaluatedAt), arg(by), arg(decision.RenewedBy), arg(decision.RenewedAt), arg(decision.Meaning), arg(CommitmentSourceUnavailable))
	tag, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("activities: recording commitment admission: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	// Source words stay in the erasable decision; audits retain only its classifications.
	var meaningState, deadlinePrecision *string
	if decision.Meaning != nil {
		meaningState, deadlinePrecision = &decision.Meaning.State, &decision.Meaning.DeadlinePrecision
	}
	auditID, err := storekit.Audit(ctx, tx, "update", "activity", source.UUID, before, map[string]any{"commitment_admission": map[string]any{
		"locator": decision.Locator, "outcome": decision.Outcome, "current_work": decision.CurrentWork,
		"meaning_state": meaningState, "deadline_precision": deadlinePrecision,
		"rule_version": decision.RuleVersion, "evidence_ids": decision.EvidenceIds, "evaluated_at": decision.EvaluatedAt, "renewed_by": decision.RenewedBy, "renewed_at": decision.RenewedAt,
	}})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, source.UUID, crmcontracts.PublicEventActivityUpdated{ChangedFields: crmcontracts.PublicEventActivityChangedFields{CommitmentAdmissionsChanged: new(true)}})
}

// WithCommitmentAdmissions attaches review evidence through the source's content boundary.
func WithCommitmentAdmissions(ctx context.Context, tx pgx.Tx, list []crmcontracts.Activity) error {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return err
	}
	positions := map[ids.UUID]int{}
	sources := []ids.UUID{}
	for i := range list {
		list[i].CommitmentAdmissions = nil
		if list[i].ContentState != nil && *list[i].ContentState == crmcontracts.ActivityContentStateWithheld {
			continue
		}
		id := ids.UUID(list[i].Id)
		positions[id] = i
		sources = append(sources, id)
	}
	if len(sources) == 0 {
		return nil
	}
	args := []any{}
	arg := func(value any) int { args = append(args, value); return len(args) }
	content, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return err
	}
	renewal, err := auth.CommitmentEvidenceClause(ctx, "d.source_activity_id", "d.locator", arg)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(`SELECT d.source_activity_id, d.locator, d.summary, d.source_quote, d.outcome, d.current_work, d.rule_version, d.evidence_ids, d.evaluated_at, d.renewed_by, d.renewed_at, d.meaning
 FROM activity_commitment_admission d JOIN activity a ON a.id = d.source_activity_id
 WHERE a.id = ANY($%d::uuid[]) AND (d.summary <> '' OR d.source_quote <> '') AND `+content+` AND `+renewal+` ORDER BY d.source_activity_id, d.locator`, arg(sources))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("activities: reading commitment admissions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var source ids.UUID
		var decision crmcontracts.CommitmentAdmission
		if err := rows.Scan(&source, &decision.Locator, &decision.Summary, &decision.SourceQuote, &decision.Outcome, &decision.CurrentWork, &decision.RuleVersion, &decision.EvidenceIds, &decision.EvaluatedAt, &decision.RenewedBy, &decision.RenewedAt, &decision.Meaning); err != nil {
			return fmt.Errorf("activities: reading commitment admission: %w", err)
		}
		i, ok := positions[source]
		if !ok {
			continue
		}
		if list[i].CommitmentAdmissions == nil {
			list[i].CommitmentAdmissions = new([]crmcontracts.CommitmentAdmission)
		}
		*list[i].CommitmentAdmissions = append(*list[i].CommitmentAdmissions, decision)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("activities: reading commitment admissions: %w", err)
	}
	return nil
}

// CommitmentAdmissionStateTx distinguishes prior decisions from work created before admission tracking.
func CommitmentAdmissionStateTx(ctx context.Context, tx pgx.Tx, source ids.ActivityID, locator string, asOf time.Time, renewedBy *ids.UUID, renewedAt *time.Time) (held, recorded bool, err error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return false, false, err
	}
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, source.UUID); err != nil {
		return false, false, err
	}
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	query := fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM activity_commitment_admission
 WHERE source_activity_id = $%[1]d AND locator = $%[2]d AND
 (outcome = $%[6]d OR evaluated_at > $%[3]d OR (current_work AND $%[4]d::uuid IS NULL)
 OR (renewed_at IS NOT NULL AND ($%[5]d::timestamptz IS NULL
 OR (renewed_at, renewed_by) > ($%[5]d, $%[4]d)
 OR (current_work AND renewed_by = $%[4]d))))),
 EXISTS (SELECT 1 FROM activity_commitment_admission WHERE source_activity_id = $%[1]d AND locator = $%[2]d)`,
		arg(source), arg(locator), arg(asOf), arg(renewedBy), arg(renewedAt), arg(CommitmentSourceUnavailable))
	err = tx.QueryRow(ctx, query, args...).Scan(&held, &recorded)
	if err != nil {
		return false, false, fmt.Errorf("activities: checking prior commitment admission: %w", err)
	}
	return held, recorded, nil
}

// CommitmentStillOpenTx is whether the recorded decision for one promise
// still makes it open, current work. A staged proposal that names no claim
// rests on this decision. One with no decision on record has no basis to
// rebuild, so it is withdrawn rather than mailed or accepted.
func CommitmentStillOpenTx(ctx context.Context, tx pgx.Tx, source ids.UUID, locator string) (bool, error) {
	_, open, err := readCommitmentAdmissionTx(ctx, tx, source, locator, "")
	return open, err
}

// CommitmentAdmissionTx reads that decision as a delayed send's basis: a
// fingerprint of the whole row, which any re-evaluation changes, and whether
// it is still open.
func CommitmentAdmissionTx(ctx context.Context, tx pgx.Tx, source ids.UUID, locator string) (int64, bool, error) {
	return readCommitmentAdmissionTx(ctx, tx, source, locator, "")
}

// HoldCommitmentAdmissionTx is CommitmentAdmissionTx under a share lock, so a
// re-evaluation waits for the send's reservation to commit.
func HoldCommitmentAdmissionTx(ctx context.Context, tx pgx.Tx, source ids.UUID, locator string) (int64, bool, error) {
	return readCommitmentAdmissionTx(ctx, tx, source, locator, " FOR SHARE")
}

func readCommitmentAdmissionTx(ctx context.Context, tx pgx.Tx, source ids.UUID, locator, lock string) (int64, bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var fingerprint int64
	var open bool
	err := tx.QueryRow(ctx, `SELECT hashtextextended(adm::text, 0), adm.current_work AND coalesce(adm.meaning->>'state', 'open') = 'open'
		  FROM activity_commitment_admission adm WHERE adm.source_activity_id = $1 AND adm.locator = $2`+lock,
		source, locator).Scan(&fingerprint, &open)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("activities: reading whether a promise is still open: %w", err)
	}
	return fingerprint, open, nil
}
