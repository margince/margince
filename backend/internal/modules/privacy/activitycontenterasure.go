// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// Everything an activity's text leaves behind, destroyed in one place.
//
// Its own file rather than a helper beside one of its callers, and the reason
// is the census that reads this package. gates/piicoverage_test.go names the
// files the nightly sweep destroys through, and this belongs on that list: it
// IS the sweep's destructive vocabulary for an activity's content. The
// restriction LIFT does not belong there — it answers to a different contract,
// completing a suspended erasure rather than applying a retention window, and
// it destroys counterparty_email where the retention action deliberately keeps
// it. Two acts, two contracts, and a file each so the census can tell them
// apart.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// purgeContentDerivedFrom destroys everything an activity's text left behind:
// the verbatim provider original, the vectors, the field-level provenance of
// text that is now gone, the transcript readings, the proposals that quote it,
// the attachments, and the transmitted copy in the send log.
//
// ONE implementation, called by every arm that erases an activity's content —
// the nightly erase action, a statutory floor expiring, and a controller's
// release. The file already stated the invariant and enforced it with a second
// list: "a record must not be more thoroughly erased by the clock than by a
// controller's decision". Two lists is how it stopped being true. The lift arm
// destroyed the body and left the raw_capture row standing, joined on the
// (source_system, source_id) pair the lift deliberately keeps — so an Art. 15
// export served the original back, on a record whose floor had expired, with no
// erasure request anywhere in the chain. It also left the proposals quoting the
// body, which the transcript sweep never revisits: its selector requires a
// body, and the lift removed it.
//
// A nil purger REFUSES rather than skipping. There is no second path that ages
// raw_capture out — the Art. 17 cascade's purge is scoped to a PERSON where a
// retention window is scoped to time — so an unwired seam is not a degraded
// mode, it is an erasure that reports success over an intact original.
func (e *Eraser) purgeContentDerivedFrom(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	if e.purgeRawCaptures == nil {
		return fmt.Errorf("%w: this eraser was built without a raw-capture purger, so it can destroy "+
			"an activity's text and not the provider original behind it", ErrRetentionSeamMissing)
	}
	// FIRST, because it is the copy an export reaches when everything else has
	// been done and this has not. Ordering buys nothing on the success path —
	// one transaction — and on the failure path it is the one that must not be
	// the step nobody got to.
	if err := e.purgeRawCaptures(ctx, tx, []ids.UUID{id}); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM embedding WHERE entity_type = 'activity' AND entity_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM field_provenance WHERE object_type = 'activity' AND object_id = $1`, id); err != nil {
		return err
	}
	if err := purgeTranscriptReadings(ctx, tx, []ids.UUID{id}); err != nil {
		return err
	}
	if err := redactApprovalsCitingActivities(ctx, tx, []ids.UUID{id}, AgedOutSourceWithdrawal); err != nil {
		return err
	}
	if err := e.eraseAttachments(ctx, tx, "retention: the record's content was erased", causeRetention, `entity_type = 'activity' AND entity_id = $1`, id); err != nil {
		return err
	}
	return redactDeliveries(ctx, tx, []ids.UUID{id}, erasedActivitySubject)
}
