// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package initiatives

import (
	"context"
	"errors"
	"fmt"
	"time"

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

// enrolmentDating is the SQL that dates a new row, given the observation
// instant @at. Never later than @at, never before the launch, and a pick
// joined when it was saved.
type enrolmentDating string

// pickedSince dates a pick from when it was saved.
const pickedSince = `coalesce((SELECT m.entered_at FROM sales_initiative_member m WHERE m.initiative_id = i.id AND m.record_type = u.record_type AND m.record_id = u.record_id), @at)`

// observedJoin dates a rule match or a carried deal when the writer first
// sees it. Every write after the upgrade dates this way.
const observedJoin enrolmentDating = `least(@at, greatest(i.launch_at, CASE WHEN u.reason = 'picked' THEN ` + pickedSince + `
	ELSE @at END))`

// estimatedJoin is for an initiative live before the dated rows existed, whose
// matches were never observed. A match dates from the last of the launch, the
// last rule change and the record's own first day.
const estimatedJoin enrolmentDating = `least(@at, greatest(i.launch_at, CASE WHEN u.reason = 'picked' THEN ` + pickedSince + `
	ELSE greatest(i.created_at,
		(SELECT max(r.created_at) FROM sales_initiative_revision r WHERE r.initiative_id = i.id),
		CASE u.record_type WHEN 'lead' THEN (SELECT l.created_at FROM lead l WHERE l.id = u.record_id)
			ELSE (SELECT d.created_at FROM deal d WHERE d.id = u.record_id) END) END))`

// promotedFromEnrolledSQL selects the deals promoted after the launch from a
// lead the initiative held when it was promoted. It reads the dated rows, so a
// lead the promotion archived still counts. $1 is the initiative, $2 the launch.
const promotedFromEnrolledSQL = `SELECT DISTINCT l.qualified_deal_id FROM sales_initiative_enrolment e JOIN lead l ON l.id = e.record_id
	WHERE e.initiative_id = $1 AND e.record_type = 'lead' AND l.qualified_deal_id IS NOT NULL AND l.promoted_at >= $2
		AND e.joined_at <= l.promoted_at AND (e.left_at IS NULL OR e.left_at > l.promoted_at)`

// SourcedDealsTx is the member deals the initiative sourced: each was promoted
// after the launch from a lead enrolled when it was promoted. A lead picked
// after its promotion belongs now but sourced nothing. A reader without the
// lead grant gets none, since sourcing is read off the lead table.
func SourcedDealsTx(ctx context.Context, tx pgx.Tx, id ids.UUID, membership Membership) ([]ids.UUID, error) {
	if membership.Launch.IsZero() || !auth.ReadGranted(ctx, recordLead) {
		return nil, nil
	}
	args := []any{id, membership.Launch, membership.Deals}
	scope, err := auth.ScopeClauseFor(ctx, recordLead, "l", func(value any) int {
		args = append(args, value)
		return len(args)
	})
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = unscoped
	}
	rows, err := tx.Query(ctx, promotedFromEnrolledSQL+` AND l.qualified_deal_id = ANY($3) AND `+scope, args...)
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
// membership, for the sweep. A new row is dated now.
func (s *Store) SyncEnrolmentTx(ctx context.Context, tx pgx.Tx, id ids.UUID) (EnrolmentChange, error) {
	return s.lockedEnrolmentTx(ctx, tx, id, observedJoin)
}

// EstimateEnrolmentTx is SyncEnrolmentTx for the upgrade task, for an
// initiative live before the dated rows existed: its new rows are estimated.
func (s *Store) EstimateEnrolmentTx(ctx context.Context, tx pgx.Tx, id ids.UUID) (EnrolmentChange, error) {
	return s.lockedEnrolmentTx(ctx, tx, id, estimatedJoin)
}

func (s *Store) lockedEnrolmentTx(ctx context.Context, tx pgx.Tx, id ids.UUID, dating enrolmentDating) (EnrolmentChange, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return EnrolmentChange{}, err
	}
	initiative, err := lockInitiative(ctx, tx, id)
	if err != nil {
		return EnrolmentChange{}, err
	}
	return s.reconcileEnrolmentTx(ctx, tx, initiative, dating, s.now().UTC())
}

