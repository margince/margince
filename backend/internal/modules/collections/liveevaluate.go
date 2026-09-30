// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// The Live List check: every Live List evaluated completely, as the system,
// and compared with the members it had at its last check. Each difference is
// an entered or left event stamped with the check's time. The check sees every
// record, so every read of what it recorded applies the reader's own row scope.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The budgets of one pass. A list matching more than LiveCheckRecordCap
// records is checked but not compared: holding a larger set would make one
// pass unbounded, and a partial set would record members leaving who did not.
const (
	LiveCheckRecordCap = 50_000
	liveCheckListCap   = 200
	liveCheckPage      = 200
)

// How a check ended (list_evaluation_outcome_check). CheckFailed is never
// stored: the list keeps its last checkpoint, so the next pass takes it first.
const (
	CheckComplete = "complete"
	CheckTooLarge = "too_large"
	CheckInvalid  = "invalid"
	CheckFailed   = "failed"
)

// Why a Live List member was recorded entering or leaving.
const (
	reasonEvaluated     = "evaluated"
	reasonFilterChanged = "filter_changed"
)

// The observed membership actions (list_member_event_action_check).
const (
	memberEntered = "entered"
	memberLeft    = "left"
)

// listEvaluatedAction is the system_log action a check that saw a change
// records, and the ledger row its list.evaluated event traces to.
const listEvaluatedAction = "list_evaluated"

// LiveCheck is what one check of one list did. Err is set when the check
// failed and recorded nothing.
type LiveCheck struct {
	ListID  ids.ListID
	Outcome string
	Members int
	Entered int
	Left    int
	Err     error
}

// CheckLiveLists checks the Live Lists checked longest ago, up to the pass's
// list budget, each in its own transaction and stamped with its own clock
// reading, so a pass cut short loses nothing and the next resumes with the
// lists it did not reach. One list's failure is reported on its LiveCheck and
// never stops the lists after it. The caller is the system: the check must see
// every record, whoever reads it later.
func (s *Store) CheckLiveLists(ctx context.Context, now func() time.Time) ([]LiveCheck, error) {
	due, err := s.dueLiveLists(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]LiveCheck, 0, len(due))
	for _, l := range due {
		check, err := s.checkLiveList(ctx, l, now())
		if err != nil {
			check = LiveCheck{ListID: l.ID, Outcome: CheckFailed, Err: fmt.Errorf("check list %s: %w", l.ID, err)}
		}
		out = append(out, check)
	}
	return out, nil
}

// invalidFilter says the stored filter can no longer be evaluated: it does not
// decode, or it names what the vocabulary no longer holds.
func invalidFilter(err error) bool {
	return errors.As(err, new(*storekit.PredicateError)) || errors.Is(err, errNotAFilterTree)
}

// dueLiveLists reads the live, non-archived Live Lists, never-checked first
// and then oldest check first.
func (s *Store) dueLiveLists(ctx context.Context) ([]listRow, error) {
	var out []listRow
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "SELECT "+listColumns+` FROM list l
			LEFT JOIN list_evaluation ev ON ev.list_id = l.id
			WHERE l.list_type = @list_type AND l.archived_at IS NULL
			ORDER BY ev.evaluated_at ASC NULLS FIRST, l.id LIMIT @cap`,
			pgx.StrictNamedArgs{listTypeField: listTypeDynamic, "cap": liveCheckListCap})
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (listRow, error) { return scanList(row) })
		return err
	})
	return out, err
}

// checkLiveList evaluates one list and records what changed, atomically with
// its checkpoint. A filter that no longer compiles is checkpointed invalid
// rather than failing the pass for every other list.
func (s *Store) checkLiveList(ctx context.Context, l listRow, at time.Time) (LiveCheck, error) {
	check, err := s.compareLiveList(ctx, l, at)
	if !invalidFilter(err) {
		return check, err
	}
	check = LiveCheck{ListID: l.ID, Outcome: CheckInvalid}
	return check, s.db.Tx(ctx, func(tx pgx.Tx) error {
		return writeCheckpoint(ctx, tx, l, at, check, nil)
	})
}

// compareLiveList runs the filter and records the difference in one
// REPEATABLE READ transaction. A record changed by another transaction after
// the filter ran fails the row lock the write takes, so an erasure that lands
// in between aborts this check rather than being written back.
func (s *Store) compareLiveList(ctx context.Context, l listRow, at time.Time) (LiveCheck, error) {
	check := LiveCheck{ListID: l.ID}
	engine, pred, err := s.liveFilter(ctx, l)
	if err != nil {
		return check, err
	}
	err = s.db.TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		matched, err := matchAll(ctx, tx, engine, pred)
		if err != nil {
			return err
		}
		check.Members = len(matched)
		if len(matched) > LiveCheckRecordCap {
			check.Outcome = CheckTooLarge
			return writeCheckpoint(ctx, tx, l, at, check, nil)
		}
		check.Outcome = CheckComplete
		return s.compareSnapshot(ctx, tx, l, matched, at, &check)
	})
	return check, err
}

