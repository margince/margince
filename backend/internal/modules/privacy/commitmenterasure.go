// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/retentionscope"
)

type commitmentErasure struct {
	source  ids.UUID
	locator string
	task    *ids.UUID
}

func (e *Eraser) eraseCommitmentDerivatives(ctx context.Context, tx pgx.Tx, sources []ids.UUID, act erasureAct) error {
	if len(sources) == 0 {
		return nil
	}
	work, err := lockCommitmentDerivatives(ctx, tx, sources)
	if err != nil {
		return err
	}
	for _, item := range work {
		if err := e.eraseCommitmentDerivative(ctx, tx, item, act); err != nil {
			return err
		}
	}
	for _, source := range sources {
		if err := eraseCompletedCommitmentBoundary(ctx, tx, source); err != nil {
			return err
		}
	}
	return nil
}

func (e *Eraser) eraseCommitmentDerivative(ctx context.Context, tx pgx.Tx, item commitmentErasure, act erasureAct) error {
	erased := true
	if item.task != nil {
		var err error
		erased, err = e.eraseCommitmentTask(ctx, tx, *item.task, act)
		if err != nil {
			return err
		}
	}
	if err := eraseCommitmentClaims(ctx, tx, item); err != nil {
		return err
	}
	if !erased {
		return redactCommitmentDecision(ctx, tx, item, true)
	}
	return eraseCommitmentDecision(ctx, tx, item)
}

func commitmentDerivatives(ctx context.Context, tx pgx.Tx, sources []ids.UUID, locking bool) ([]commitmentErasure, error) {
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	lock := ""
	if locking {
		lock = " FOR UPDATE OF d"
	}
	query := fmt.Sprintf(`SELECT d.source_activity_id, d.locator, a.id
 FROM activity_commitment_admission d
 LEFT JOIN activity a ON a.source_activity_id = d.source_activity_id
 AND a.source_id = d.locator AND a.source_system = $%[1]d
 WHERE d.source_activity_id = ANY($%[2]d::uuid[]) OR d.renewed_by = ANY($%[2]d::uuid[])
 ORDER BY a.id, d.source_activity_id, d.locator%[3]s`, arg(provenance.CommitmentTaskSource), arg(sources), lock)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("privacy: finding work derived from erased evidence: %w", err)
	}
	work, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (commitmentErasure, error) {
		var item commitmentErasure
		err := row.Scan(&item.source, &item.locator, &item.task)
		return item, err
	})
	return work, err
}

func lockCommitmentDerivatives(ctx context.Context, tx pgx.Tx, sources []ids.UUID) ([]commitmentErasure, error) {
	work, err := commitmentDerivatives(ctx, tx, sources, false)
	if err != nil {
		return nil, err
	}
	legacy, err := legacyCommitmentDerivatives(ctx, tx, sources)
	if err != nil {
		return nil, err
	}
	work = append(work, legacy...)
	slices.SortFunc(work, func(a, b commitmentErasure) int {
		if a.task == nil {
			if b.task == nil {
				return 0
			}
			return -1
		}
		if b.task == nil {
			return 1
		}
		return bytes.Compare(a.task[:], b.task[:])
	})
	for _, item := range work {
		if item.task != nil {
			if _, err := storekit.LockRow(ctx, tx, "activity", *item.task, storekit.IncludeArchived); err != nil {
				return nil, err
			}
		}
	}
	work, err = commitmentDerivatives(ctx, tx, sources, true)
	if err != nil {
		return nil, err
	}
	legacy, err = legacyCommitmentDerivatives(ctx, tx, sources)
	return append(work, legacy...), err
}

func (e *Eraser) eraseCommitmentTask(ctx context.Context, tx pgx.Tx, task ids.UUID, act erasureAct) (bool, error) {
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	var held, alreadyWithheld bool
	query := fmt.Sprintf(`SELECT a.restricted_at IS NOT NULL OR %s, NOT (%s) FROM activity a WHERE a.id = $%d`, retentionscope.HeldThroughAnyLink("a.id"), auth.CommitmentContentRetainedClause("a"), arg(task))
	if err := tx.QueryRow(ctx, query, args...).Scan(&held, &alreadyWithheld); err != nil {
		return false, err
	}
	if held {
		if alreadyWithheld {
			return false, nil
		}
		if err := purgeActivityIndexes(ctx, tx, []ids.UUID{task}); err != nil {
			return false, err
		}
		auditID, err := storekit.AuditEventWithEvidence(ctx, tx, "update", "activity", task, map[string]any{"commitment_evidence_unavailable": true},
			map[string]any{evidenceKeyReason: act.collateralReason, evidenceKeyCause: act.cause})
		if err != nil {
			return false, err
		}
		return false, storekit.EmitEvent(ctx, tx, auditID, task, crmcontracts.PublicEventActivityUpdated{
			ChangedFields: crmcontracts.PublicEventActivityChangedFields{CommitmentAdmissionsChanged: new(true)},
		})
	}
	if err := clearActivityContent(ctx, tx, task); err != nil {
		return false, err
	}
	if err := e.purgeContentDerivedFrom(ctx, tx, task, act); err != nil {
		return false, err
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, actionErase, "activity", task, nil, nil,
		map[string]any{evidenceKeyReason: act.collateralReason, evidenceKeyCause: act.cause})
	if err != nil {
		return false, err
	}
	return true, storekit.EmitEventForEntity(ctx, tx, auditID, "activity", task, retentionAppliedPayload(crmcontracts.RetentionAppliedErase, nil, &act.collateralReason))
}

