// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The breadcrumb a refused record leaves.
//
// A unit moves its cursor past a record the core's grammar cannot express —
// stopping on one malformed message parks the whole connection — so without
// this the drop exists only in that unit's own logs, and a provider format
// change that made EVERY record unrepresentable presented to the installation
// exactly like a healthy quiet feed.
//
// The class is written and the sentence is not. Validate quotes the record back
// to say what is wrong with it, so its text is third-party content; the class
// (extension.RecordRefusal) is the core's own closed vocabulary. Counting per
// unit per class per day keeps a connector refusing a million records to one
// row a day rather than a million.

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/pkg/extension"
)

// noteRefusal records one refused record against the invoking unit.
//
// It is NOT allowed to fail the ingest. The record is already refused and the
// unit is already moving past it; turning a bookkeeping failure into an ingest
// error would stall a connector over the write that exists to report that the
// connector is fine. So the failure is logged at Error — which is itself an
// operator-visible trace, and the only one there was before this table — and
// the disposition is returned either way.
func (r *callRuntime) noteRefusal(ctx context.Context, class extension.RecordRefusal) {
	// A Runtime that was never bound to a database (BindExtensionRuntime) has
	// no installation to record against. It is not a state a serving role
	// reaches — both boot paths bind before anything can be invoked — and the
	// write it would have done is proved on the real path by
	// TestARefusedRecordIsCountedAgainstTheUnitThatSentIt, so this is a
	// precondition rather than a branch that could hide one.
	if r.deps.pool == nil {
		return
	}
	// The INVOCATION's tenant, the same one every other capability re-derives,
	// and bound here rather than by the caller because the grammar check runs
	// before Ingest scopes anything — deliberately, so a malformed record is
	// refused before any authority is spent.
	scoped, err := r.scoped(ctx)
	if err == nil {
		err = database.WithWorkspaceTx(scoped, r.deps.pool, func(tx pgx.Tx) error {
			return recordIngestRefusal(scoped, tx, r.unit, class, time.Now().UTC())
		})
	}
	if err != nil {
		slog.ErrorContext(ctx, "recording an ingest refusal failed",
			"unit", r.unit, "refusal", string(class), "err", err)
	}
}

// recordIngestRefusal is the upsert: one row per unit per class per day,
// carrying the count and the window it spans.
func recordIngestRefusal(ctx context.Context, tx pgx.Tx, unit string, class extension.RecordRefusal, at time.Time) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO extension_ingest_refusal (unit, refusal, day, refused, first_at, last_at)
		VALUES ($1, $2, $3::date, 1, $4, $4)
		ON CONFLICT (unit, refusal, day) DO UPDATE
		   SET refused = extension_ingest_refusal.refused + 1,
		       last_at = EXCLUDED.last_at`,
		unit, string(class), at, at)
	return err
}