// matchAll walks the filter's whole result in keyset pages, one past the cap
// so a set too large to hold is known to be so.
func matchAll(ctx context.Context, tx pgx.Tx, engine storekit.Query, pred storekit.Predicate) ([]ids.UUID, error) {
	matched := []ids.UUID{}
	var after *ids.UUID
	for len(matched) <= LiveCheckRecordCap {
		page, more, err := engine.SelectPage(ctx, tx, pred, after, liveCheckPage)
		if err != nil {
			return nil, err
		}
		matched = append(matched, page...)
		if !more || len(page) == 0 {
			return matched, nil
		}
		after = &page[len(page)-1]
	}
	return matched, nil
}

// compareSnapshot records who entered and left since the last complete check
// and replaces the held members. The first complete check only takes the
// snapshot: a list that existed before anyone watched it has no changes to
// report, only members.
func (s *Store) compareSnapshot(ctx context.Context, tx pgx.Tx, l listRow, matched []ids.UUID, at time.Time, check *LiveCheck) error {
	prior, held, err := readSnapshotVersion(ctx, tx, l.ID)
	if err != nil {
		return err
	}
	reason := reasonEvaluated
	if held {
		changed, err := filterChangedSince(ctx, tx, l, prior)
		if err != nil {
			return err
		}
		if changed {
			reason = reasonFilterChanged
		}
	}
	actor, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	came, gone, err := snapshotDifference(ctx, tx, l, matched)
	if err != nil {
		return err
	}
	if err := lockChangedRecords(ctx, tx, l.EntityType, came, gone); err != nil {
		return err
	}
	args := pgx.StrictNamedArgs{
		listIDField: l.ID, "changed": gone, "at": at,
		versionField: l.Version, "reason": reason, "actor": actor, "record": held,
	}
	if err := tx.QueryRow(ctx, leaveStatement, args).Scan(&check.Left); err != nil {
		return fmt.Errorf("record who left: %w", err)
	}
	args["changed"], args[entityTypeField] = came, l.EntityType
	if err := tx.QueryRow(ctx, enterStatement, args).Scan(&check.Entered); err != nil {
		return fmt.Errorf("record who entered: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE list_live_member SET definition_version = @version
		WHERE list_id = @list_id AND definition_version <> @version`,
		pgx.StrictNamedArgs{listIDField: l.ID, versionField: l.Version}); err != nil {
		return err
	}
	if !held {
		check.Entered, check.Left = 0, 0
	}
	if err := writeCheckpoint(ctx, tx, l, at, *check, &l.Version); err != nil {
		return err
	}
	if check.Entered+check.Left == 0 {
		return nil
	}
	return announceCheck(ctx, tx, l, at, *check, reason == reasonFilterChanged)
}

// snapshotDifference answers which matched records the list does not hold
// yet, and which held members the filter no longer selects.
func snapshotDifference(ctx context.Context, tx pgx.Tx, l listRow, matched []ids.UUID) (came, gone []ids.UUID, err error) {
	args := pgx.StrictNamedArgs{listIDField: l.ID, "matched": matched}
	rows, err := tx.Query(ctx, `SELECT m.id FROM unnest(@matched::uuid[]) AS m(id)
		WHERE NOT EXISTS (SELECT 1 FROM list_live_member s WHERE s.list_id = @list_id AND s.entity_id = m.id)`, args)
	if err != nil {
		return nil, nil, err
	}
	if came, err = storekit.ScanUUIDColumn(rows, "the records a Live List gained"); err != nil {
		return nil, nil, err
	}
	rows, err = tx.Query(ctx, `SELECT s.entity_id FROM list_live_member s WHERE s.list_id = @list_id
		AND s.entity_id NOT IN (SELECT unnest(@matched::uuid[]))`, args)
	if err != nil {
		return nil, nil, err
	}
	gone, err = storekit.ScanUUIDColumn(rows, "the records a Live List lost")
	return nonNil(came), nonNil(gone), err
}

// nonNil keeps an empty set an empty array: a nil slice binds as SQL NULL.
func nonNil(set []ids.UUID) []ids.UUID {
	if set == nil {
		return []ids.UUID{}
	}
	return set
}

// lockChangedRecords takes a share lock on every record about to be written
// as entered or left. Erasure updates the record before it clears the list
// rows, so either it waits for this check and then clears what it wrote, or
// it got there first and this REPEATABLE READ check fails on the lock instead
// of writing an erased record back.
func lockChangedRecords(ctx context.Context, tx pgx.Tx, entityType string, came, gone []ids.UUID) error {
	if len(came)+len(gone) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(
		`SELECT e.id FROM %s e WHERE e.id = ANY(@came) OR e.id = ANY(@gone) ORDER BY e.id FOR SHARE`,
		pgx.Identifier{entityType}.Sanitize()), pgx.StrictNamedArgs{"came": came, "gone": gone})
	if err != nil {
		return err
	}
	_, err = storekit.ScanUUIDColumn(rows, "the records a Live List check writes")
	return err
}

// leaveStatement drops the held members the filter no longer selects and,
// past the first check, records each as having left.
const leaveStatement = `WITH gone AS (
		DELETE FROM list_live_member s WHERE s.list_id = @list_id AND s.entity_id = ANY(@changed::uuid[])
		RETURNING s.entity_type, s.entity_id),
	logged AS (
		INSERT INTO list_member_event (list_id, entity_type, entity_id, action, reason, actor, occurred_at, definition_version)
		SELECT @list_id, g.entity_type, g.entity_id, 'left', @reason, @actor, @at, @version FROM gone g WHERE @record::boolean)
	SELECT count(*) FROM gone`

// enterStatement holds the members the filter newly selects and, past the
// first check, records each as having entered.
const enterStatement = `WITH came AS (
		INSERT INTO list_live_member (list_id, entity_type, entity_id, member_since, definition_version)
		SELECT @list_id, @entity_type, m.id, @at, @version FROM unnest(@changed::uuid[]) AS m(id)
		ON CONFLICT (list_id, entity_id) DO NOTHING
		RETURNING entity_type, entity_id),
	logged AS (
		INSERT INTO list_member_event (list_id, entity_type, entity_id, action, reason, actor, occurred_at, definition_version)
		SELECT @list_id, c.entity_type, c.entity_id, 'entered', @reason, @actor, @at, @version FROM came c WHERE @record::boolean)
	SELECT count(*) FROM came`

// readSnapshotVersion answers the version the held members were taken under;
// held is false until a check of the list has completed.
func readSnapshotVersion(ctx context.Context, tx pgx.Tx, listID ids.ListID) (version int64, held bool, err error) {
	var stored *int64
	err = tx.QueryRow(ctx, `SELECT snapshot_version FROM list_evaluation WHERE list_id = @list_id`,
		pgx.StrictNamedArgs{listIDField: listID}).Scan(&stored)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && stored == nil) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return *stored, true, nil
}

// filterChangedSince says whether the list's filter differs from the one it
// had at version: a rename or a sharing change bumps the version too, and
// only a different filter makes a change "after the filter changed".
func filterChangedSince(ctx context.Context, tx pgx.Tx, l listRow, version int64) (bool, error) {
	if version == l.Version {
		return false, nil
	}
	var changed bool
	err := tx.QueryRow(ctx, `SELECT NOT EXISTS (
			SELECT 1 FROM list_revision r JOIN list l ON l.id = r.list_id
			WHERE r.list_id = @list_id AND r.version = @version AND r.definition IS NOT DISTINCT FROM l.definition)`,
		pgx.StrictNamedArgs{listIDField: l.ID, versionField: version}).Scan(&changed)
	return changed, err
}

// writeCheckpoint records the check, keeping the snapshot version of the
// last complete one when this one held no members.
func writeCheckpoint(ctx context.Context, tx pgx.Tx, l listRow, at time.Time, check LiveCheck, snapshot *int64) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO list_evaluation (list_id, definition_version, evaluated_at, member_count, outcome, snapshot_version)
		VALUES (@list_id, @version, @at, @members, @outcome, @snapshot)
		ON CONFLICT (list_id) DO UPDATE SET definition_version = EXCLUDED.definition_version,
			evaluated_at = EXCLUDED.evaluated_at, member_count = EXCLUDED.member_count,
			outcome = EXCLUDED.outcome,
			snapshot_version = COALESCE(EXCLUDED.snapshot_version, list_evaluation.snapshot_version)`,
		pgx.StrictNamedArgs{
			listIDField: l.ID, versionField: l.Version, "at": at, "members": check.Members,
			"outcome": check.Outcome, "snapshot": snapshot,
		})
	return err
}

// announceCheck ledgers a check that saw a change and publishes it once. The
// counts stay on the ledger row: the check saw every record, and a count on
// the event would tell a subscriber how many they cannot see.
func announceCheck(ctx context.Context, tx pgx.Tx, l listRow, at time.Time, check LiveCheck, filterChanged bool) error {
	ledgerID, err := storekit.LogSystem(ctx, tx, listEvaluatedAction, map[string]any{
		listIDField: l.ID.String(), "definition_version": l.Version,
		"entered": check.Entered, "left": check.Left, "filter_changed": filterChanged,
	})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, ledgerID, l.ID.UUID, crmcontracts.PublicEventListEvaluated{
		DefinitionVersion: l.Version, EvaluatedAt: at, FilterChanged: filterChanged,
	})
}
