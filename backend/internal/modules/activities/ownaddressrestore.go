// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ownAddressUnlink is one message RefileOwnAddressMailTx took a contact off,
// as its audit row remembers it.
type ownAddressUnlink struct {
	audit        ids.UUID
	activity     ids.ActivityID
	link         json.RawMessage
	participants []json.RawMessage
}

// RestoreOwnAddressMailTx puts back what a withdrawn claim's refile removed:
// the contact's link on each message, and its participant rows without the
// stated address. It answers how many messages it restored. A message archived
// since is left. A restored message is marked, so a rerun restores nothing.
func RestoreOwnAddressMailTx(ctx context.Context, tx pgx.Tx, claim ids.UUID, limit int) (int, error) {
	unlinks, err := ownAddressUnlinksTx(ctx, tx, claim, limit)
	if err != nil || len(unlinks) == 0 {
		return 0, err
	}
	activityIDs := make([]ids.UUID, len(unlinks))
	for i, u := range unlinks {
		activityIDs[i] = u.activity.UUID
	}
	if err := storekit.LockMeetingAttendeePage(ctx, tx, activityIDs); err != nil {
		return 0, err
	}
	restored := 0
	for _, u := range unlinks {
		ok, err := restoreOwnAddressUnlinkTx(ctx, tx, u)
		if err != nil {
			return 0, err
		}
		if ok {
			restored++
		}
	}
	return restored, nil
}

func ownAddressUnlinksTx(ctx context.Context, tx pgx.Tx, claim ids.UUID, limit int) ([]ownAddressUnlink, error) {
	rows, err := tx.Query(ctx, `
		SELECT a.id, a.entity_id, a.evidence -> $4::text, a.evidence -> $5::text
		  FROM audit_log a
		 WHERE a.entity_type = 'activity' AND a.action = 'activity_relink'
		   AND a.evidence ->> $1::text = $2 AND a.evidence ->> $3::text = $6
		   AND NOT EXISTS (SELECT 1 FROM audit_log u
		                    WHERE u.entity_type = 'activity' AND u.evidence ->> $7::text = a.id::text)
		 ORDER BY a.id LIMIT $8`,
		evidenceReason, ownAddressClaimedReason, evidenceClaim, evidenceLink, evidenceParticipants,
		claim.String(), evidenceUndoOf, limit)
	if err != nil {
		return nil, fmt.Errorf("activities: reading what a withdrawn claim removed: %w", err)
	}
	defer rows.Close()
	var out []ownAddressUnlink
	for rows.Next() {
		var u ownAddressUnlink
		if err := rows.Scan(&u.audit, &u.activity, &u.link, &u.participants); err != nil {
			return nil, fmt.Errorf("activities: reading what a withdrawn claim removed: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func restoreOwnAddressUnlinkTx(ctx context.Context, tx pgx.Tx, u ownAddressUnlink) (bool, error) {
	touched, err := tx.Exec(ctx, `UPDATE activity SET updated_at = now() WHERE id = $1 AND archived_at IS NULL AND restricted_at IS NULL`, u.activity)
	if err != nil {
		return false, fmt.Errorf("activities: touching a message to restore its contact: %w", err)
	}
	if touched.RowsAffected() == 0 {
		return false, nil
	}
	var link struct {
		Contact ids.UUID `json:"contact_id"`
	}
	if string(u.link) != "null" {
		if err := json.Unmarshal(u.link, &link); err != nil {
			return false, fmt.Errorf("activities: reading a removed link: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_link SELECT * FROM jsonb_populate_record(NULL::activity_link, $1::jsonb)
			ON CONFLICT DO NOTHING`, u.link); err != nil {
			return false, fmt.Errorf("activities: putting a claimed address's link back: %w", err)
		}
	}
	for _, p := range u.participants {
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_participant SELECT * FROM jsonb_populate_record(NULL::activity_participant,
			       CASE WHEN jsonb_typeof($1::jsonb->'address') = 'string'
			            THEN jsonb_set($1::jsonb, '{address}', to_jsonb(lower(btrim($1::jsonb->>'address')))) ELSE $1::jsonb END)
			ON CONFLICT DO NOTHING`, p); err != nil {
			return false, fmt.Errorf("activities: putting a claimed address's participant back: %w", err)
		}
	}
	evidence := map[string]any{evidenceReason: ownAddressReleasedReason, evidenceUndoOf: u.audit}
	after := map[string]any{fieldEntityType: linkEntityContact, fieldEntityID: link.Contact}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "activity_relink", "activity", u.activity.UUID, nil, after, evidence)
	if err != nil {
		return false, err
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, u.activity.UUID, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: relinkedChangedFields(linkEntityContact, link.Contact),
	}); err != nil {
		return false, err
	}
	return true, RecomputeAudienceTx(ctx, tx, u.activity)
}
