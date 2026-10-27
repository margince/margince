// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const syncSnapshotMatch = `id=@id AND generation=@generation
 AND sync_revision=@revision AND sync_cursor IS NOT DISTINCT FROM @prior::jsonb
 AND status IN ('connected','error') AND archived_at IS NULL`

type syncSnapshot struct {
	id         ids.UUID
	generation int
	revision   int64
	prior      connector.Cursor
}

func (s syncSnapshot) args() pgx.NamedArgs {
	return pgx.NamedArgs{"id": s.id, "generation": s.generation, "revision": s.revision, "prior": cursorJSON(s.prior)}
}

func cursorJSON(cursor connector.Cursor) []byte {
	if len(cursor) == 0 {
		return nil
	}
	return []byte(cursor)
}
