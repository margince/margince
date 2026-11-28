// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const providerCopyColumns = ` (SELECT count(DISTINCT post_id) FROM chat_post_item WHERE link_id=l.id AND post_id IS NOT NULL AND redacted_at IS NULL)+CASE WHEN coalesce(card_message_id,'')<>'' THEN 1 ELSE 0 END,
 (SELECT count(DISTINCT post_id) FROM chat_post_item WHERE link_id=l.id AND redacted_at IS NOT NULL),
 (SELECT count(DISTINCT post_id) FROM chat_post_item WHERE link_id=l.id AND withdrawn_at IS NOT NULL AND message_id IS NOT NULL),
 (SELECT count(*) FROM chat_bot_delivery WHERE link_id=l.id AND state IN ('sending','unknown','reviewed_unknown'))`

// EndedSync reports retained coverage after a record's channel synchronization ends.
type EndedSync struct {
	Provider              string
	Ended                 time.Time
	ReadThrough           *time.Time
	LastPosted            *time.Time
	RetainedMessages      int
	ConfirmedRedactions   int
	UnverifiedWithdrawals int
	UnconfirmedDeliveries int
}

func lastEndedSync(ctx context.Context, tx pgx.Tx, kind string, record ids.UUID) (*EndedSync, error) {
	column, err := recordColumn(kind)
	if err != nil {
		return nil, err
	}
	var out EndedSync
	err = tx.QueryRow(ctx, `SELECT provider,archived_at,native_coverage_at,
 (SELECT max(posted_at) FROM chat_post_item WHERE link_id=l.id),
`+providerCopyColumns+`
 FROM chat_link l WHERE `+column+`=@record AND archived_at IS NOT NULL ORDER BY archived_at DESC,id DESC LIMIT 1`, pgx.StrictNamedArgs{recordKey: record}).Scan(&out.Provider, &out.Ended, &out.ReadThrough, &out.LastPosted, &out.RetainedMessages, &out.ConfirmedRedactions, &out.UnverifiedWithdrawals, &out.UnconfirmedDeliveries)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // A record without an ended link has no retained-sync status.
	}
	return &out, err
}

func readProviderCopies(ctx context.Context, tx pgx.Tx, link ids.UUID) (ProviderCopies, error) {
	var out ProviderCopies
	err := tx.QueryRow(ctx, `SELECT `+providerCopyColumns+` FROM chat_link l WHERE id=@link`, pgx.StrictNamedArgs{linkKey: link}).Scan(&out.RetainedMessages, &out.ConfirmedRedactions, &out.UnverifiedWithdrawals, &out.UnconfirmedDeliveries)
	return out, err
}

// ProviderCopies distinguishes retained, redacted and unverified provider copies.
type ProviderCopies struct {
	RetainedMessages      int
	ConfirmedRedactions   int
	UnverifiedWithdrawals int
	UnconfirmedDeliveries int
}
