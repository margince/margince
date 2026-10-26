// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type recoverableMailBody struct {
	ID      ids.UUID
	Version int64
	Body    string
	Payload []byte
}

// recoverableMailBodyScope is captured mail not yet known to be whole whose
// stored original survives. Whether its body is short is decided per row, by
// the parser, from that original.
const recoverableMailBodyScope = `FROM activity a ` + mailOriginalJoin + `
 WHERE a.kind = 'email' AND a.body_completeness IN ('unassessed', 'unreadable', 'partial')
   AND a.restricted_at IS NULL AND a.archived_at IS NULL AND a.body IS NOT NULL
   AND rc.payload IS NOT NULL AND ` + capturedByMailConnector

// walkUUIDBatch runs one data-task page over locked rows. The query selects
// the page with the @page_after and @page_limit arguments it is given. Every
// scanned row advances the cursor; apply reports whether it wrote one.
func walkUUIDBatch[T any](
	ctx context.Context, tx pgx.Tx, in datatask.BatchIn, query string,
	idOf func(T) ids.UUID, apply func(context.Context, pgx.Tx, T) (bool, error),
) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, query, pgx.NamedArgs{"page_after": after, "page_limit": in.Limit})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	candidates, err := pgx.CollectRows(rows, pgx.RowToStructByPos[T])
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned := make([]ids.UUID, 0, len(candidates))
	changed := 0
	for _, candidate := range candidates {
		scanned = append(scanned, idOf(candidate))
		written, err := apply(ctx, tx, candidate)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if written {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, scanned, changed)
}

func restoreMailBodiesBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	return walkUUIDBatch(ctx, tx, in, `SELECT a.id, a.version, a.body, rc.payload `+recoverableMailBodyScope+`
		AND a.id > @page_after ORDER BY a.id LIMIT @page_limit FOR UPDATE OF a`,
		func(c recoverableMailBody) ids.UUID { return c.ID }, restoreMailBody)
}

// restoreMailBody writes the body the original yields, when the stored one is
// the short shape an older parser or cap left. The conversation it is on was
// read from the old text, so it is made due again.
func restoreMailBody(ctx context.Context, tx pgx.Tx, candidate recoverableMailBody) (bool, error) {
	raw, readable := decodedMailOriginal(candidate.Payload)
	if !readable {
		return false, nil
	}
	body, completeness, ok := mailmap.RecoverStoredBody(raw, candidate.Body)
	if !ok {
		return false, nil
	}
	return writeMailRestoration(ctx, tx, activities.MailBodyRestoration{
		Activity: candidate.ID, Version: candidate.Version, StoredBody: candidate.Body,
		Body: body, Completeness: string(completeness),
	})
}

// writeMailRestoration is the one write every mail restoration takes, so a
// conversation read from replaced text is made due again by each of them.
func writeMailRestoration(ctx context.Context, tx pgx.Tx, restoration activities.MailBodyRestoration) (bool, error) {
	restored, threadKey, err := activities.RestoreMailBodyTx(ctx, tx, restoration)
	if err != nil || !restored || threadKey == nil {
		return restored, err
	}
	if err := markConversationDueTx(ctx, tx, *threadKey); err != nil {
		return false, fmt.Errorf("mail body restore: %w", err)
	}
	return true, nil
}

func estimateRecoverableMailBodies(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, "SELECT count(*) FROM (SELECT a.id "+recoverableMailBodyScope+" LIMIT @recoverable_cap) candidates",
		pgx.NamedArgs{"recoverable_cap": maximum + 1}).Scan(&count)
	return min(count, maximum), count > maximum, err
}
