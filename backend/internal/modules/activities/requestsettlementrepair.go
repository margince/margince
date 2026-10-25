// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SettlementRepairVersion names the legacy settlement rule a reviewed repair
// applies, so a preview taken under another rule cannot be activated.
const SettlementRepairVersion = "1"

// SettlementRepairCategory is what a reviewed repair may do with one settled
// verdict that recorded no evidence.
type SettlementRepairCategory string

const (
	// SettlementRevalidate is a verdict the settlement pass wrote, over a
	// request still current at its original age, with no human work on its
	// reminder. The repair asks the pass to judge it again.
	SettlementRevalidate SettlementRepairCategory = "revalidate"
	// SettlementProvenanceUnknown has no audit row showing the pass wrote it.
	SettlementProvenanceUnknown SettlementRepairCategory = "provenance_unknown"
	// SettlementHumanWork has a reminder a human wrote to; it is kept.
	SettlementHumanWork SettlementRepairCategory = "human_work"
	// SettlementPastHorizon is older than the waiting horizon; it stays as
	// record information.
	SettlementPastHorizon SettlementRepairCategory = "past_horizon"
	// SettlementNoAnswers has no answer the pass may read now, so judging it
	// again could not run; it is kept for a colleague to review.
	SettlementNoAnswers SettlementRepairCategory = "no_usable_answers"
	// SettlementPending was asked to be judged again and has not been yet.
	SettlementPending SettlementRepairCategory = "revalidation_pending"
	// SettlementUnavailable is a request under a hold; nothing about it is shown.
	SettlementUnavailable SettlementRepairCategory = "unavailable"
	// SettlementChanged was judged again or removed since the page was listed.
	SettlementChanged SettlementRepairCategory = "changed"
)

// SettlementRepairItem is one legacy settled verdict as a repair preview sees it.
type SettlementRepairItem struct {
	RequestID ids.UUID
	Version   int64
	Snapshot  string
	Category  SettlementRepairCategory
}

// legacySettledSQL is a settled verdict recorded before verdicts named their
// evidence, including one asked to be judged again and still waiting. It is
// selected from the verdict table itself, never through the outstanding-request
// query that hides it.
const legacySettledSQL = `s.verdict = '` + RequestSettled + `' AND s.judged_evidence IS NULL`

