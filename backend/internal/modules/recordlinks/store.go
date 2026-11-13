// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package recordlinks

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// nameColumns is the column that names each linkable record on its table. The
// keys are table names and are formatted into SQL. Every key that reaches a
// query has passed recordkinds.IsLinkEnd first.
var nameColumns = map[string]string{"deal": "name", "company": "display_name", "contact": "full_name"}

// listCap bounds one record's links on a read. A record is linked a handful of
// times; past this the screen is no longer the place to read them.
const listCap = 200

// NewLink is one link to make.
type NewLink struct {
	FromType string
	FromID   ids.UUID
	ToType   string
	ToID     ids.UUID
	LinkType string
}

// Store owns the record_link table.
type Store struct {
	db *database.DB
}

// NewStore binds the store to db.
func NewStore(db *database.DB) *Store {
	return &Store{db: db}
}

func (s *Store) tx(ctx context.Context, fn func(pgx.Tx) error) error {
	return s.db.Tx(ctx, fn)
}
