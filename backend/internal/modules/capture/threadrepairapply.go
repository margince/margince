// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func applyThreadRepairUnitTx(ctx context.Context, tx pgx.Tx, job threadRepair, apply ThreadRepairApplier) (bool, error) {
	node, err := scanThreadNode(tx.QueryRow(ctx, `SELECT `+threadNodeColumns+` FROM capture_thread_repair_node WHERE repair_id=@job AND activity_id>@after ORDER BY activity_id LIMIT 1`, pgx.NamedArgs{threadArgJob: job.ID, threadArgAfter: job.After}))
	if errors.Is(err, pgx.ErrNoRows) {
		next := map[string]string{threadPhaseInherit: threadPhaseApply, threadPhaseApply: threadPhaseRepair, threadPhaseRepair: "published"}[job.Phase]
		return false, threadRepairPhaseTx(ctx, tx, job.ID, next)
	}
	if err != nil {
		return false, err
	}
	if err := addThreadBarrierTx(ctx, tx, job.ID, node.State.RootKey); err != nil {
		return false, err
	}
	switch job.Phase {
	case threadPhaseInherit:
		return false, inheritThreadRepairUnitTx(ctx, tx, job, node, apply)
	case threadPhaseApply:
		return moveThreadRepairUnitTx(ctx, tx, job, node, apply)
	default:
		return false, repairThreadRepresentativeUnitTx(ctx, tx, job, node, apply)
	}
}

func advanceThreadNodeTx(ctx context.Context, tx pgx.Tx, job ids.UUID, id ids.ActivityID) error {
	_, err := tx.Exec(ctx, `UPDATE capture_thread_repair SET after_activity=@after WHERE id=@job`, pgx.NamedArgs{threadArgJob: job, threadArgAfter: id})
	return err
}

// Topic boundaries never reopen a component's holding verdict or discard its consent evidence.
func inheritThreadRepairUnitTx(ctx context.Context, tx pgx.Tx, job threadRepair, n threadRepairNode, apply ThreadRepairApplier) error {
	var alreadyInherited bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM capture_thread_repair_node WHERE repair_id=@job AND root_key=@root AND activity_id<@id)`, pgx.NamedArgs{threadArgJob: job.ID, threadArgRoot: n.State.RootKey, "id": n.ID}).Scan(&alreadyInherited); err != nil {
		return err
	}
	if alreadyInherited {
		return advanceThreadNodeTx(ctx, tx, job.ID, n.ID)
	}
	var stage, source string
	var afterSeat, afterBasis ids.UUID
	if err := tx.QueryRow(ctx, `SELECT stage,inherit_source,after_seat,after_basis FROM capture_thread_repair_node WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job.ID, "id": n.ID}).Scan(&stage, &source, &afterSeat, &afterBasis); err != nil {
		return err
	}
	if source == "" {
		return nextThreadInheritanceSourceTx(ctx, tx, job, n, source)
	}
	if source == n.State.RootKey {
		return nextThreadInheritanceSourceTx(ctx, tx, job, n, source)
	}
	if stage == "verdict" {
		var seat ids.UUID
		err := tx.QueryRow(ctx, `SELECT user_id FROM capture_thread_verdict WHERE thread_key=@key AND user_id>@after ORDER BY user_id LIMIT 1`, pgx.NamedArgs{threadArgKey: source, threadArgAfter: afterSeat}).Scan(&seat)
		if errors.Is(err, pgx.ErrNoRows) {
			_, err = tx.Exec(ctx, `UPDATE capture_thread_repair_node SET stage='basis' WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job.ID, "id": n.ID})
			return err
		}
		if err != nil {
			return err
		}
		if err := apply.Verdict(ctx, tx, source, n.State.RootKey, seat); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE capture_thread_repair_node SET after_seat=@seat WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job.ID, "id": n.ID, threadArgSeat: seat})
		return err
	}
	var basis ids.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM communication_basis WHERE thread_key=@key AND id>@after AND revoked_at IS NULL ORDER BY id LIMIT 1`, pgx.NamedArgs{threadArgKey: source, threadArgAfter: afterBasis}).Scan(&basis)
	if errors.Is(err, pgx.ErrNoRows) {
		return nextThreadInheritanceSourceTx(ctx, tx, job, n, source)
	}
	if err != nil {
		return err
	}
	if err := apply.Basis(ctx, tx, basis, source, n.State.RootKey); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE capture_thread_repair_node SET after_basis=@basis WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job.ID, "id": n.ID, "basis": basis})
	return err
}

func nextThreadInheritanceSourceTx(ctx context.Context, tx pgx.Tx, job threadRepair, n threadRepairNode, after string) error {
	var source string
	err := tx.QueryRow(ctx, `SELECT blocked_key FROM capture_thread_barrier WHERE repair_id=@job AND loaded AND blocked_key>@after ORDER BY blocked_key LIMIT 1`, pgx.NamedArgs{threadArgJob: job.ID, threadArgAfter: after}).Scan(&source)
	if errors.Is(err, pgx.ErrNoRows) {
		return advanceThreadNodeTx(ctx, tx, job.ID, n.ID)
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE capture_thread_repair_node SET inherit_source=@source,stage='verdict',after_seat=@zero,after_basis=@zero WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgSource: source, threadArgZero: ids.Nil, threadArgJob: job.ID, "id": n.ID})
	return err
}

