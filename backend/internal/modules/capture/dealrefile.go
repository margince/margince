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
	err := tx.QueryRow(ctx, `SELECT id FROM activity WHERE (archived_at IS NULL AND activity_thread_available(thread_key)) AND restricted_at IS NULL ORDER BY id DESC LIMIT 1`).Scan(&ceiling)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, nil
	}
	return ceiling, err
}

// RefileCandidatesTx advances by inspected identity, including ambiguous activities.
func (r DealRepair) RefileCandidatesTx(ctx context.Context, tx pgx.Tx, scope DealRefileScope, asOf time.Time, after, ceiling ids.UUID) ([]ids.UUID, error) {
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
 ORDER BY a.id LIMIT $%d`, capturedConnectorActivitySQL, horizon, arg(after), arg(ceiling), arg(scope.All), arg(scope.Activities), arg(scope.Contacts), arg(scope.Companies), arg(scope.Projects), arg(scope.Contacts), arg(scope.Companies), arg(DealRefileLimit))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

// RefileTx never moves an existing link, including a link an operator repair wrote.
func (r DealRepair) RefileTx(ctx context.Context, tx pgx.Tx, activity ids.ActivityID, asOf time.Time) (DealOutcome, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return "", err
	}
	if live, err := lockForDeal(ctx, tx, activity); err != nil || !live {
		return DealNone, err
	}
	args := []any{activity}
	arg := func(value any) int { args = append(args, value); return len(args) }
	horizon := refileHorizonSQL(asOf, arg)
	var eligible bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM activity a WHERE a.id=$1
 AND a.kind IN ('email','meeting') AND `+capturedConnectorActivitySQL+` AND `+horizon+`)`, args...).Scan(&eligible); err != nil || !eligible {
		return DealNone, err
	}
	links, _, human, _, err := readDealProvenance(ctx, tx, activity)
	if err != nil {
		return "", err
	}
	if human {
		return DealHumanKept, nil
	}
	if links > 0 {
		return DealAlreadyFiled, nil
	}
	return r.asMailbox(ctx, tx, activity, dealRefileReason, func(f dealFiling) (DealOutcome, error) {
		verdict, err := fileOnDeal(f, tx, activity, r.Candidates, nil)
		return verdict.Outcome, err
	})
}

func refileHorizonSQL(asOf time.Time, arg func(any) int) string {
	return storekit.SourceWithinHorizonSQL("a", asOf, arg) + fmt.Sprintf(" AND (a.kind<>'email' OR a.created_at >= $%d::timestamptz - make_interval(days => %s))", arg(asOf), storekit.WaitingHorizonSQL(asOf, arg))
}
