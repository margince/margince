// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What an opened deal rests on, asked again under the opening's own locks. The
// cited evidence must still be readable by the whole workspace, and an amount
// must be stated in the document's own words.

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/dealverdict"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// scoutEvidenceHolds takes FOR SHARE on every cited item and the activities
// behind it, then asks Deal Scout's visibility rule of each again.
func scoutEvidenceHolds(ctx context.Context, tx pgx.Tx, evidence []deals.SuggestionEvidence) (bool, error) {
	for _, item := range evidence {
		activities, ok, err := lockCitedItem(ctx, tx, item)
		if err != nil || !ok {
			return false, err
		}
		for _, activity := range activities {
			held, err := workspaceReadable(ctx, tx, activity)
			if err != nil || !held {
				return false, err
			}
		}
	}
	return true, nil
}

// lockCitedItem holds one cited item and answers the activities it rests on.
func lockCitedItem(ctx context.Context, tx pgx.Tx, item deals.SuggestionEvidence) ([]ids.UUID, bool, error) {
	var query string
	var ref ids.UUID
	switch {
	case item.ActivityID != nil:
		return []ids.UUID{*item.ActivityID}, true, nil
	case item.SignalID != nil:
		ref = *item.SignalID
		query = `SELECT array(SELECT (` + storekit.CitedActivityID("cite") + `)::uuid
		          FROM jsonb_array_elements(s.evidence) cite WHERE cite->>'source_type' = 'activity')
		  FROM signal s
		 WHERE s.id = $1 AND s.status = 'open' AND s.archived_at IS NULL AND s.visibility = 'workspace'
		   FOR SHARE OF s`
	case item.AttachmentID != nil:
		ref = *item.AttachmentID
		query = `SELECT ARRAY[at.activity_id] FROM attachment at
		 WHERE at.id = $1 AND at.archived_at IS NULL AND NOT at.bytes_withheld
		   FOR SHARE OF at`
	default:
		return nil, false, nil
	}
	var activities []ids.UUID
	err := tx.QueryRow(ctx, query, ref).Scan(&activities)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("compose: holding the evidence a deal opens on: %w", err)
	}
	return activities, true, nil
}

// workspaceReadable holds one activity and its links, and asks whether the
// whole workspace may still read it.
func workspaceReadable(ctx context.Context, tx pgx.Tx, activity ids.UUID) (bool, error) {
	if _, err := tx.Exec(ctx, `SELECT 1 FROM activity_link WHERE activity_id = $1 FOR SHARE`, activity); err != nil {
		return false, fmt.Errorf("compose: holding the evidence's links: %w", err)
	}
	var readable bool
	err := tx.QueryRow(ctx, `SELECT `+workspaceEvidence("a")+` FROM activity a WHERE a.id = $1 FOR SHARE OF a`, activity).Scan(&readable)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("compose: rereading the evidence's audience: %w", err)
	}
	return readable, nil
}

// scoutStatedAmount is the amount the newest cited document's current reading
// states in its own quoted words, with its currency. A reading whose quote is
// missing or names another figure gives no amount.
func scoutStatedAmount(ctx context.Context, tx pgx.Tx, evidence []deals.SuggestionEvidence) (*int64, *string, error) {
	for _, item := range evidence {
		if item.AttachmentID == nil {
			continue
		}
		var value, quote, currency *string
		err := tx.QueryRow(ctx, `
			SELECT (SELECT f->>'Value' FROM jsonb_array_elements(x.fields) f
			         WHERE f->>'Field' = 'amount_minor' AND NOT coalesce((f->>'Omitted')::boolean, false)),
			       (SELECT f->>'SourceQuote' FROM jsonb_array_elements(x.fields) f
			         WHERE f->>'Field' = 'amount_minor' AND NOT coalesce((f->>'Omitted')::boolean, false)),
			       (SELECT f->>'Value' FROM jsonb_array_elements(x.fields) f
			         WHERE f->>'Field' = 'currency' AND NOT coalesce((f->>'Omitted')::boolean, false))
			  FROM attachment_extraction x
			 WHERE x.attachment_id = $1 AND x.status = 'done' AND x.question = 'deal_facts'
			 ORDER BY x.finished_at DESC LIMIT 1
			   FOR SHARE OF x`, *item.AttachmentID).Scan(&value, &quote, &currency)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("compose: rereading the document's amount: %w", err)
		}
		if value == nil || currency == nil {
			continue
		}
		return quoteBackedAmount(*value, *currency, quote)
	}
	return nil, nil, nil
}

// quoteBackedAmount keeps a read amount only when the quote states that very
// number in that very currency.
func quoteBackedAmount(value, currency string, quote *string) (*int64, *string, error) {
	if quote == nil {
		return nil, nil, nil
	}
	minor, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, nil, nil
	}
	stated, statedCurrency, ok := dealverdict.QuotedPrice(*quote)
	if !ok || stated != minor || statedCurrency != currency {
		return nil, nil, nil
	}
	return &minor, &currency, nil
}
