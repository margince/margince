// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const threadDirectParentSQL = `(SELECT r.referenced_id FROM activity_mail_reference r WHERE r.activity_id=a.id AND r.reference_order>0 AND NOT EXISTS(SELECT 1 FROM activity_mail_reference conflicting WHERE conflicting.activity_id=r.activity_id AND conflicting.reference_order>0 AND (conflicting.reference_order<r.reference_order OR (conflicting.reference_order=r.reference_order AND conflicting.referenced_id<>r.referenced_id))) ORDER BY r.reference_order,r.referenced_id LIMIT 1)`

func loadThreadRepairUnitTx(ctx context.Context, tx pgx.Tx, job threadRepair) error {
	var key string
	var after ids.UUID
	err := tx.QueryRow(ctx, `SELECT blocked_key,after_activity FROM capture_thread_barrier WHERE repair_id=@job AND NOT loaded ORDER BY blocked_key LIMIT 1`, pgx.NamedArgs{threadArgJob: job.ID}).Scan(&key, &after)
	if errors.Is(err, pgx.ErrNoRows) {
		return threadRepairPhaseTx(ctx, tx, job.ID, "discover")
	}
	if err != nil {
		return err
	}
	var id ids.ActivityID
	var message, subject, parent string
	var persisted *string
	var version int64
	err = tx.QueryRow(ctx, `SELECT a.id,coalesce((SELECT i.identity_key FROM activity_identity i WHERE i.activity_id=a.id AND i.identity_kind='mail' ORDER BY i.identity_key LIMIT 1),nullif(a.source_id,''),'activity:'||a.id::text),coalesce(a.subject,''),coalesce(`+threadDirectParentSQL+`,''),CASE WHEN t.subject_hash=md5(coalesce(a.subject,'')) THEN t.own_topic END,a.version
 FROM activity a LEFT JOIN capture_thread_topic t ON t.activity_id=a.id
 WHERE a.thread_key=@key AND a.kind='email' AND `+threadSourceContentSQL+` AND a.id>@after ORDER BY a.id LIMIT 1`, pgx.NamedArgs{threadArgKey: key, threadArgAfter: after}).Scan(&id, &message, &subject, &parent, &persisted, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = tx.Exec(ctx, `UPDATE capture_thread_barrier SET loaded=true WHERE repair_id=@job AND blocked_key=@key`, pgx.NamedArgs{threadArgJob: job.ID, threadArgKey: key})
		return err
	}
	if err != nil {
		return err
	}
	topic := ""
	if persisted != nil {
		topic = *persisted
	} else {
		keys, err := threadHistoricalProjectKeysTx(ctx, tx, id)
		if err != nil {
			return err
		}
		topic = threadBoundaryTopic(subject, keys)
	}
	state := newThreadBoundaryState(message, topic)
	_, err = tx.Exec(ctx, `INSERT INTO capture_thread_repair_node(repair_id,activity_id,source_key,message_key,parent_key,own_topic,ancestor_topic,ancestor_distance,frontier_key,frontier_topic,root_key,observed_version,stage) VALUES(@job,@id,@source,@message,@parent,@topic,@ancestor,@distance,@frontier,@frontiertopic,@message,@version,@message) ON CONFLICT DO NOTHING`, pgx.NamedArgs{threadArgJob: job.ID, "id": id, threadArgSource: key, "message": message, threadArgParent: parent, threadArgTopic: topic, "ancestor": state.AncestorTopic, "distance": state.AncestorDistance, "frontier": state.FrontierKey, "frontiertopic": state.FrontierTopic, "version": version})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE capture_thread_barrier SET after_activity=@after WHERE repair_id=@job AND blocked_key=@key`, pgx.NamedArgs{threadArgJob: job.ID, threadArgKey: key, threadArgAfter: id})
	return err
}

func threadHistoricalProjectKeysTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT unnest(t.recognized_keys) FROM capture_thread_topic t WHERE t.activity_id=@id UNION SELECT p.key FROM activity_link l JOIN project p ON p.id=l.project_id WHERE l.activity_id=@id AND l.entity_type='project' ORDER BY 1`, pgx.NamedArgs{"id": id})
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func discoverThreadRepairUnitTx(ctx context.Context, tx pgx.Tx, job threadRepair, apply ThreadRepairApplier) error {
	var id ids.ActivityID
	var key, parent string
	var after ids.UUID
	err := tx.QueryRow(ctx, `SELECT activity_id,stage,parent_key,child_after FROM capture_thread_repair_node WHERE repair_id=@job AND NOT edges_done ORDER BY activity_id LIMIT 1`, pgx.NamedArgs{threadArgJob: job.ID}).Scan(&id, &key, &parent, &after)
	if errors.Is(err, pgx.ErrNoRows) {
		return threadRepairPhaseTx(ctx, tx, job.ID, threadPhaseTopics)
	}
	if err != nil {
		return err
	}
	if after == ids.Nil {
		if err := discoverThreadParentTx(ctx, tx, job.ID, id, parent, apply); err != nil {
			return err
		}
	}

	var child ids.ActivityID
	var childThread string
	var direct bool
	err = tx.QueryRow(ctx, `SELECT a.id,coalesce(a.thread_key,''),coalesce(`+threadDirectParentSQL+`=@key,false) FROM activity_mail_reference candidate JOIN activity a ON a.id=candidate.activity_id WHERE candidate.referenced_id=@key AND candidate.activity_id>@after ORDER BY candidate.activity_id LIMIT 1`, pgx.NamedArgs{threadArgKey: key, threadArgAfter: after}).Scan(&child, &childThread, &direct)
	if errors.Is(err, pgx.ErrNoRows) {
		return finishThreadAliasTx(ctx, tx, job.ID, id, key)
	}
	if err != nil {
		return err
	}
	allowed := false
	if direct {
		allowed, err = apply.AuthorizeEdge(ctx, tx, child, id)
	}
	if err != nil {
		return err
	}
	if allowed {
		if err := addThreadBarrierTx(ctx, tx, job.ID, childThread); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE capture_thread_repair_node SET child_after=@after WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job.ID, "id": id, threadArgAfter: child})
	if err != nil {
		return err
	}
	return threadRepairPhaseTx(ctx, tx, job.ID, "load")
}

