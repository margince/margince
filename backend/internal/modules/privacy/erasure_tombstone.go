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
	leadsAnonymized             int
}

// evidence is the tombstone's certificate. Its count keys are also the
// categories a privacy request's proof records, so the two cannot name one
// count two ways.
func (c contactErasureCounts) evidence() map[string]int64 {
	return map[string]int64{
		"contacts_anonymized": 1, "leads_anonymized": int64(c.leadsAnonymized),
		"emails_suppressed": int64(c.emailsSuppressed), "raw_rows_purged": c.rawRowsPurged,
		"ai_payloads_purged": c.aiPayloadsPurged, "activities_redacted": int64(c.activitiesRedacted),
		"activities_restricted":         int64(c.activitiesRestricted),
		"channel_identities_suppressed": int64(c.channelIdentitiesSuppressed),
	}
}

// certifyErasure ends the cascade: it suppresses the subject's channel
// identities, then writes the tombstone that certifies every count.
func certifyErasure(ctx context.Context, tx pgx.Tx, subject ids.ContactID, reason string,
	identities []channelIdentity, counts contactErasureCounts,
) (map[string]int64, error) {
	suppressed, err := eraseChannelIdentities(ctx, tx, identities)
	if err != nil {
		return nil, err
	}
	counts.channelIdentitiesSuppressed = suppressed
	return tombstoneContactErasure(ctx, tx, subject, reason, counts)
}

// tombstoneContactErasure closes the cascade with action=erase and counts only —
// proof without PII. The counts are evidence ABOUT the scrub, so they ride the
// evidence column; before/after stay empty — they are reserved for field
// images, and the record-history read serves a tombstone's images verbatim. The
// paired event tells consumers the subject is gone.
//
// It answers the counts it certified, by category.
func tombstoneContactErasure(ctx context.Context, tx pgx.Tx, subject ids.ContactID, reason string, counts contactErasureCounts) (map[string]int64, error) {
	certified := counts.evidence()
	certificate := map[string]any{"reason": reason}
	for category, count := range certified {
		certificate[category] = count
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, actionErase, "contact", subject.UUID, nil, nil, certificate)
	if err != nil {
		return nil, err
	}
	return certified, storekit.EmitEventForEntity(ctx, tx, auditID, "contact", subject.UUID, retentionAppliedPayload(crmcontracts.RetentionAppliedErase, nil, &reason))
}
