// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const rootColumns = `id,root_id,revision,applied_revision,content_digest,activity_id,source_deleted,notice_revision,last_error,retry_at`

const rootUnchangedSQL = `revision=@revision AND content_digest IS NOT DISTINCT FROM @prior_digest
 AND notice_revision IS NOT DISTINCT FROM @notice AND last_error IS NOT DISTINCT FROM @prior_error
 AND retry_at IS NOT DISTINCT FROM @prior_retry AND applied_revision IS NOT DISTINCT FROM @prior_applied
 AND activity_id IS NOT DISTINCT FROM @prior_activity AND source_deleted=@prior_deleted`

func rootExpected(root RootState, link ids.UUID) pgx.StrictNamedArgs {
	return pgx.StrictNamedArgs{
		"id": root.ID, linkKey: link, revisionKey: root.Revision, "prior_digest": root.Digest,
		"notice": root.Notice, "prior_error": root.Error, "prior_retry": root.Retry, "prior_applied": root.Applied,
		"prior_activity": root.Activity, "prior_deleted": root.Deleted,
	}
}

func scanRootState(row pgx.CollectableRow) (RootState, error) {
	var root RootState
	err := row.Scan(&root.ID, &root.Root, &root.Revision, &root.Applied, &root.Digest, &root.Activity, &root.Deleted, &root.Notice, &root.Error, &root.Retry)
	return root, err
}

func rootImage(root RootState) map[string]any {
	return map[string]any{rootIDKey: root.Root, revisionKey: root.Revision, "applied_revision": root.Applied, "content_digest": root.Digest, "activity_id": root.Activity, "source_deleted": root.Deleted, "notice_revision": root.Notice, lastErrorKey: root.Error, retryAtKey: root.Retry}
}

func stageRootTx(ctx context.Context, tx pgx.Tx, link Link, root chatwire.RootRef) error {
	if err := validateRootReference(link, root); err != nil {
		return err
	}
	if err := lockRootTx(ctx, tx, link.ID, root.ID); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT `+rootColumns+` FROM chat_root WHERE link_id=@link AND root_id=@root FOR UPDATE`, pgx.StrictNamedArgs{linkKey: link.ID, rootKey: root.ID})
	if err != nil {
		return err
	}
	before, err := pgx.CollectOneRow(rows, scanRootState)
	if errors.Is(err, pgx.ErrNoRows) {
		after := RootState{Root: root.ID, Revision: root.Revision, Deleted: root.Deleted}
		if err := tx.QueryRow(ctx, `INSERT INTO chat_root(link_id,root_id,revision,source_deleted)
   VALUES(@link,@root,@revision,@deleted) RETURNING id`, pgx.StrictNamedArgs{linkKey: link.ID, rootKey: root.ID, revisionKey: root.Revision, deletedKey: root.Deleted}).Scan(&after.ID); err != nil {
			return err
		}
		_, err = storekit.Audit(ctx, tx, "create", "chat_root", after.ID, nil, rootImage(after))
		return err
	}
	if err != nil {
		return err
	}
	root.Deleted = root.Deleted || before.Deleted
	if before.Revision == root.Revision && before.Deleted == root.Deleted {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE chat_root SET revision=@revision,source_deleted=@deleted WHERE id=@id`, pgx.StrictNamedArgs{revisionKey: root.Revision, "deleted": root.Deleted, "id": before.ID}); err != nil {
		return err
	}
	after := before
	after.Revision, after.Deleted = root.Revision, root.Deleted
	_, err = storekit.Audit(ctx, tx, "update", "chat_root", before.ID, rootImage(before), rootImage(after))
	return err
}

func lockRootTx(ctx context.Context, tx pgx.Tx, link ids.UUID, root string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(@key,0))`, pgx.StrictNamedArgs{keyKey: "chat-root:" + link.String() + ":" + root})
	return err
}

func validateRootReference(link Link, root chatwire.RootRef) error {
	if root.ID == "" || root.Revision == "" || len(root.ID) > 1024 || len(root.Revision) > 128 || link.RootID != "" && link.RootID != root.ID {
		return apperrors.ErrInvalidArgument
	}
	return nil
}
