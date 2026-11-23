// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// DealRefileLimit bounds one resumable pass through recent captured activities.
const DealRefileLimit = 200

const dealRefileReason = "captured_activity_deal_refile"

// DealRefileScope carries identities only; All is reserved for the recovery sweep.
type DealRefileScope struct {
	Activities []ids.UUID `json:"activities,omitempty"`
	Contacts   []ids.UUID `json:"contacts,omitempty"`
	Companies  []ids.UUID `json:"companies,omitempty"`
	Projects   []ids.UUID `json:"projects,omitempty"`
	All        bool       `json:"all,omitempty"`
	// Recheck also asks the topic check about the rule's older suggested
	// links; only the recovery sweep sets it, beside All.
	Recheck bool `json:"recheck,omitempty"`
	// Backfill adds the one-time pass over a deal's stakeholders' old mail
	// (stakeholderbackfill.go) after the recheck. Only the recovery sweep sets
	// it, and only until that pass has finished.
	Backfill bool `json:"backfill,omitempty"`
}

// Empty prevents an unscoped trigger from becoming a full sweep.
func (s DealRefileScope) Empty() bool {
	return !s.All && len(s.Activities)+len(s.Contacts)+len(s.Companies)+len(s.Projects) == 0
}

// DealRefileCeilingTx freezes a finite key range before work is acknowledged.
func DealRefileCeilingTx(ctx context.Context, tx pgx.Tx) (ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return ids.Nil, err
	}
	var ceiling ids.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM activity WHERE archived_at IS NULL AND restricted_at IS NULL ORDER BY id DESC LIMIT 1`).Scan(&ceiling)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, nil
	}
	return ceiling, err
}

// RefileCandidatesTx advances by inspected identity, including ambiguous
// activities, and returns at most limit of them.
func (r DealRepair) RefileCandidatesTx(ctx context.Context, tx pgx.Tx, scope DealRefileScope, asOf time.Time, after, ceiling ids.UUID, limit int) ([]ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	if scope.Empty() {
		return nil, nil
	}
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	horizon := refileHorizonSQL(asOf, arg)
	query := fmt.Sprintf(`SELECT a.id FROM activity a
 WHERE a.kind IN ('email','meeting') AND %s AND %s
 AND a.id > $%d AND a.id <= $%d
 AND NOT EXISTS (SELECT 1 FROM activity_link filed WHERE filed.activity_id=a.id AND filed.entity_type='deal')
 AND ($%d::boolean OR a.id=ANY($%d::uuid[])
 OR EXISTS (SELECT 1 FROM activity_link link WHERE link.activity_id=a.id
 AND (link.contact_id=ANY($%d::uuid[]) OR link.company_id=ANY($%d::uuid[]) OR link.project_id=ANY($%d::uuid[])))
 OR EXISTS (SELECT 1 FROM activity_participant participant WHERE participant.activity_id=a.id
 AND (participant.contact_id=ANY($%d::uuid[]) OR EXISTS (
 SELECT 1 FROM relationship employer WHERE employer.contact_id=participant.contact_id
 AND `+employment.CurrentMembershipSQL("employer")+`
 AND employer.company_id=ANY($%d::uuid[])))))
 ORDER BY a.id LIMIT $%d`, capturedConnectorActivitySQL, horizon, arg(after), arg(ceiling), arg(scope.All), arg(scope.Activities), arg(scope.Contacts), arg(scope.Companies), arg(scope.Projects), arg(scope.Contacts), arg(scope.Companies), arg(limit))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

// Refile files one unfiled captured activity in its own transactions, with a
// suggested deal confirmed by the repair's Check between them. It never moves
// an existing link, including one an operator repair wrote, and reports
// whether it asked the model.
func (r DealRepair) Refile(ctx context.Context, db TxRunner, activity ids.ActivityID, asOf time.Time) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	plan := refilePlan{
		eligible:   func(arg func(any) int) string { return refileHorizonSQL(asOf, arg) },
		candidates: r.Candidates, ask: r.Ask,
	}
	result, err := r.checkedRefile(ctx, db, activity, plan)
	return result.Asked, err
}

// refilePlan is what varies between the refile passes. It names which mail may
// be filed, which deals it may be filed on, and whether a question may be
// raised. The guards and the decision are one path (refileTx).
type refilePlan struct {
	// eligible is a SQL condition over activity alias a, binding its values
	// through arg.
	eligible   func(arg func(any) int) string
	candidates DealCandidates
	// admit is one closed deal the mail may be filed on; zero for none.
	admit ids.UUID
	ask   AskWhichDeal
}

// checkedRefile runs refileTx with the topic check between its transactions.
// It reports whether the model was asked, whether the last decision filed the
// mail, and whether the check was deferred.
func (r DealRepair) checkedRefile(ctx context.Context, db TxRunner, activity ids.ActivityID, plan refilePlan) (BackfillResult, error) {
	var outcome DealOutcome
	var deferred bool
	asked, err := checkedFiling(ctx, db, r.Check, func(tx pgx.Tx, topic *dealTopicState) (*topicQuestion, error) {
		question, decided, err := r.refileTx(ctx, tx, activity, plan, topic)
		outcome = decided
		deferred = topic != nil && topic.answer != nil && topic.answer.deferred
		return question, err
	})
	return BackfillResult{Asked: asked, Filed: outcome == DealLinked, Deferred: deferred}, err
}

// refileTx is one transaction of a refile pass.
func (r DealRepair) refileTx(ctx context.Context, tx pgx.Tx, activity ids.ActivityID, plan refilePlan, topic *dealTopicState) (*topicQuestion, DealOutcome, error) {
	if live, err := lockForDeal(ctx, tx, activity); err != nil || !live {
		return nil, DealNone, err
	}
	args := []any{activity}
	arg := func(value any) int { args = append(args, value); return len(args) }
	var eligible bool
	query := `SELECT EXISTS (SELECT 1 FROM activity a WHERE a.id=$1
 AND a.kind IN ('email','meeting') AND ` + capturedConnectorActivitySQL + ` AND ` + plan.eligible(arg) + `)`
	if err := tx.QueryRow(ctx, query, args...).Scan(&eligible); err != nil || !eligible {
		return nil, DealNone, err
	}
	links, _, human, _, err := readDealProvenance(ctx, tx, activity)
	if err != nil || human || links > 0 {
		return nil, DealNone, err
	}
	var question *topicQuestion
	outcome, err := r.asMailbox(ctx, tx, activity, dealRefileReason, func(f dealFiling) (DealOutcome, error) {
		f.topic, f.admit = topic, plan.admit
		verdict, asked, err := fileOnDeal(f, tx, activity, plan.candidates, plan.ask)
		question = asked
		return verdict.Outcome, err
	})
	return question, outcome, err
}

func refileHorizonSQL(asOf time.Time, arg func(any) int) string {
	return storekit.SourceWithinHorizonSQL("a", asOf, arg) + fmt.Sprintf(" AND (a.kind<>'email' OR a.created_at >= $%d::timestamptz - make_interval(days => %s))", arg(asOf), storekit.WaitingHorizonSQL(asOf, arg))
}
