// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// Filing an import's created records under one word, so a batch stays findable
// as a batch — "the K5 conference list", "the January partner file".

// parseContextTag reads the run's stored word, answering the zero id for none.
func parseContextTag(raw string) ids.TagID {
	if raw == "" {
		return ids.TagID{}
	}
	parsed, err := ids.Parse(raw)
	if err != nil {
		return ids.TagID{}
	}
	return ids.TagID{UUID: parsed}
}

// fileUnderContextTag applies the run's word to a record it just created.
//
// Inside the create's own transaction: a record and the tag that files it land
// together or neither lands, because a crash between the two leaves a row in
// the estate that the batch it belongs to cannot find — and nothing afterwards
// knows to look for it.
func (w *csvWriters) fileUnderContextTag(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	if w.contextTag.IsZero() {
		return nil
	}
	entityType, ok := taggableObjectOf(w.object)
	if !ok {
		// The importer writes objects the tag surface does not carry (a lead is
		// one). Filing them is not refused, it is simply not offered — and a
		// refusal here would fail a whole run over a word nobody could apply.
		return nil
	}
	_, err := w.tags.ApplyTagTx(ctx, tx, w.contextTag, entityType, id)
	if errors.Is(err, apperrors.ErrConflict) {
		// The record already carries the word: a resumed run replaying its last
		// page. Not a failure — the batch is filed exactly as asked.
		return nil
	}
	if err != nil {
		return fmt.Errorf("import: filing %s under the run's tag: %w", id, err)
	}
	return nil
}

// taggableObjectOf maps an import object onto the tag surface's own name for
// it, and says when there is none.
func taggableObjectOf(object string) (string, bool) {
	// The importer's objects are `organization`, `person` and `lead`, and the
	// taggable set is derived from the canonical record vocabulary, which holds
	// all three under those same names. Asked rather than restated, so an
	// object added to either side does not need this switch remembered.
	for _, known := range datasource.RecordTypes() {
		if string(known) == object {
			return object, true
		}
	}
	return "", false
}
