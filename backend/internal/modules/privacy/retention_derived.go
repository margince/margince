// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// deleteDerivedContactRows removes what the system DERIVED about the subject
// and keyed on their contact id.
//
// None is anonymizable, which is what makes the three one step rather than
// three. An embedding is an opaque vector of the text. A provenance row names
// where a field value came from — its source, who captured it, the evidence it
// was read out of — and points at fields the anonymize has just nulled, so what
// identifies the subject IS the record of where they were found. A feedback row
// names this contact as the subject an AI answer was judged about, and the
// judgement cannot be held without them.
func deleteDerivedContactRows(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	// One loop over three literal statements: they are one step, and a copy of
	// the error check per table is three chances to forget one.
	for _, statement := range []string{
		`DELETE FROM embedding WHERE entity_type = 'contact' AND entity_id = $1`,
		`DELETE FROM field_provenance WHERE object_type = 'contact' AND object_id = $1`,
		`DELETE FROM ai_feedback WHERE subject_type = 'contact' AND subject_id = $1`,
	} {
		if _, err := tx.Exec(ctx, statement, id); err != nil {
			return err
		}
	}
	return nil
}
