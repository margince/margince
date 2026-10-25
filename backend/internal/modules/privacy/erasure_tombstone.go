// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The contact tombstone: what an Art. 17 erasure certifies when it closes, and
// nothing about whom.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/erasureaccount"
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

// Evidence keys of the account the tombstone carries beside its named counts.
const (
	evidenceKeyRows      = "rows"
	evidenceKeyUncounted = "uncounted"
	// legacyCertificate names the per-table rows a tombstone written before
	// they were counted cannot show.
	legacyCertificate = "per_table_rows"
)

// certifyErasure ends the cascade: it suppresses the subject's channel
// identities, then writes the tombstone that certifies every count. A request
// whose erasure is already certified keeps that tombstone and its account.
func certifyErasure(ctx context.Context, tx *tallyTx, subject ids.ContactID, reason string,
	identities []channelIdentity, uncounted []string, counts contactErasureCounts,
) (erasureaccount.Account, error) {
	suppressed, err := eraseChannelIdentities(ctx, tx, identities)
	if err != nil {
		return erasureaccount.Account{}, err
	}
	counts.channelIdentitiesSuppressed = suppressed
	if fulfillingCase(reason) != (ids.UUID{}) {
		prior, certified, err := certifiedErasure(ctx, tx, reason, subject.UUID)
		if err != nil || certified {
			return prior, err
		}
	}
	account := erasureaccount.Account{Rows: tx.counted(), Uncounted: uncounted}
	return account, tombstoneContactErasure(ctx, tx, subject, reason, counts, account)
}

// uncountedArms are the eraser's seams that destroy something outside the
// database. The rows that named it are counted; the objects are not.
func (e *Eraser) uncountedArms() []string {
	arms := []string{}
	if e.blob != nil {
		arms = append(arms, "attachment_object")
	}
	if e.payloads != nil {
		arms = append(arms, "controller_delivery_payload")
	}
	return arms
}

// tombstoneContactErasure closes the cascade with action=erase and counts only —
// proof without PII. The counts are evidence ABOUT the scrub, so they ride the
// evidence column; before/after stay empty — they are reserved for field
// images, and the record-history read serves a tombstone's images verbatim. The
// paired event tells consumers the subject is gone.
func tombstoneContactErasure(ctx context.Context, tx pgx.Tx, subject ids.ContactID, reason string,
	counts contactErasureCounts, account erasureaccount.Account,
) error {
	auditID, err := storekit.AuditWithEvidence(ctx, tx, actionErase, "contact", subject.UUID, nil, nil, map[string]any{
		"reason": reason, "emails_suppressed": counts.emailsSuppressed, "raw_rows_purged": counts.rawRowsPurged,
		"ai_payloads_purged": counts.aiPayloadsPurged, "activities_redacted": counts.activitiesRedacted,
		"activities_restricted":         counts.activitiesRestricted,
		"channel_identities_suppressed": counts.channelIdentitiesSuppressed,
		evidenceKeyRows:                 account.Rows, evidenceKeyUncounted: account.Uncounted,
	})
	if err != nil {
		return err
	}
	return storekit.EmitEventForEntity(ctx, tx, auditID, "contact", subject.UUID, retentionAppliedPayload(crmcontracts.RetentionAppliedErase, nil, &reason))
}

// CertifiedErasureTx answers the account the tombstone of reason certifies,
// whichever contact it names, for the backfill of a request's proof.
func CertifiedErasureTx(ctx context.Context, tx pgx.Tx, reason string) (erasureaccount.Account, bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return erasureaccount.Account{}, false, err
	}
	return certifiedErasure(ctx, tx, reason, ids.UUID{})
}

// certifiedErasure reads the first tombstone carrying reason, on contact
// unless contact is the zero id. A tombstone from before the per-table count
// answers its named counts and names the rows it cannot show as uncounted.
func certifiedErasure(ctx context.Context, tx pgx.Tx, reason string, contact ids.UUID) (erasureaccount.Account, bool, error) {
	var evidence []byte
	err := tx.QueryRow(ctx, `
		SELECT evidence FROM audit_log
		 WHERE action = 'erase' AND entity_type = 'contact' AND evidence->>'reason' = $1
		   AND ($2::uuid IS NULL OR entity_id = $2)
		 ORDER BY occurred_at, id LIMIT 1`, reason, storekit.UUIDOrNil(contact)).Scan(&evidence)
	if errors.Is(err, pgx.ErrNoRows) {
		return erasureaccount.Account{}, false, nil
	}
	if err != nil {
		return erasureaccount.Account{}, false, fmt.Errorf("privacy: reading an erasure's tombstone: %w", err)
	}
	var certificate struct {
		Rows      map[string]int64 `json:"rows"`
		Uncounted []string         `json:"uncounted"`
	}
	if err := json.Unmarshal(evidence, &certificate); err != nil {
		return erasureaccount.Account{}, false, fmt.Errorf("privacy: reading an erasure's tombstone: %w", err)
	}
	if certificate.Rows != nil {
		return erasureaccount.Account{Rows: certificate.Rows, Uncounted: append([]string{}, certificate.Uncounted...)}, true, nil
	}
	return legacyAccount(evidence)
}

// legacyAccount reads the named whole-number counts of a tombstone written
// before the per-table rows were counted.
func legacyAccount(evidence []byte) (erasureaccount.Account, bool, error) {
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(evidence, &entries); err != nil {
		return erasureaccount.Account{}, false, err
	}
	rows := map[string]int64{}
	for category, raw := range entries {
		var count int64
		if json.Unmarshal(raw, &count) == nil {
			rows[category] = count
		}
	}
	return erasureaccount.Account{Rows: rows, Uncounted: []string{legacyCertificate}}, true, nil
}
