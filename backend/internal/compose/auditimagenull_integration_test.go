// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An absent audit image reaches the column as SQL NULL, and the JSON scalar
// `null` cannot get in at all.
//
// The two look alike and answer differently. Every "there was no prior state"
// query in this tree is `WHERE before IS NULL`, and a column holding the four
// bytes `null` is not null — it misses the row, silently, in exactly the place
// a reader is trying to establish that nothing preceded a change.
//
// storekit.AbsentImage answers this correctly at the door it owns. The
// constraint is here because not every writer goes through that door: the
// approvals module INSERTs its own row, and a caller that marshals an image
// itself hands over bytes the seam never sees.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// writeAuditImage inserts one audit row with the given before/after literals,
// as a direct INSERT — the shape that does not pass storekit at all, which is
// the whole reason the refusal lives in the table.
func writeAuditImage(t *testing.T, e *integration.Env, before, after string) error {
	t.Helper()
	return database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id,
			                       before, after, occurred_at)
			VALUES ('human', 'user:'||$1::text, 'update', 'person', $2, nullif($3,'')::jsonb, nullif($4,'')::jsonb, $5)`,
			ids.NewV7(), ids.NewV7(), before, after, time.Now().UTC())
		return err
	})
}

func TestAnAuditImageCannotBeTheJSONScalarNull(t *testing.T) {
	e := integration.Setup(t)

	// The absent image, which is the shape every reader's `IS NULL` is written
	// for. Asserted first: a constraint that refused this would break the
	// honest case rather than the dishonest one.
	if err := writeAuditImage(t, e, "", `{"full_name":"Sara"}`); err != nil {
		t.Fatalf("an absent before-image was refused: %v — SQL NULL is what an image that says "+
			"nothing is supposed to be", err)
	}

	for _, tc := range []struct{ name, before, after string }{
		{"a null before-image", "null", `{"full_name":"Sara"}`},
		{"a null after-image", `{"full_name":"Sara"}`, "null"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := writeAuditImage(t, e, tc.before, tc.after)
			if err == nil {
				t.Fatal("the JSON scalar null was stored — every `WHERE before IS NULL` reader " +
					"now misses this row while the column claims to carry an image")
			}
			if !strings.Contains(err.Error(), "audit_log_images_are_absent_or_present") {
				t.Errorf("refused by %v, want the image constraint — a different refusal means this "+
					"case is passing for a reason that has nothing to do with what it asserts", err)
			}
		})
	}
}