func eraseCommitmentClaims(ctx context.Context, tx pgx.Tx, item commitmentErasure) error {
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	query := fmt.Sprintf(`DELETE FROM conversation_claim WHERE source_activity_id = $%d
 AND evidence_fingerprint = $%d RETURNING id, contact_id`, arg(item.source), arg(item.locator))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	type erasedClaim struct{ id, contact ids.UUID }
	claims, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (erasedClaim, error) {
		var claim erasedClaim
		err := row.Scan(&claim.id, &claim.contact)
		return claim, err
	})
	if err != nil {
		return err
	}
	for _, claim := range claims {
		auditID, err := storekit.Audit(ctx, tx, "update", "contact", claim.contact, map[string]any{"claim_id": claim.id},
			map[string]any{"claim_id": claim.id, "claim_erased": true})
		if err != nil {
			return err
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, claim.contact, crmcontracts.PublicEventConversationClaimChanged{
			ClaimId: openapi_types.UUID(claim.id), Status: "erased",
		}); err != nil {
			return err
		}
	}
	return nil
}

func eraseCommitmentDecision(ctx context.Context, tx pgx.Tx, item commitmentErasure) error {
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	query := fmt.Sprintf(`DELETE FROM activity_commitment_admission
 WHERE source_activity_id = $%d AND locator = $%d`, arg(item.source), arg(item.locator))
	tag, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	auditID, err := storekit.Audit(ctx, tx, "update", "activity", item.source,
		map[string]any{"commitment_locator": item.locator}, map[string]any{"commitment_admission_erased": item.locator})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, item.source, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: crmcontracts.PublicEventActivityChangedFields{CommitmentAdmissionsChanged: new(true)},
	})
}

// Retained work still needs its provenance to enforce the source's audience.
func redactCommitmentDecision(ctx context.Context, tx pgx.Tx, item commitmentErasure, unavailable bool) error {
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO activity_commitment_admission AS prior
 (source_activity_id, locator, summary, source_quote, outcome, current_work, rule_version, evidence_ids, evaluated_at, captured_by)
 VALUES (@evidence_source, @locator, '', '', CASE WHEN @unavailable THEN @reason ELSE 'retained' END,
 NOT @unavailable, 'erasure', '{}', now(), @by)
 ON CONFLICT (source_activity_id, locator) DO UPDATE SET summary = '', source_quote = '', meaning = NULL,
 outcome = CASE WHEN @unavailable THEN @reason ELSE prior.outcome END,
 current_work = prior.current_work AND NOT @unavailable
 WHERE prior.summary <> '' OR prior.source_quote <> '' OR prior.meaning IS NOT NULL
 OR (@unavailable AND (prior.outcome <> @reason OR prior.current_work))`, pgx.StrictNamedArgs{
		"unavailable": unavailable, evidenceKeyReason: auth.CommitmentEvidenceUnavailable, "evidence_source": item.source, "locator": item.locator, "by": by,
	})
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil
	}

	auditID, err := storekit.Audit(ctx, tx, "update", "activity", item.source,
		map[string]any{"commitment_locator": item.locator}, map[string]any{"commitment_admission_redacted": item.locator})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, item.source, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: crmcontracts.PublicEventActivityChangedFields{CommitmentAdmissionsChanged: new(true)},
	})
}

func eraseCompletedCommitmentBoundary(ctx context.Context, tx pgx.Tx, task ids.UUID) error {
	item, err := unavailableCommitmentForTask(ctx, tx, task)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return eraseCommitmentDecision(ctx, tx, item)
}

func unavailableCommitmentForTask(ctx context.Context, tx pgx.Tx, task ids.UUID) (commitmentErasure, error) {
	item := commitmentErasure{task: &task}
	err := tx.QueryRow(ctx, `SELECT d.source_activity_id, d.locator
 FROM activity_commitment_admission d JOIN activity a
 ON a.source_activity_id = d.source_activity_id AND a.source_id = d.locator
 WHERE a.id = @task AND a.source_system = @task_origin AND d.outcome = @outcome
 FOR UPDATE OF d`, pgx.StrictNamedArgs{
		"task": task, "task_origin": provenance.CommitmentTaskSource, "outcome": auth.CommitmentEvidenceUnavailable,
	}).Scan(&item.source, &item.locator)
	return item, err
}
