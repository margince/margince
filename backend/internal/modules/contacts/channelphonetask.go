// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ChannelPhonesDataTask fills the phone fields of contacts whose WhatsApp
// identity predates the runtime writer. Existing phone values stay.
func ChannelPhonesDataTask() datatask.Task {
	return datatask.Task{ID: "core/1795756057_channel_phones", Version: 1, Requires: []string{"core/1795639491", "core/1795772241"}, Title: systemtext.Literal("datatask.title.core.1795756057_channel_phones"), Steps: []datatask.Step{{
		Name: "phones", Limit: 100, MinLimit: 1,
		Estimate: estimateChannelPhones, Batch: materializeChannelPhonesBatch,
		Audit: datatask.AuditPolicy{PerRecord: true},
	}}}
}

func estimateChannelPhones(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT id FROM contact_channel_identity ORDER BY id LIMIT @cap) candidates`, pgx.NamedArgs{"cap": maximum + 1}).Scan(&count)
	return min(count, maximum), count > maximum, err
}

func materializeChannelPhonesBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT id, contact_id, provider, account_scope, channel_user_id, archived_at IS NOT NULL
 FROM contact_channel_identity WHERE id>@after ORDER BY id LIMIT @limit`, pgx.NamedArgs{afterCursorArg: after, signaturePhoneLimitParam: in.Limit})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	type binding struct {
		ID                    ids.UUID
		Contact               ids.ContactID
		Provider, Scope, User string
		Archived              bool
	}
	page, err := pgx.CollectRows(rows, pgx.RowToStructByPos[binding])
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned := make([]ids.UUID, 0, len(page))
	for _, row := range page {
		scanned = append(scanned, row.ID)
	}
	slices.SortFunc(page, func(a, b binding) int { return strings.Compare(a.Contact.String(), b.Contact.String()) })
	changed := 0
	for _, row := range page {
		identity := connector.ChannelIdentity{Provider: row.Provider, AccountScope: row.Scope, ChannelUserID: row.User}
		if row.Archived || whatsAppPhone(identity) == "" {
			continue
		}
		if err := lockContactForAttach(ctx, tx, row.Contact); err != nil {
			if errors.Is(err, apperrors.ErrNotFound) {
				continue
			}
			return datatask.BatchOut{}, err
		}
		wrote, err := materializeChannelPhone(ctx, tx, row.Contact, identity)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if wrote {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, scanned, changed)
}
