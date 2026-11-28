// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/recordmerge"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MoveDealRefsTx moves the chat links of a merged-away deal onto its survivor.
// A live link leaves the retired deal's project and takes the survivor's under
// a new link's rule. The project joins when it has no channel of its own and
// the caller may edit it. The old project leaves in the same statement as the
// move. A project both deals share is then not mistaken for one that already
// has a channel.
func MoveDealRefsTx(ctx context.Context, tx pgx.Tx, from, to ids.UUID) ([]recordmerge.Moved, error) {
	if err := moveMessengerRefsTx(ctx, tx, RecordDeal, from, to); err != nil {
		return nil, err
	}
	movedIDs, err := recordmerge.MovedIDs(ctx, tx,
		`UPDATE chat_link SET deal_id = $2,
		        project_id = CASE WHEN archived_at IS NULL THEN NULL ELSE project_id END
		  WHERE deal_id = $1 RETURNING id`, from, to)
	if err != nil {
		return nil, err
	}
	if err := storekit.AuditEach(ctx, tx, "update", chatLinkEntity, movedIDs,
		map[string]any{chatLinkDealColumn: from}, map[string]any{chatLinkDealColumn: to}); err != nil {
		return nil, fmt.Errorf("audit the chat links moving to the merge survivor: %w", err)
	}
	_, project, err := linkedRecords(ctx, tx, RecordDeal, to)
	if err != nil || project == nil || len(movedIDs) == 0 {
		return []recordmerge.Moved{{Table: "chat_link", IDs: movedIDs}}, err
	}
	joined, err := recordmerge.MovedIDs(ctx, tx,
		`UPDATE chat_link SET project_id = $2
		  WHERE id = ANY($1) AND archived_at IS NULL RETURNING id`, movedIDs, *project)
	if err != nil {
		return nil, err
	}
	if err := storekit.AuditEach(ctx, tx, "update", chatLinkEntity, joined,
		map[string]any{chatLinkProjectColumn: nil}, map[string]any{chatLinkProjectColumn: *project}); err != nil {
		return nil, fmt.Errorf("audit the chat links taking the survivor's project: %w", err)
	}
	return []recordmerge.Moved{{Table: "chat_link", IDs: movedIDs}}, nil
}

// chatLinkDealColumn and chatLinkProjectColumn are the columns a chat link
// names its deal and its project in.
const (
	chatLinkDealColumn    = "deal_id"
	chatLinkProjectColumn = "project_id"
)

func moveMessengerRefsTx(ctx context.Context, tx pgx.Tx, kind string, from, to ids.UUID) error {
	args := pgx.StrictNamedArgs{kindKey: kind, "from": from, "target": to}
	rows, err := tx.Query(ctx, `DELETE FROM chat_member_room source WHERE record_type=@kind AND record_id=@from
 AND EXISTS (SELECT 1 FROM chat_member_room target WHERE target.record_type=@kind AND target.record_id=@target
 AND target.grant_id=source.grant_id AND target.grant_version=source.grant_version AND target.roster_hash=source.roster_hash) RETURNING id`, args)
	if err != nil {
		return err
	}
	retired, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return err
	}
	if err := storekit.AuditEach(ctx, tx, "delete", "chat_member_room", retired, map[string]any{recordIDKey: from}, nil); err != nil {
		return err
	}
	for _, statement := range []struct{ table, sql string }{
		{"chat_member_room", `UPDATE chat_member_room SET record_id=@target WHERE record_type=@kind AND record_id=@from RETURNING id`},
		{"chat_send_attempt", `UPDATE chat_send_attempt SET record_id=@target WHERE record_type=@kind AND record_id=@from RETURNING id`},
	} {
		rows, err := tx.Query(ctx, statement.sql, args)
		if err != nil {
			return err
		}
		moved, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		if err != nil {
			return err
		}
		if err := storekit.AuditEach(ctx, tx, "update", statement.table, moved, map[string]any{"record_id": from}, map[string]any{"record_id": to}); err != nil {
			return err
		}
	}
	return nil
}
