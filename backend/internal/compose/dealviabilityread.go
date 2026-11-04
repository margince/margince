// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// GET /deals/{id}/viability: the deal's treatment in the qualified forecast and
// what it rests on. The qualification and its basis come from the same SQL the
// forecast read uses, so the deal page cannot disagree with the forecast.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type dealViabilityHandlers struct {
	pool *pgxpool.Pool
}

// GetDealViability answers whether the customer still supports one deal.
func (h dealViabilityHandlers) GetDealViability(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	ctx := r.Context()
	if err := auth.Require(ctx, "forecast", principal.ActionRead); err != nil {
		httperr.Write(w, r, err)
		return
	}
	deal := ids.UUID(id)
	var out crmcontracts.DealViability
	err := database.WithWorkspaceTx(ctx, h.pool, func(tx pgx.Tx) error {
		var err error
		out, err = readDealViability(ctx, tx, deal)
		return err
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// readDealViability reads one deal's viability in the caller's transaction.
func readDealViability(ctx context.Context, tx pgx.Tx, deal ids.UUID) (crmcontracts.DealViability, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return crmcontracts.DealViability{}, err
	}
	policy, err := forecastQualifiedPolicy(ctx, tx)
	if err != nil {
		return crmcontracts.DealViability{}, err
	}
	zone, err := installationZoneTx(ctx, tx)
	if err != nil {
		return crmcontracts.DealViability{}, err
	}
	args := []any{deal}
	arg := func(v any) int { args = append(args, v); return len(args) }
	// The deal's row scope and the task's discovery scope, in this read's own
	// transaction: a deal outside the scope is no row, so it answers 404.
	dealScope, err := auth.ScopeClauseFor(ctx, tableDeal, "d", arg)
	if err != nil {
		return crmcontracts.DealViability{}, err
	}
	if dealScope == "" {
		dealScope = "true"
	}
	taskScope, err := auth.ActivityDiscoverClause(ctx, "t", arg)
	if err != nil {
		return crmcontracts.DealViability{}, err
	}
	reviewType := arg(assurance.TypeDealViability)
	today := arg(time.Now().In(zone).Format(time.DateOnly))
	sql := fmt.Sprintf(`
		SELECT d.status = 'open', d.expected_close_date, d.close_date_provisional,
		       COALESCE(d.expected_close_date < $%[4]d::date, false), COALESCE(s.win_probability, 0),
		       %[1]s, %[2]s,
		       r.outcome, r.reasons, r.valid_until, r.policy_version,
		       x.id, x.first_seen_at, x.episode, x.delivery_state, t.id, t.is_done
		FROM deal d
		LEFT JOIN stage s ON s.id = d.stage_id
		LEFT JOIN assurance_deal_receipt r ON r.deal_id = d.id
		LEFT JOIN LATERAL (
		  SELECT vx.id, vx.first_seen_at, vx.episode, vx.delivery_state, vx.task_activity_id
		    FROM assurance_exception vx
		   WHERE vx.subject_kind = 'deal' AND vx.subject_id = d.id AND vx.type = $%[3]d AND vx.status = 'open'
		   ORDER BY vx.first_seen_at DESC LIMIT 1) x ON true
		LEFT JOIN activity t ON t.id = x.task_activity_id AND t.archived_at IS NULL AND %[5]s
		      AND EXISTS (SELECT 1 FROM activity_link tl WHERE tl.activity_id = t.id AND tl.deal_id = d.id)
		WHERE d.id = $1 AND d.archived_at IS NULL AND %[6]s`,
		viabilityQualificationSQL("d", arg), viabilityBasisSQL("d", arg), reviewType, today, taskScope, dealScope)
	var v dealViabilityRow
	err = tx.QueryRow(ctx, sql, args...).Scan(
		&v.open, &v.close, &v.provisional, &v.overdue, &v.probability,
		&v.qualification, &v.policy, &v.assessed, &v.disposition, &v.complete,
		&v.outcome, &v.reasons, &v.validUntil, &v.receiptPolicy,
		&v.review, &v.opened, &v.episode, &v.delivery, &v.task, &v.done,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return crmcontracts.DealViability{}, apperrors.ErrNotFound
	}
	if err != nil {
		return crmcontracts.DealViability{}, fmt.Errorf("compose: reading a deal's viability: %w", err)
	}
	return v.wire(deal, policy), nil
}

// dealViabilityRow is the read's one row, nullable where the deal has no
// receipt, no open review or no task.
type dealViabilityRow struct {
	close                                *time.Time
	open, provisional, overdue, complete bool
	probability                          int
	qualification, policy                string
	disposition                          string
	assessed, validUntil, opened         *time.Time
	outcome, delivery, receiptPolicy     *string
	reasons                              []string
	review, task                         *ids.UUID
	episode                              *int
	done                                 *bool
}

func (v dealViabilityRow) wire(deal ids.UUID, enforced string) crmcontracts.DealViability {
	mode := crmcontracts.DealViabilityQualificationModeOff
	if enforced != "" {
		mode = crmcontracts.DealViabilityQualificationModeEnforcing
	}
	out := crmcontracts.DealViability{
		DealId: openapi_types.UUID(deal), QualificationMode: mode,
		Qualification: v.standing(),
		AssessedAt:    v.assessed, Complete: &v.complete, PolicyVersion: &v.policy,
		Disposition:          (*crmcontracts.DealViabilityDisposition)(&v.disposition),
		CloseDateProvisional: v.provisional, CloseDateOverdue: v.overdue, StageProbability: v.probability,
	}
	if v.close != nil {
		out.ExpectedCloseDate = &openapi_types.Date{Time: *v.close}
	}
	if v.outcome != nil && v.validUntil != nil && v.receiptPolicy != nil {
		// The forecast takes a receipt exactly when the basis is complete; one
		// it no longer takes leaves the deal unknown.
		out.Receipt = &crmcontracts.DealViabilityReceipt{
			Outcome: *v.outcome, Reasons: v.reasons, ValidUntil: *v.validUntil,
			PolicyVersion: *v.receiptPolicy, Current: v.complete,
		}
	}
	if v.review != nil && v.opened != nil && v.episode != nil {
		review := &crmcontracts.DealViabilityReview{
			Id: openapi_types.UUID(*v.review), OpenedAt: *v.opened, Episode: *v.episode,
			DeliveryState: (*crmcontracts.DealViabilityReviewDeliveryState)(v.delivery),
		}
		if v.task != nil && v.done != nil {
			review.Task = &crmcontracts.DealViabilityTask{Id: openapi_types.UUID(*v.task), Done: *v.done}
		}
		out.Review = review
	}
	return out
}

// standing is the qualification as the qualified forecast counts the deal. A
// won or lost deal is in no open reading, so it reads closed rather than the
// eligible the qualification rule gives every deal that is not open.
func (v dealViabilityRow) standing() crmcontracts.DealViabilityQualification {
	if !v.open {
		return crmcontracts.DealViabilityQualificationClosed
	}
	return crmcontracts.DealViabilityQualification(v.qualification)
}