// SettlementRepairPageTx reads one page of legacy settled verdicts, after the
// given request id, with the category each falls in as of asOf.
func SettlementRepairPageTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, asOf time.Time) ([]SettlementRepairItem, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT s.request_activity_id FROM activity_request_settlement s
		 WHERE `+legacySettledSQL+` AND s.request_activity_id > $1
		 ORDER BY s.request_activity_id LIMIT $2`, after, limit)
	if err != nil {
		return nil, fmt.Errorf("activities: reading legacy settled verdicts: %w", err)
	}
	requests, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("activities: reading legacy settled verdicts: %w", err)
	}
	out := make([]SettlementRepairItem, 0, len(requests))
	for _, request := range requests {
		item, err := settlementRepairItemTx(ctx, tx, request, asOf, false)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

// settlementRepairItemTx classifies one legacy verdict from recorded facts.
// They are the audit row of the pass's own write, human writes on the
// reminder and the request's original age.
func settlementRepairItemTx(ctx context.Context, tx pgx.Tx, request ids.UUID, asOf time.Time, lock bool) (SettlementRepairItem, error) {
	cutoff, err := requestReminderCutoff(ctx, tx, asOf)
	if err != nil {
		return SettlementRepairItem{}, err
	}
	query := `SELECT a.version, coalesce((SELECT sum(r.version) FROM activity r
	       WHERE r.source_system = $4 AND r.source_activity_id = a.id), 0),
	     s.judged_through_activity_id::text || '|' || s.decided_at::text || '|' || s.decided_by
	       || '|' || coalesce(s.revalidation_requested_at::text, ''),
	     a.restricted_at IS NOT NULL, s.revalidation_requested_at IS NOT NULL,
	     EXISTS (SELECT 1 FROM audit_log written WHERE written.entity_type = 'activity'
	       AND written.entity_id = a.id AND written.actor_type = 'system' AND written.actor_id = $5
	       AND written.after->>'` + settlementVerdictCol + `' = '` + RequestSettled + `'
	       AND written.after->>'` + settlementThroughCol + `' = s.judged_through_activity_id::text),
	     EXISTS (SELECT 1 FROM activity r WHERE r.source_system = $4 AND r.source_activity_id = a.id
	       AND ` + storekit.HumanWroteActivityClause("r") + `),
	     ` + occurredday.FromSQL("a", "$3") + `,
	     a.archived_at IS NULL AND a.audience = 'workspace' AND a.restricted_at IS NULL
	       AND EXISTS (` + settlementAnswersSQL() + `)
	   FROM activity_request_settlement s JOIN activity a ON a.id = s.request_activity_id
	  WHERE s.request_activity_id = $2 AND ` + legacySettledSQL
	if lock {
		query += ` FOR UPDATE OF s`
	}
	var version, reminders int64
	var verdict string
	var held, pending, machine, human, current, answered bool
	err = tx.QueryRow(ctx, query, asOf, request, cutoff, EmailRequestTaskSource, RequestSettlementActor).
		Scan(&version, &reminders, &verdict, &held, &pending, &machine, &human, &current, &answered)
	if errors.Is(err, pgx.ErrNoRows) {
		return SettlementRepairItem{RequestID: request, Category: SettlementChanged}, nil
	}
	if err != nil {
		return SettlementRepairItem{}, fmt.Errorf("activities: classifying a legacy settled verdict: %w", err)
	}
	if held {
		return SettlementRepairItem{RequestID: request, Category: SettlementUnavailable}, nil
	}
	sum := sha256.Sum256(fmt.Appendf(nil, "%d|%d|%s", version, reminders, verdict))
	item := SettlementRepairItem{RequestID: request, Version: version, Snapshot: hex.EncodeToString(sum[:])}
	item.Category = settlementFacts{pending: pending, machine: machine, human: human, current: current, answered: answered}.category()
	return item, nil
}

// settlementFacts are the recorded facts a legacy verdict is classified by.
type settlementFacts struct {
	pending, machine, human, current, answered bool
}

// category is the first of the reasons, in order, that decides a legacy
// verdict: pending work, missing proof, human work, age, no evidence.
func (f settlementFacts) category() SettlementRepairCategory {
	switch {
	case f.pending:
		return SettlementPending
	case !f.machine:
		return SettlementProvenanceUnknown
	case f.human:
		return SettlementHumanWork
	case !f.current:
		return SettlementPastHorizon
	case !f.answered:
		return SettlementNoAnswers
	default:
		return SettlementRevalidate
	}
}

// SettlementRepairCurrentTx re-reads one previewed verdict under its lock.
func SettlementRepairCurrentTx(ctx context.Context, tx pgx.Tx, request ids.UUID, asOf time.Time) (SettlementRepairItem, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return SettlementRepairItem{}, err
	}
	return settlementRepairItemTx(ctx, tx, request, asOf, true)
}

// RequestSettlementRevalidationTx asks the pass to judge one previewed
// verdict again, when it is still what the preview showed. It writes
// nothing else: the pass's next verdict decides, and reopens only a reminder
// the pass itself completed.
func RequestSettlementRevalidationTx(ctx context.Context, tx pgx.Tx, expected SettlementRepairItem, asOf time.Time) (bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return false, err
	}
	now, err := settlementRepairItemTx(ctx, tx, expected.RequestID, asOf, true)
	if err != nil {
		return false, err
	}
	if now.Category != SettlementRevalidate || now.Snapshot != expected.Snapshot {
		return false, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE activity_request_settlement SET revalidation_requested_at = $2
		 WHERE request_activity_id = $1`, expected.RequestID, asOf); err != nil {
		return false, fmt.Errorf("activities: asking for a verdict to be judged again: %w", err)
	}
	_, err = storekit.Audit(ctx, tx, "update", "activity", expected.RequestID,
		map[string]any{settlementRevalidationCol: nil},
		map[string]any{settlementRevalidationCol: asOf})
	return err == nil, err
}