func moveThreadRepairUnitTx(ctx context.Context, tx pgx.Tx, job threadRepair, n threadRepairNode, apply ThreadRepairApplier) (bool, error) {
	var stage string
	var after ids.UUID
	if err := tx.QueryRow(ctx, `SELECT stage,after_delivery FROM capture_thread_repair_node WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job.ID, "id": n.ID}).Scan(&stage, &after); err != nil {
		return false, err
	}
	current, err := threadRepairNodeCurrentTx(ctx, tx, job, n)
	if err != nil || !current {
		return false, err
	}
	if stage != "delivery" {
		return moveThreadRepairActivityTx(ctx, tx, job, n, apply)
	}
	var delivery ids.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM comms_outbound WHERE activity_id=@id AND thread_key=@source AND id>@after ORDER BY id LIMIT 1`, pgx.NamedArgs{"id": n.ID, threadArgSource: n.Source, threadArgAfter: after}).Scan(&delivery)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, advanceThreadNodeTx(ctx, tx, job.ID, n.ID)
	}
	if err != nil {
		return false, err
	}
	if err := apply.Delivery(ctx, tx, delivery, n.ID, n.Source, n.State.RootKey); err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `UPDATE capture_thread_repair_node SET after_delivery=@after WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job.ID, "id": n.ID, threadArgAfter: delivery})
	return false, err
}

func moveThreadRepairActivityTx(ctx context.Context, tx pgx.Tx, job threadRepair, n threadRepairNode, apply ThreadRepairApplier) (bool, error) {
	changed, err := apply.Activity(ctx, tx, n.ID, n.Source, n.State.RootKey)
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO capture_thread_topic(activity_id,own_topic,subject_hash,effective_topic,root_key) SELECT @id,@topic,md5(coalesce(subject,'')),@effective,@root FROM activity WHERE id=@id ON CONFLICT(activity_id) DO UPDATE SET own_topic=EXCLUDED.own_topic,subject_hash=EXCLUDED.subject_hash,effective_topic=EXCLUDED.effective_topic,root_key=EXCLUDED.root_key`, pgx.NamedArgs{"id": n.ID, threadArgTopic: n.State.OwnTopic, "effective": n.State.topic(), threadArgRoot: n.State.RootKey})
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `UPDATE capture_thread_repair_node SET observed_version=(SELECT version FROM activity WHERE id=@id),stage='delivery' WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job.ID, "id": n.ID})
	return changed, err
}

func threadRepairNodeCurrentTx(ctx context.Context, tx pgx.Tx, job threadRepair, n threadRepairNode) (bool, error) {
	var version int64
	if err := tx.QueryRow(ctx, `SELECT version FROM activity WHERE id=@id FOR UPDATE`, pgx.NamedArgs{"id": n.ID}).Scan(&version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	invalid, err := threadRepairInvalidatedTx(ctx, tx, job.ID)
	if err != nil || invalid {
		return false, err
	}
	if version != n.Version {
		return false, threadRepairPhaseTx(ctx, tx, job.ID, "refresh")
	}
	return true, nil
}

func repairThreadRepresentativeUnitTx(ctx context.Context, tx pgx.Tx, job threadRepair, n threadRepairNode, apply ThreadRepairApplier) error {
	current, err := threadRepairNodeCurrentTx(ctx, tx, job, n)
	if err != nil || !current {
		return err
	}
	var after ids.UUID
	if err := tx.QueryRow(ctx, `SELECT repair_after_seat FROM capture_thread_repair_node WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job.ID, "id": n.ID}).Scan(&after); err != nil {
		return err
	}
	var seat ids.UUID
	err = tx.QueryRow(ctx, `(SELECT user_id FROM capture_thread_verdict WHERE thread_key=@source AND user_id>@after ORDER BY user_id LIMIT 1) UNION (SELECT user_id FROM capture_thread_verdict WHERE thread_key=@target AND user_id>@after ORDER BY user_id LIMIT 1) ORDER BY user_id LIMIT 1`, pgx.NamedArgs{threadArgSource: n.Source, "target": n.State.RootKey, threadArgAfter: after}).Scan(&seat)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err := tx.Exec(ctx, `UPDATE capture_thread_repair_node SET stage='publish',after_seat=@zero WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgZero: ids.Nil, threadArgJob: job.ID, "id": n.ID}); err != nil {
			return err
		}
		return advanceThreadNodeTx(ctx, tx, job.ID, n.ID)
	}
	if err != nil {
		return err
	}
	if err := apply.Representative(ctx, tx, n.Source, seat); err != nil {
		return err
	}
	if err := apply.Representative(ctx, tx, n.State.RootKey, seat); err != nil {
		return err
	}
	if apply.Hold != nil {
		if err := apply.Hold(ctx, tx, n.ID, seat); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE capture_thread_repair_node SET observed_version=(SELECT version FROM activity WHERE id=@id),repair_after_seat=@seat WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job.ID, "id": n.ID, threadArgSeat: seat})
	return err
}
