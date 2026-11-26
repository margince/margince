// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// FieldCatalog is the custom-field catalog a record store reads. ActiveColumnsIn
// reads on the store's own transaction, so an entry point takes one connection;
// it lives here because the shared port may not name pgx.
type FieldCatalog interface {
	fieldcatalog.Reader
	ActiveColumnsIn(ctx context.Context, tx pgx.Tx, object string) ([]fieldcatalog.Column, error)
}
