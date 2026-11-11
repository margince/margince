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

const (
	subjectRedactionReason = "subject_data_removed"
	subjectRedactionCause  = "subject_redaction"
)

// tombstoneCollateralScrubs certifies each erased record and physically clears
// its retained audit and event payloads in the same transaction.
func tombstoneCollateralScrubs(ctx context.Context, tx pgx.Tx, entityType string, records []ids.UUID, reason, cause string) error {
	for _, id := range records {
		if _, err := certifyCollateralScrub(ctx, tx, entityType, id, reason, cause); err != nil {
			return err
		}
	}
	return nil
}

func certifyCollateralScrub(ctx context.Context, tx pgx.Tx, entityType string, id ids.UUID, reason, cause string) (ids.UUID, error) {
	proof, err := storekit.AuditWithEvidence(ctx, tx, actionErase, entityType, id, nil, nil, map[string]any{evidenceKeyReason: reason, evidenceKeyCause: cause})
	if err != nil {
		return ids.UUID{}, fmt.Errorf("tombstoning scrubbed %s: %w", entityType, err)
	}
	return proof, storekit.RedactAuditHistory(ctx, tx, proof)
}

func (e *Eraser) redactSubjectActivities(ctx context.Context, tx pgx.Tx, subject ids.ContactID, keys subjectKeys, floor FloorPolicy, reason string) ([]ids.UUID, []ids.UUID, error) {
	// Lock before checking the floor so another writer cannot change the erase-or-hold decision.
	if err := lockSubjectTimeline(ctx, tx, subject, keys.emails, channelActivityKeys(keys.identities)); err != nil {
		return nil, nil, err
	}
	pending, err := e.pendingTimelineUpdates(ctx, tx, subject, keys)
	if err != nil {
		return nil, nil, err
	}
	activitiesRedacted, err := e.redactSubjectTimeline(ctx, tx, subject, keys.emails, channelActivityKeys(keys.identities), floor, reason)
	if err != nil {
		return nil, nil, err
	}
	notesRedacted, err := redactUnlinkedNotes(ctx, tx, keys)
	if err != nil {
		return nil, nil, err
	}
	// The same floor assigns each timeline row to erasure or restricted retention.
	activitiesHeld, err := holdShieldedTimeline(ctx, tx, subject, keys.emails, channelActivityKeys(keys.identities), floor, e.payloads)
	if err != nil {
		return nil, nil, err
	}
	if err := e.purgeRedactedActivityTraces(ctx, tx, activitiesRedacted, activitiesHeld, reason); err != nil {
		return nil, nil, err
	}
	if err := purgeActivityTraces(ctx, tx, notesRedacted, e.payloads); err != nil {
		return nil, nil, err
	}
	if err := e.purgeTextCopies(ctx, tx, notesRedacted); err != nil {
		return nil, nil, err
	}
	activitiesRedacted = append(activitiesRedacted, notesRedacted...)
	if len(pending) > 0 {
		if err := e.invalidateEdges(ctx, tx, pending); err != nil {
			return nil, nil, err
		}
	}

	return activitiesRedacted, activitiesHeld, nil
}
