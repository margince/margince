// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What the sweep changed on a rep's own deals last night, for the morning's
// "handled for you" lane.
//
// The corrections are applied when they are made — there is no card, and no
// approval row to read them back from — so this is the only telling a rep gets.
// It carries the before and the after, the reason, and the audit row an Undo
// puts back.

package deals

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CorrectionReceipt is one applied correction as a reader sees it.
type CorrectionReceipt struct {
	DealID      ids.DealID
	DealName    string
	BeforeClose *string
	AfterClose  *string
	// AuditLogID is what an Undo names: the record-history restore route reads
	// the before-image from it, which is why nothing here copies those values
	// into a second place.
	AuditLogID ids.UUID
	// Fields the correction moved, so the sentence can name them all rather
	// than the one the reader happens to notice.
	Fields        []string
	EstimateBasis *CorrectionEstimateBasis
	Basis         string
	AppliedAt     time.Time
	// Reversed reports that somebody already put this back. The row stays on
	// the lane saying so, rather than vanishing and leaving the reader
	// wondering whether their Undo landed.
	Reversed bool
	Version  int64
	// Who made the correction, as its audit row records them.
	ActorType  string
	ActorID    string
	OnBehalfOf *ids.UUID
	// Correction is which correction this was: a close-date tier, a close as
	// lost, or a change from the nightly verdict.
	Correction string
	// Before and after values of an amount or probability change. Currency
	// is the one the change was written in, not the deal's currency today.
	BeforeAmount, AfterAmount           *int64
	BeforeProbability, AfterProbability *int
	Currency                            *string
	// Evidence is the change's audit evidence, withheld like Basis after the
	// deal changed hands.
	Evidence []byte
	// Accepted reports that the owner kept the change.
	Accepted bool
	OwnerID  *ids.UUID
	// FromStage and ToStage name the stages a stage move or a close went
	// between, as they are called today.
	FromStage, ToStage *string
	// StageProbability is the probability the deal took with the stage it
	// was moved to.
	StageProbability *int
}

// heldCorrections stay on Home until their owner accepts or undoes them.
var heldCorrections = []string{
	CloseLostCorrection, VerdictCloseDateCorrection, VerdictAmountCorrection, VerdictProbabilityCorrection,
	VerdictStageMoveCorrection, DealOpenCorrection, DealReviveCorrection,
}

// RecentCorrectionsOwnedBy reads what the sweep changed on THIS reader's deals
// since an instant, newest first.
//
// Narrowed to their own deals, not to every deal they may see. A manager who
// can read the whole pipeline does not want every rep's overnight hygiene in
// their morning — the receipt answers "what happened to my work while I was
// away", and a lane that answered it for the workspace would be unreadable on
// any installation with more than a few reps.
func (s *Store) RecentCorrectionsOwnedBy(
	ctx context.Context, since time.Time, limit int,
) ([]CorrectionReceipt, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		// No seat, no "my deals" to answer for. An empty lane rather than a
		// refusal: a background assembly with no reader is a real caller.
		return nil, nil
	}
	var out []CorrectionReceipt
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		args := []any{}
		arg := func(v any) int { args = append(args, v); return len(args) }
		sincePos := arg(since)
		heldPos := arg(heldCorrections)
		ownerPos := arg(actor.UserID)
		limitPos := arg(limit)
		// The reader's own row scope, ON TOP of the owner narrowing above.
		//
		// The two answer different questions and neither implies the other: the
		// owner clause is the product rule (a receipt is about MY deals), and
		// the scope clause is the permission one (which deals this seat may see
		// at all). A seat whose scope was narrowed below its own deals must not
		// read them back through this lane, and asking only the product
		// question would let it.
		scope, err := auth.ScopeClauseFor(ctx, dealTable, "d", arg)
		if err != nil {
			return err
		}
		selected, err := correctionReceiptSelect(ctx, arg)
		if err != nil {
			return err
		}
		query := selected + storekit.SQLf(`
			 -- A close as lost and a verdict change stay until the owner accepts or undoes them.
			 WHERE (c.applied_at >= $%d
			        OR (c.correction = ANY($%d) AND c.accepted_at IS NULL AND c.reversed_at IS NULL))
			   AND d.owner_id = $%d
			   AND d.archived_at IS NULL`, sincePos, heldPos, ownerPos)
		if scope != "" {
			query += storekit.SQLf(" AND %s", scope)
		}
		query += storekit.SQLf(" ORDER BY c.applied_at DESC LIMIT $%d", limitPos)
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("deals: reading last night's corrections: %w", err)
		}
		out, err = pgx.CollectRows(rows, scanCorrectionReceipt)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func scanCorrectionReceipt(rows pgx.CollectableRow) (CorrectionReceipt, error) {
	var row CorrectionReceipt
	if err := rows.Scan(&row.DealID, &row.DealName, &row.AuditLogID,
		&row.Fields, &row.Basis, &row.AppliedAt, &row.Reversed,
		&row.Version, &row.BeforeClose, &row.AfterClose, &row.Evidence,
		&row.ActorType, &row.ActorID, &row.OnBehalfOf, &row.Correction,
		&row.BeforeAmount, &row.AfterAmount, &row.BeforeProbability, &row.AfterProbability,
		&row.Currency, &row.Accepted, &row.OwnerID, &row.FromStage, &row.ToStage,
		&row.StageProbability); err != nil {
		return row, err
	}
	row.EstimateBasis = correctionEstimateBasis(row.Evidence)
	return row, nil
}

