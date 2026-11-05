// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// purgeRedactedActivityTraces finishes off the activities the timeline redaction
// just emptied. It removes their vectors, their own audit spines, the external
// identities they answered to and the proposals read out of them. It also
// clears the transmitted copy in the send log and the copies other modules
// took of their text. Those copies go for the held activities too: the floor
// keeps the original record, never a voice sample read from it.
func (e *Eraser) purgeRedactedActivityTraces(ctx context.Context, tx pgx.Tx, activities, held []ids.UUID, reason string) error {
	if err := purgeActivityTraces(ctx, tx, activities, e.payloads); err != nil {
		return err
	}
	if err := e.purgeTextCopies(ctx, tx, append(slices.Clone(activities), held...)); err != nil {
		return err
	}
	return tombstoneCollateralScrubs(ctx, tx, "activity", activities, reason, causeContactErasure)
}

func purgeActivityTraces(ctx context.Context, tx pgx.Tx, activities []ids.UUID, payloads PayloadPurger) error {
	if err := purgeActivityIndexes(ctx, tx, activities); err != nil {
		return err
	}
	// The external identities of the messages just emptied. The activity-content
	// arm has always retired these and this arm never did, so a subject's
	// Message-IDs outlived the erasure that emptied their correspondence.
	if err := retireActivityIdentities(ctx, tx, activities); err != nil {
		return err
	}
	// The readings of those rows, which describe a body that is now gone. The
	// proposals a reading produced are scrubbed later in the cascade, after the
	// subject's own stagings (EraseContact states why the order matters).
	if err := purgeTranscriptReadings(ctx, tx, activities); err != nil {
		return err
	}
	// The transmitted copy of every activity just redacted. Without this the
	// timeline row is a tombstone while the send log still holds the message.
	// That copy carries the address, the subject line and the body.
	return redactDeliveries(ctx, tx, activities, erasedName, payloads)
}

// tombstoneCollateralScrubs stamps an erase tombstone on each record the erasure
// scrubbed alongside the subject. Field history cuts a record's timeline at its
// own newest erase row, so the contact's tombstone cannot bound a twin's spine.
// Without these, old audit images (a lead's email, an activity's subject line)
// would serve the erased PII again. The context rides evidence and the images
// stay empty: the record-history read serves them verbatim.
// No outbox event is paired: the contact's single retention.applied is the fact.
// cause is the caller's, because retention reaches this path too, and
// contact_erasure on a retention sweep's record would misstate why the data went.
func tombstoneCollateralScrubs(ctx context.Context, tx pgx.Tx, entityType string, records []ids.UUID, reason, cause string) error {
	for _, id := range records {
		if _, err := storekit.AuditWithEvidence(ctx, tx, actionErase, entityType, id, nil, nil, map[string]any{
			evidenceKeyReason: reason, evidenceKeyCause: cause,
		}); err != nil {
			return fmt.Errorf("tombstoning scrubbed %s: %w", entityType, err)
		}
	}
	return nil
}
