// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package initiatives

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// EnrolmentChange counts the dated rows one reconcile opened and closed.
type EnrolmentChange struct {
	Joined, Left int
}

// sweepPage is how many initiatives one read of the sweep pages.
const sweepPage = 100

type enrolmentKey struct {
	recordType string
	id         ids.UUID
}

// enrolmentJoinedAt dates a new row. Enrolment starts at the launch. A pick
// joined when it was saved. A rule match or a carried deal joined once both
// the record and the definition that admits it existed. Never later than now.
const enrolmentJoinedAt = `least(now(), greatest(i.launch_at, CASE WHEN u.reason = 'picked'
	THEN coalesce((SELECT m.entered_at FROM sales_initiative_member m WHERE m.initiative_id = i.id AND m.record_type = u.record_type AND m.record_id = u.record_id), now())
	ELSE greatest(i.created_at,
		(SELECT max(r.created_at) FROM sales_initiative_revision r WHERE r.initiative_id = i.id),
		CASE u.record_type WHEN 'lead' THEN (SELECT l.created_at FROM lead l WHERE l.id = u.record_id)
			ELSE (SELECT d.created_at FROM deal d WHERE d.id = u.record_id) END) END))`

// enrolmentMemberSQL is whether record recordID of recordType was a member of
// initiative at time at. Every argument is a SQL expression or a bind.
func enrolmentMemberSQL(initiative, recordType, recordID, at string) string {
	return `EXISTS (SELECT 1 FROM sales_initiative_enrolment e WHERE e.initiative_id = ` + initiative +
		` AND e.record_type = ` + recordType + ` AND e.record_id = ` + recordID +
		` AND e.joined_at <= ` + at + ` AND (e.left_at IS NULL OR e.left_at > ` + at + `))`
}

