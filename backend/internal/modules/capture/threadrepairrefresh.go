// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func refreshThreadRepairUnitTx(ctx context.Context, tx pgx.Tx, job threadRepair) error {
	var id ids.ActivityID
	err := tx.QueryRow(ctx, `SELECT activity_id FROM capture_thread_repair_node WHERE repair_id=@job AND activity_id>@after ORDER BY activity_id LIMIT 1`, pgx.NamedArgs{threadArgJob: job.ID, threadArgAfter: job.After}).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return threadRepairPhaseTx(ctx, tx, job.ID, "discover")
	}
	if err != nil {
		return err
	}
	live, err := threadSourceLiveTx(ctx, tx, id)
	if err != nil {
		return err
	}
	if !live {
		_, err := tx.Exec(ctx, `DELETE FROM capture_thread_repair_node WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job.ID, "id": id})
		return err
	}
	var subject, source, parent, message string
	var version int64
	var cached *string
	err = tx.QueryRow(ctx, `SELECT coalesce(a.subject,''),coalesce(a.thread_key,''),a.version,coalesce(`+threadDirectParentSQL+`,''),CASE WHEN t.subject_hash=md5(coalesce(a.subject,'')) THEN t.own_topic END,coalesce((SELECT i.identity_key FROM activity_identity i WHERE i.activity_id=a.id AND i.identity_kind='mail' ORDER BY i.identity_key LIMIT 1),nullif(a.source_id,''),'activity:'||a.id::text) FROM activity a LEFT JOIN capture_thread_topic t ON t.activity_id=a.id WHERE a.id=@id`, pgx.NamedArgs{"id": id}).Scan(&subject, &source, &version, &parent, &cached, &message)
	if err != nil {
		return err
	}
	keys, err := threadHistoricalProjectKeysTx(ctx, tx, id)
	if err != nil {
		return err
	}
	topic := threadBoundaryTopic(subject, keys)
	if cached != nil {
		topic = *cached
	}
	_, err = tx.Exec(ctx, `UPDATE capture_thread_repair_node SET message_key=@message,source_key=@source,parent_key=@parent,parent_activity_id=NULL,own_topic=@topic,ancestor_topic=@topic,ancestor_distance=CASE WHEN @topic='' THEN -1 ELSE 0 END,frontier_key=CASE WHEN @topic='' THEN '' ELSE @message END,frontier_topic=@topic,root_key=@message,root_proven=false,child_after=@zero,edges_done=false,after_seat=@zero,after_basis=@zero,after_delivery=@zero,repair_after_seat=@zero,inherit_source='',stage=@message,observed_version=@version WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job.ID, "id": id, threadArgSource: source, threadArgParent: parent, threadArgTopic: topic, threadArgZero: ids.Nil, "version": version, "message": message})
	if err != nil {
		return err
	}
	return advanceThreadNodeTx(ctx, tx, job.ID, id)
}

func cleanThreadRepairUnitTx(ctx context.Context, tx pgx.Tx, job threadRepair, apply ThreadRepairApplier) error {
	var id ids.ActivityID
	var after ids.UUID
	var key string
	err := tx.QueryRow(ctx, `SELECT activity_id,after_seat,root_key FROM capture_thread_repair_node WHERE repair_id=@job ORDER BY activity_id LIMIT 1`, pgx.NamedArgs{threadArgJob: job.ID}).Scan(&id, &after, &key)
	if err == nil {
		return publishThreadNodeUnitTx(ctx, tx, job.ID, id, key, after, apply, job.Resumed)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM capture_thread_barrier WHERE ctid=(SELECT ctid FROM capture_thread_barrier WHERE repair_id=@job LIMIT 1)`, pgx.NamedArgs{threadArgJob: job.ID})
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	_, err = tx.Exec(ctx, `DELETE FROM capture_thread_repair WHERE id=@job AND phase='published'`, pgx.NamedArgs{threadArgJob: job.ID})
	return err
}

func publishThreadNodeUnitTx(ctx context.Context, tx pgx.Tx, job ids.UUID, id ids.ActivityID, key string, after ids.UUID, apply ThreadRepairApplier, notify bool) error {
	var locked ids.ActivityID
	if err := tx.QueryRow(ctx, `SELECT id FROM activity WHERE id=@id FOR UPDATE`, pgx.NamedArgs{"id": id}).Scan(&locked); err != nil {
		return err
	}
	invalid, err := threadRepairInvalidatedTx(ctx, tx, job)
	if err != nil || invalid {
		return err
	}
	if apply.PublishSeat != nil {
		found, err := publishThreadSeatUnitTx(ctx, tx, job, id, after, apply.PublishSeat)
		if err != nil || found {
			return err
		}
	}
	if err := emitPendingThreadReplyTx(ctx, tx, id); err != nil {
		return err
	}
	if notify && apply.Notify != nil {
		if err := apply.Notify(ctx, tx, id, key); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `DELETE FROM capture_thread_repair_node WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job, "id": id})
	return err
}

func publishThreadSeatUnitTx(ctx context.Context, tx pgx.Tx, job ids.UUID, id ids.ActivityID, after ids.UUID, publish func(context.Context, pgx.Tx, ids.ActivityID, ids.UUID) error) (bool, error) {
	var seat ids.UUID
	err := tx.QueryRow(ctx, `SELECT user_id FROM capture_import WHERE activity_id=@id AND user_id>@after ORDER BY user_id LIMIT 1`, pgx.NamedArgs{"id": id, threadArgAfter: after}).Scan(&seat)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := publish(ctx, tx, id, seat); err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `UPDATE capture_thread_repair_node SET after_seat=@seat WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job, "id": id, threadArgSeat: seat})
	return true, err
}
