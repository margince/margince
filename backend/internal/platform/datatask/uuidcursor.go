// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datatask

import (
	"encoding/json"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// UUIDCursor decodes the position used by corrections that scan a UUID primary key.
func UUIDCursor(cursor json.RawMessage) (ids.UUID, error) {
	var after ids.UUID
	if len(cursor) == 0 {
		return after, nil
	}
	if err := json.Unmarshal(cursor, &after); err != nil {
		return ids.Nil, &storekit.MalformedCursorError{}
	}
	return after, nil
}

// AdvanceUUID advances over every scanned row, including those that stayed unchanged.
func AdvanceUUID(in BatchIn, scanned []ids.UUID, changed int) (BatchOut, error) {
	out := BatchOut{Cursor: in.Cursor, Scanned: len(scanned), Changed: changed, Done: len(scanned) < in.Limit}
	if len(scanned) == 0 {
		return out, nil
	}
	cursor, err := json.Marshal(scanned[len(scanned)-1])
	if err != nil {
		return BatchOut{}, err
	}
	out.Cursor = cursor
	return out, nil
}