// correctionReceiptSelect reads one correction the way every reader shows it.
// It binds the reader through arg. The reason and evidence reach only the
// deal's owner whose grants composed them. The amounts follow the reader's
// field mask. Each reader adds its own `WHERE` and `ORDER BY`.
func correctionReceiptSelect(ctx context.Context, arg func(any) int) (string, error) {
	readable := basisReadable(arg(readerOf(ctx)))
	amountShown, masked, err := auth.MaskExcludedClause(ctx, maskObject, "amount_minor", "d", arg)
	if err != nil {
		return "", err
	}
	if !masked {
		amountShown = "TRUE"
	}
	before, err := auth.MaskedExpressionSQL(ctx, maskObject, "amount_minor", "d", "(a.before->>'amount_minor')::bigint", arg)
	if err != nil {
		return "", err
	}
	after, err := auth.MaskedExpressionSQL(ctx, maskObject, "amount_minor", "d", "(a.after->>'amount_minor')::bigint", arg)
	if err != nil {
		return "", err
	}
	return storekit.SQLf(`
			SELECT c.deal_id, d.name, c.audit_log_id, c.fields,
			       -- The reason, only for the seat whose grants composed it.
			       --
			       -- The sentence can name a contact and a correspondence date,
			       -- read under the owner's permissions on the night it was
			       -- written, and it is stored text by the time anybody reads it
			       -- back. A deal that has changed hands since shows the change
			       -- with no reason rather than disclosing a name this reader was
			       -- never entitled to. Compared in SQL so no row leaves the
			       -- database carrying a sentence its reader may not have.
			       --
			       -- IS NOT DISTINCT FROM, never a bare equality. An unowned deal
			       -- records the empty string and owner_id is NULL, and equality
			       -- against NULL is NULL — so that test would withhold the reason on
			       -- every unowned deal, whose sentence names nobody and was
			       -- composed under no seat's grants at all.
			       CASE WHEN %[1]s AND (%[4]s OR c.correction <> $%[5]d)
			            THEN coalesce(a.evidence->>'basis', '')
			            ELSE '' END,
			       c.applied_at,
			       c.reversed_at IS NOT NULL, d.version, a.before->>'expected_close_date', a.after->>'expected_close_date',
			       -- A masked amount takes every key that states it out of the
			       -- evidence: the customer's words, the figure and the rate.
			       CASE WHEN NOT %[1]s THEN '{}'::jsonb
			            WHEN %[4]s THEN a.evidence
			            ELSE a.evidence - ARRAY['quoted_words', 'quoted_amount', 'quoted_currency',
			                                    'amount_currency', 'rate_from', 'rate_to', 'rate_date'] END,
			       a.actor_type, a.actor_id, a.on_behalf_of, c.correction,
			       %[2]s, %[3]s,
			       (a.before->>'win_probability_override')::int, (a.after->>'win_probability_override')::int,
			       coalesce(a.evidence->>'amount_currency', d.currency::text),
			       c.accepted_at IS NOT NULL, d.owner_id,
			       (SELECT st.name FROM stage st WHERE st.id::text = a.before->>'stage_id'),
			       (SELECT st.name FROM stage st WHERE st.id::text = a.after->>'stage_id'),
			       (SELECT h.win_probability_at_change::int FROM deal_stage_history h WHERE h.id = c.stage_history_id)
			  FROM deal_correction c
			  JOIN deal d ON d.id = c.deal_id
			  JOIN audit_log a ON a.id = c.audit_log_id
`, readable, before, after, amountShown, arg(VerdictAmountCorrection)), nil
}

// basisReadable is when the reader may read a correction's reason. The deal
// is unowned, or the reader owns it and the reason was composed for them. A
// colleague with row scope still sees what changed, never why.
func basisReadable(readerPos int) string {
	return storekit.SQLf(`(nullif(a.evidence->>'basis_owner', '') IS NOT DISTINCT FROM d.owner_id::text
			                 AND (d.owner_id IS NULL OR d.owner_id = $%d))`, readerPos)
}
