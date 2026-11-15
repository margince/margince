// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// attachContactChannels reads a page's emails, phones, social handles and
// channel reachability. The four reads share one batch on the caller's
// transaction, so they cost one round trip instead of four.
//
// The ids come from a page the caller already authorised; this adds no row check.
// Each Query callback owns its rows, and Close drains every result and reports
// the first error, so a failed read never leaves results open on the connection.
func attachContactChannels(ctx context.Context, tx pgx.Tx, idx map[openapi_types.UUID]*crmcontracts.Contact, contactIDs []ids.UUID) error {
	if len(contactIDs) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	batch.Queue(`SELECT contact_id, id, email, email_type, is_primary, position, source, captured_by
		 FROM contact_email WHERE contact_id = ANY($1) AND archived_at IS NULL`+
		ReachableEmailOrder, contactIDs).Query(func(rows pgx.Rows) error {
		return attachContactEmails(rows, idx)
	})
	batch.Queue(`SELECT contact_id, id, phone, phone_type, is_primary, position, source, captured_by
		 FROM contact_phone WHERE contact_id = ANY($1) AND archived_at IS NULL
		 ORDER BY position, created_at`, contactIDs).Query(func(rows pgx.Rows) error {
		return attachContactPhones(rows, idx)
	})
	// The wire keeps social as the (platform → handle) map; the relation
	// is the stored form.
	batch.Queue(`SELECT contact_id, platform, handle FROM contact_social WHERE contact_id = ANY($1)
		 ORDER BY platform`, contactIDs).Query(func(rows pgx.Rows) error {
		return attachContactSocial(rows, idx)
	})
	batch.Queue(`SELECT contact_id, provider, blocked_at, created_at
		 FROM contact_channel_identity WHERE contact_id = ANY($1) AND archived_at IS NULL
		 ORDER BY provider, created_at`, contactIDs).Query(func(rows pgx.Rows) error {
		return attachContactReachability(rows, idx)
	})
	return tx.SendBatch(ctx, batch).Close()
}