// SourcedDealsTx is the member deals the initiative sourced: each was promoted
// after the launch from a member lead that was enrolled when it was promoted.
// A lead picked after its promotion belongs now but sourced nothing. A reader
// without the lead grant gets none, since sourcing is read off the lead table.
func SourcedDealsTx(ctx context.Context, tx pgx.Tx, id ids.UUID, membership Membership) ([]ids.UUID, error) {
	if membership.Launch.IsZero() || !auth.ReadGranted(ctx, recordLead) {
		return nil, nil
	}
	var args []any
	arg := func(value any) int {
		args = append(args, value)
		return len(args)
	}
	bind := func(value any) string { return "$" + strconv.Itoa(arg(value)) }
	where := `l.id = ANY(` + bind(membership.Leads) + `) AND l.qualified_deal_id = ANY(` + bind(membership.Deals) + `) AND l.promoted_at >= ` + bind(membership.Launch) +
		` AND ` + enrolmentMemberSQL(bind(id), "'lead'", "l.id", "l.promoted_at")
	scope, err := auth.ScopeClauseFor(ctx, recordLead, "l", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = "TRUE"
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT l.qualified_deal_id FROM lead l WHERE `+where+` AND `+scope, args...)
	if err != nil {
		return nil, fmt.Errorf("read the sourced deals: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

// LiveInitiativesTx pages the live initiatives after id, in id order: the ones
// whose membership is dated.
func LiveInitiativesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT id FROM sales_initiative WHERE id > $1 AND status = 'live' AND archived_at IS NULL ORDER BY id LIMIT $2`, after, limit)
	if err != nil {
		return nil, fmt.Errorf("page live initiatives: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

// SyncEnrolmentTx brings one live initiative's dated rows in line with its
// membership, for the sweep and the upgrade task.
func (s *Store) SyncEnrolmentTx(ctx context.Context, tx pgx.Tx, id ids.UUID) (EnrolmentChange, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return EnrolmentChange{}, err
	}
	initiative, err := lockInitiative(ctx, tx, id)
	if err != nil {
		return EnrolmentChange{}, err
	}
	return s.syncEnrolmentTx(ctx, tx, initiative)
}

// syncEnrolmentTx writes sales_initiative_enrolment. The caller
// holds the initiative row. A new member gets an open row and a record that
// stopped belonging gets its left_at. A draft or ended initiative is left as
// it is: enrolment starts at the launch.
func (s *Store) syncEnrolmentTx(ctx context.Context, tx pgx.Tx, initiative crmcontracts.SalesInitiative) (EnrolmentChange, error) {
	if initiative.Status != crmcontracts.SalesInitiativeStatusLive {
		return EnrolmentChange{}, nil
	}
	id := ids.UUID(initiative.Id)
	// The dated rows are a fact about the initiative, so no reader's row scope
	// narrows the evaluation that writes them.
	membership, err := s.membersTx(principal.SystemActing(ctx, "system:initiative-enrolment"), tx, initiative)
	if err != nil {
		return EnrolmentChange{}, err
	}
	current := map[enrolmentKey]string{}
	for recordType, members := range map[string][]ids.UUID{recordLead: membership.Leads, recordDeal: membership.Deals} {
		for _, member := range members {
			current[enrolmentKey{recordType, member}] = membership.Reasons[member]
		}
	}
	open, err := openEnrolments(ctx, tx, id)
	if err != nil {
		return EnrolmentChange{}, err
	}
	var joinTypes, joinReasons []string
	var joinIDs []ids.UUID
	for key, reason := range current {
		if !open[key] {
			joinTypes, joinIDs, joinReasons = append(joinTypes, key.recordType), append(joinIDs, key.id), append(joinReasons, reason)
		}
	}
	var leftRows []ids.UUID
	// A truncated rule read cannot tell a record past the bound from one that
	// stopped matching, so nothing leaves on it.
	if !membership.Truncated {
		for key := range open {
			if _, still := current[key]; !still {
				leftRows = append(leftRows, key.id)
			}
		}
	}
	change := EnrolmentChange{Joined: len(joinIDs), Left: len(leftRows)}
	if change == (EnrolmentChange{}) {
		return change, nil
	}
	if err := s.writeEnrolment(ctx, tx, id, joinTypes, joinIDs, joinReasons, leftRows); err != nil {
		return change, err
	}
	return change, recordEnrolment(ctx, tx, initiative, len(open), len(open)+change.Joined-change.Left)
}

func openEnrolments(ctx context.Context, tx pgx.Tx, id ids.UUID) (map[enrolmentKey]bool, error) {
	rows, err := tx.Query(ctx, `SELECT record_type, record_id FROM sales_initiative_enrolment WHERE initiative_id = $1 AND left_at IS NULL`, id)
	if err != nil {
		return nil, fmt.Errorf("read open enrolments: %w", err)
	}
	keys, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (enrolmentKey, error) {
		var key enrolmentKey
		err := row.Scan(&key.recordType, &key.id)
		return key, err
	})
	if err != nil {
		return nil, fmt.Errorf("read open enrolments: %w", err)
	}
	open := make(map[enrolmentKey]bool, len(keys))
	for _, key := range keys {
		open[key] = true
	}
	return open, nil
}

func (s *Store) writeEnrolment(ctx context.Context, tx pgx.Tx, id ids.UUID, joinTypes []string, joinIDs []ids.UUID, joinReasons []string, leftRows []ids.UUID) error {
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	args := pgx.NamedArgs{"initiative": id, "by": by, "types": joinTypes, "ids": joinIDs, "reasons": joinReasons, "left": leftRows}
	if _, err := tx.Exec(ctx, `UPDATE sales_initiative_enrolment SET left_at = greatest(joined_at, now())
		WHERE initiative_id = @initiative AND left_at IS NULL AND record_id = ANY(@left::uuid[])`, args); err != nil {
		return fmt.Errorf("close initiative enrolments: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO sales_initiative_enrolment (initiative_id, record_type, record_id, reason, joined_at, captured_by)
		SELECT i.id, u.record_type, u.record_id, u.reason, `+enrolmentJoinedAt+`, @by
		FROM sales_initiative i, unnest(@types::text[], @ids::uuid[], @reasons::text[]) AS u(record_type, record_id, reason)
		WHERE i.id = @initiative`, args); err != nil {
		return fmt.Errorf("open initiative enrolments: %w", err)
	}
	return nil
}

// recordEnrolment lands the write shape for one reconcile: an audit row on
// the initiative with its open-membership count, and the paired event.
func recordEnrolment(ctx context.Context, tx pgx.Tx, initiative crmcontracts.SalesInitiative, before, after int) error {
	id := ids.UUID(initiative.Id)
	auditID, err := storekit.Audit(ctx, tx, "update", object, id, map[string]any{"enrolled": before}, map[string]any{"enrolled": after})
	if err != nil {
		return fmt.Errorf("audit initiative enrolment: %w", err)
	}
	return storekit.EmitEvent(ctx, tx, auditID, id, crmcontracts.PublicEventSalesInitiativeUpdated{
		InitiativeId: initiative.Id, Status: crmcontracts.PublicEventSalesInitiativeUpdatedStatus(initiative.Status), ChangedFields: []string{"enrolment"},
	})
}

// SyncLiveEnrolments reconciles every live initiative of the workspace, each
// in its own transaction, so one initiative's failure leaves the rest written.
func (s *Store) SyncLiveEnrolments(ctx context.Context) (EnrolmentChange, error) {
	var total EnrolmentChange
	var failures []error
	for after := (ids.UUID{}); ; {
		var page []ids.UUID
		if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
			var err error
			page, err = LiveInitiativesTx(ctx, tx, after, sweepPage)
			return err
		}); err != nil {
			return total, err
		}
		for _, id := range page {
			var change EnrolmentChange
			if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
				var err error
				change, err = s.SyncEnrolmentTx(ctx, tx, id)
				return err
			}); err != nil {
				// An initiative archived since the page was read has nothing to date.
				if !errors.Is(err, apperrors.ErrNotFound) {
					failures = append(failures, fmt.Errorf("initiative %s: %w", id, err))
				}
				continue
			}
			total.Joined, total.Left = total.Joined+change.Joined, total.Left+change.Left
		}
		if len(page) < sweepPage {
			return total, errors.Join(failures...)
		}
		after = page[len(page)-1]
	}
}

// CountUndatedLiveInitiatives counts, up to maximum+1, the live initiatives
// with no dated row yet: the ones the upgrade task still has to date.
func CountUndatedLiveInitiatives(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, maximum int64,
) (int64, error) {
	if err := auth.Require(ctx, object, principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM sales_initiative i WHERE i.status = 'live' AND i.archived_at IS NULL
		AND NOT EXISTS (SELECT 1 FROM sales_initiative_enrolment e WHERE e.initiative_id = i.id) LIMIT $1) undated`, maximum+1).Scan(&count)
	return count, err
}