// WitnessLeadPromotionTx dates, inside a lead's promotion and before the lead
// is archived, every live initiative that can hold the lead. So a lead created
// and promoted between two sweeps still sourced its deal. at is the promotion.
func (s *Store) WitnessLeadPromotionTx(ctx context.Context, tx pgx.Tx, lead ids.UUID, at time.Time) error {
	rows, err := tx.Query(ctx, `SELECT i.id FROM sales_initiative i WHERE i.status = 'live' AND i.archived_at IS NULL
		AND (coalesce(i.lead_rule, '{}'::jsonb) <> '{}'::jsonb OR EXISTS (SELECT 1 FROM sales_initiative_member m
			WHERE m.initiative_id = i.id AND m.record_type = 'lead' AND m.record_id = $1 AND m.reason = 'picked'))
		ORDER BY i.id`, lead)
	if err != nil {
		return fmt.Errorf("read the initiatives a promoted lead may belong to: %w", err)
	}
	candidates, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("read the initiatives a promoted lead may belong to: %w", err)
	}
	system := principal.SystemActing(ctx, enrolmentActor)
	for _, id := range candidates {
		initiative, err := lockInitiative(system, tx, id)
		if err != nil {
			return err
		}
		if _, err := s.reconcileEnrolmentTx(system, tx, initiative, observedJoin, at); err != nil {
			return err
		}
	}
	return nil
}

// enrolmentActor is the principal the dated rows are written as: they are a
// fact about the initiative, so no reader's row scope narrows them.
const enrolmentActor = "system:initiative-enrolment"

// observeEnrolmentTx dates the rows of a definition, launch or members save, now.
func (s *Store) observeEnrolmentTx(ctx context.Context, tx pgx.Tx, initiative crmcontracts.SalesInitiative) (EnrolmentChange, error) {
	return s.reconcileEnrolmentTx(ctx, tx, initiative, observedJoin, s.now().UTC())
}

// reconcileEnrolmentTx writes sales_initiative_enrolment. The caller holds the
// initiative row. A new member gets an open row and a record that stopped
// belonging gets its left_at. A draft or ended initiative is left as it is:
// enrolment starts at the launch.
func (s *Store) reconcileEnrolmentTx(ctx context.Context, tx pgx.Tx, initiative crmcontracts.SalesInitiative, dating enrolmentDating, at time.Time) (EnrolmentChange, error) {
	if initiative.Status != crmcontracts.SalesInitiativeStatusLive {
		return EnrolmentChange{}, nil
	}
	id := ids.UUID(initiative.Id)
	membership, err := s.membersTx(principal.SystemActing(ctx, enrolmentActor), tx, initiative, completeRead)
	if err != nil {
		return EnrolmentChange{}, err
	}
	current := map[enrolmentKey]string{}
	for recordType, members := range map[string][]ids.UUID{recordLead: membership.Leads, recordDeal: membership.Deals} {
		for _, member := range members {
			current[enrolmentKey{recordType, member}] = membership.Reasons[recordType][member]
		}
	}
	open, err := openEnrolments(ctx, tx, id)
	if err != nil {
		return EnrolmentChange{}, err
	}
	var joined, left []enrolmentKey
	var reasons []string
	for key, reason := range current {
		if !open[key] {
			joined, reasons = append(joined, key), append(reasons, reason)
		}
	}
	for key := range open {
		if _, still := current[key]; !still {
			left = append(left, key)
		}
	}
	change := EnrolmentChange{Joined: len(joined), Left: len(left)}
	if change == (EnrolmentChange{}) {
		return change, nil
	}
	if err := writeEnrolment(ctx, tx, id, joined, reasons, left, dating, at); err != nil {
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

func writeEnrolment(ctx context.Context, tx pgx.Tx, id ids.UUID, joined []enrolmentKey, reasons []string, left []enrolmentKey, dating enrolmentDating, at time.Time) error {
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	joinTypes, joinIDs := splitKeys(joined)
	leftTypes, leftIDs := splitKeys(left)
	args := pgx.NamedArgs{"initiative": id, "by": by, "at": at, "types": joinTypes, "ids": joinIDs, "reasons": reasons, "left_types": leftTypes, "left_ids": leftIDs}
	// A lead and a deal may share an id, so a row closes by its type and id.
	if _, err := tx.Exec(ctx, `UPDATE sales_initiative_enrolment e SET left_at = greatest(e.joined_at, @at)
		FROM unnest(@left_types::text[], @left_ids::uuid[]) AS g(record_type, record_id)
		WHERE e.initiative_id = @initiative AND e.left_at IS NULL AND e.record_type = g.record_type AND e.record_id = g.record_id`, args); err != nil {
		return fmt.Errorf("close initiative enrolments: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO sales_initiative_enrolment (initiative_id, record_type, record_id, reason, joined_at, captured_by)
		SELECT i.id, u.record_type, u.record_id, u.reason, `+string(dating)+`, @by
		FROM sales_initiative i, unnest(@types::text[], @ids::uuid[], @reasons::text[]) AS u(record_type, record_id, reason)
		WHERE i.id = @initiative`, args); err != nil {
		return fmt.Errorf("open initiative enrolments: %w", err)
	}
	return nil
}

func splitKeys(keys []enrolmentKey) ([]string, []ids.UUID) {
	types, recordIDs := make([]string, len(keys)), make([]ids.UUID, len(keys))
	for i, key := range keys {
		types[i], recordIDs[i] = key.recordType, key.id
	}
	return types, recordIDs
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
