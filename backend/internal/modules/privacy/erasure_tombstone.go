// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The contact tombstone: what an Art. 17 erasure certifies when it closes, and
// nothing about whom.

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// contactErasureCounts is what an erasure tombstone certifies: how much each arm
// of the cascade removed, and nothing whatsoever about whom.
type contactErasureCounts struct {
	emailsSuppressed            int
	rawRowsPurged               int64
	aiPayloadsPurged            int64
	activitiesRedacted          int
	activitiesRestricted        int
	channelIdentitiesSuppressed int
}

// tombstoneContactErasure closes the cascade with action=erase and counts only —
// proof without PII. The counts are evidence ABOUT the scrub, so they ride the
// evidence column; before/after stay empty — they are reserved for field
// images, and the record-history read serves a tombstone's images verbatim. The
// paired event tells consumers the subject is gone.
func tombstoneContactErasure(ctx context.Context, tx pgx.Tx, subject ids.ContactID, reason string, counts contactErasureCounts) error {
	auditID, err := storekit.AuditWithEvidence(ctx, tx, actionErase, "contact", subject.UUID, nil, nil, map[string]any{
		"reason": reason, "emails_suppressed": counts.emailsSuppressed, "raw_rows_purged": counts.rawRowsPurged,
		"ai_payloads_purged": counts.aiPayloadsPurged, "activities_redacted": counts.activitiesRedacted,
		"activities_restricted":         counts.activitiesRestricted,
		"channel_identities_suppressed": counts.channelIdentitiesSuppressed,
	})
	if err != nil {
		return err
	}
	return storekit.EmitEventForEntity(ctx, tx, auditID, "contact", subject.UUID, retentionAppliedPayload(crmcontracts.RetentionAppliedErase, nil, &reason))
}
