// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The stored half of moving the score from a lead's source to its origin: fill
// an origin from the capture a lead was written from, then score every open
// lead again. Both are bounded pages the data task runs through its executor.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// capturedOriginKeys are the payload names a source system words a lead's
// origin under, compared with case and underscores ignored. The payload is a
// connector's own object, so the list is closed: a wording found under any other
// name is left alone rather than guessed at.
var capturedOriginKeys = []string{"leadsource"}

// capturedOriginSQL reads the first of those names out of a retained capture
// that is a JSON object. The names are literals here and nothing a caller sent.
const capturedOriginSQL = `(SELECT btrim(e.value) FROM jsonb_each_text(r.payload) e
	WHERE lower(replace(e.key, '_', '')) = ANY($3) AND btrim(e.value) <> '' LIMIT 1)`

type lacksOrigin struct {
	id   ids.UUID
	text *string
}

// BackfillLeadOriginTx fills the origin of one page of leads from the wording
// their retained capture carries, and only where it resolves to an active
// catalog key. A lead that already has an origin is not selected, so a human's
// choice is never touched and a second run changes nothing. A lead imported from
// a file has no retained capture (the import keeps ids only), so it stays
// without an origin and scores neutral.
func BackfillLeadOriginTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (scanned []ids.UUID, changed int, err error) {
	if err := requireLeadRepair(ctx); err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(ctx, `
		SELECT l.id, `+capturedOriginSQL+`
		  FROM lead l
		  JOIN raw_capture r ON r.source_system = l.source_system AND r.source_id = l.source_id
		 WHERE l.id > $1 AND l.archived_at IS NULL AND l.origin IS NULL AND jsonb_typeof(r.payload) = 'object'
		 ORDER BY l.id LIMIT $2 FOR UPDATE OF l`, after, limit, capturedOriginKeys)
	if err != nil {
		return nil, 0, fmt.Errorf("contacts: reading leads to give an origin: %w", err)
	}
	var page []lacksOrigin
	for rows.Next() {
		var row lacksOrigin
		if err := rows.Scan(&row.id, &row.text); err != nil {
			rows.Close()
			return nil, 0, err
		}
		page = append(page, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	for _, row := range page {
		scanned = append(scanned, row.id)
		if row.text == nil {
			continue
		}
		key, ok, err := ResolveOrigin(ctx, tx, *row.text)
		if err != nil {
			return nil, 0, err
		}
		if !ok {
			continue
		}
		if err := auth.EnsureWritableLive(ctx, tx, leadEntity, row.id); err != nil {
			return nil, 0, err
		}
		if err := setBackfilledOrigin(ctx, tx, row.id, key); err != nil {
			return nil, 0, err
		}
		changed++
	}
	return scanned, changed, nil
}

func setBackfilledOrigin(ctx context.Context, tx pgx.Tx, id ids.UUID, key string) error {
	tag, err := tx.Exec(ctx, `UPDATE lead SET origin = $2 WHERE id = $1 AND origin IS NULL AND archived_at IS NULL`, id, key)
	if err != nil {
		return fmt.Errorf("contacts: giving a lead its origin: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	auditID, err := storekit.Audit(ctx, tx, "update", "lead", id,
		map[string]any{leadOriginField: nil}, map[string]any{leadOriginField: key})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, id, crmcontracts.PublicEventLeadUpdated{
		ChangedFields: map[string]any{eventKeyDelta: map[string]any{leadOriginField: key}},
	})
}

// CountLeadOriginBackfills counts, up to limit+1, the leads the backfill will
// inspect.
func CountLeadOriginBackfills(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, limit int64,
) (int64, error) {
	if err := auth.Require(ctx, leadEntity, principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (
		SELECT 1 FROM lead l
		  JOIN raw_capture r ON r.source_system = l.source_system AND r.source_id = l.source_id
		 WHERE l.archived_at IS NULL AND l.origin IS NULL AND jsonb_typeof(r.payload) = 'object' LIMIT $1) pending`, limit+1).Scan(&count)
	return count, err
}

// RescoreOpenLeadsTx scores one page of open leads again from their origin and
// signals. It reads the page unlocked and holds each lead as it scores it, so the
// subject is the first row it locks. It goes through the recompute the activity events use, which writes
// the history entry, audit row and event, moves only score_computed under a
// human's override, and writes nothing for a score that is already right, so a
// second run changes nothing.
func RescoreOpenLeadsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, now time.Time) (scanned []ids.UUID, changed int, err error) {
	if err := requireLeadRepair(ctx); err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id FROM lead
		 WHERE id > $1 AND archived_at IS NULL AND status = ANY($3)
		 ORDER BY id LIMIT $2`, after, limit, openLeadStatusNames())
	if err != nil {
		return nil, 0, fmt.Errorf("contacts: reading leads to score: %w", err)
	}
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, 0, err
		}
		scanned = append(scanned, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	for _, id := range scanned {
		if err := auth.HoldWritableLive(ctx, tx, leadEntity, id); err != nil {
			return nil, 0, err
		}
		before, err := leadScoreMarks(ctx, tx, id)
		if err != nil {
			return nil, 0, err
		}
		if err := recomputeLeadScoreTx(ctx, tx, ids.From[ids.LeadKind](id), now); err != nil {
			return nil, 0, fmt.Errorf("contacts: scoring lead %s again: %w", id, err)
		}
		after, err := leadScoreMarks(ctx, tx, id)
		if err != nil {
			return nil, 0, err
		}
		if before != after {
			changed++
		}
	}
	return scanned, changed, nil
}

// CountOpenLeads counts, up to limit+1, the leads the rescore will inspect.
func CountOpenLeads(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, limit int64,
) (int64, error) {
	if err := auth.Require(ctx, leadEntity, principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (
		SELECT 1 FROM lead WHERE archived_at IS NULL AND status = ANY($1) LIMIT $2) open`,
		openLeadStatusNames(), limit+1).Scan(&count)
	return count, err
}

// leadScoreMarks is the pair of numbers a rescore can move, as one comparable
// string so "did it change" is a single equality.
func leadScoreMarks(ctx context.Context, tx pgx.Tx, id ids.UUID) (string, error) {
	if err := auth.Require(ctx, leadEntity, principal.ActionRead); err != nil {
		return "", err
	}
	var score int
	var computed *int
	if err := tx.QueryRow(ctx, `SELECT score, score_computed FROM lead WHERE id = $1`, id).Scan(&score, &computed); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	marks, err := json.Marshal([]any{score, computed})
	return string(marks), err
}

func openLeadStatusNames() []string {
	out := make([]string, 0, len(openLeadStatuses))
	for _, status := range openLeadStatuses {
		out = append(out, string(status))
	}
	return out
}

func requireLeadRepair(ctx context.Context) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	return auth.Require(ctx, leadEntity, principal.ActionUpdate)
}
