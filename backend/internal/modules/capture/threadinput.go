// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func rememberThreadInputTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, seat ids.UUID) (bool, error) {
	tag, err := tx.Exec(ctx, `UPDATE capture_import ci SET thread_input_hash=fingerprint.value FROM (
 SELECT md5(jsonb_build_array(a.subject,a.archived_at,a.restricted_at,t.own_topic,
 coalesce(`+threadDirectParentSQL+`,''),
 (SELECT i.identity_key FROM activity_identity i WHERE i.activity_id=a.id AND i.identity_kind='mail' ORDER BY i.identity_key LIMIT 1))::text) AS value
 FROM activity a JOIN capture_thread_topic t ON t.activity_id=a.id WHERE a.id=@id
 ) fingerprint WHERE ci.activity_id=@id AND ci.user_id=@seat AND ci.thread_input_hash IS DISTINCT FROM fingerprint.value`, pgx.NamedArgs{"id": id, threadArgSeat: seat})
	return tag.RowsAffected() > 0, err
}