func namedThreadActivityTx(ctx context.Context, tx pgx.Tx, key string) (ids.ActivityID, string, bool, error) {
	var id ids.ActivityID
	var thread string
	err := tx.QueryRow(ctx, `WITH named AS (
 SELECT activity_id FROM activity_identity WHERE identity_kind='mail' AND identity_key=@key
 UNION SELECT id FROM activity WHERE source_system='email' AND source_id=@key
 ) SELECT a.id,coalesce(a.thread_key,'') FROM named n JOIN activity a ON a.id=n.activity_id
 WHERE a.kind='email' AND `+threadSourceContentSQL+` ORDER BY a.id LIMIT 1`, pgx.NamedArgs{threadArgKey: key}).Scan(&id, &thread)
	if errors.Is(err, pgx.ErrNoRows) {
		return id, "", false, nil
	}
	return id, thread, err == nil, err
}

func discoverThreadParentTx(ctx context.Context, tx pgx.Tx, job ids.UUID, id ids.ActivityID, reference string, apply ThreadRepairApplier) error {
	if reference == "" {
		return nil
	}
	parent, key, found, err := namedThreadActivityTx(ctx, tx, reference)
	if err != nil || !found {
		return err
	}
	allowed, err := apply.AuthorizeEdge(ctx, tx, id, parent)
	if err != nil || !allowed {
		return err
	}
	if err := addThreadBarrierTx(ctx, tx, job, key); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE capture_thread_repair_node SET parent_activity_id=@parent WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job, "id": id, threadArgParent: parent})
	return err
}

func finishThreadAliasTx(ctx context.Context, tx pgx.Tx, job ids.UUID, id ids.ActivityID, key string) error {
	var alias string
	err := tx.QueryRow(ctx, `SELECT identity_key FROM activity_identity WHERE activity_id=@id AND identity_kind='mail' AND identity_key>@key ORDER BY identity_key LIMIT 1`, pgx.NamedArgs{"id": id, threadArgKey: key}).Scan(&alias)
	switch {
	case err == nil:
		_, err = tx.Exec(ctx, `UPDATE capture_thread_repair_node SET stage=@alias,child_after=@zero WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job, "id": id, "alias": alias, threadArgZero: ids.Nil})
	case errors.Is(err, pgx.ErrNoRows):
		_, err = tx.Exec(ctx, `UPDATE capture_thread_repair_node SET edges_done=true,stage='verdict' WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job, "id": id})
	default:
		return err
	}
	if err != nil {
		return err
	}
	return threadRepairPhaseTx(ctx, tx, job, "load")
}
