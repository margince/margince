// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// Deleting a website lead who could not be told we hold them. What is left is
// the row in lead_privacy_notice that says a notice failed and why.
//
// A DELETE, unlike retention's anonymize. A nameless row would still record
// that this human was found, and the notice was the condition for keeping that.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/extprivacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	extdecl "github.com/margince/margince/backend/pkg/extension/privacy"
)

// causeUntoldLead is the tombstone cause for a website lead deleted because its
// notice could not be given.
const causeUntoldLead = "website_lead_not_told"

// untoldLeadReason is the reason the tombstones and the event carry.
const untoldLeadReason = "the website lead could not be sent its privacy notice"

// UntoldLead names a website lead whose notice failed, and what came with it.
type UntoldLead struct {
	// LeadID is zero when no lead was kept: the page published no address, or
	// the capture rolled back with the notice.
	LeadID ids.LeadID
	// ApprovalID is the accepted proposal, which carries the name, address and
	// page snippet. It is emptied and keeps its verdict.
	ApprovalID ids.ApprovalID
	// DeliveryID is the notice mail, zero when none was staged.
	DeliveryID ids.UUID
	// Extensions declares the extension tables cleared with the lead. Every
	// caller outside a test sets it.
	Extensions extprivacy.Registry
}

// EraseUntoldLeadTx deletes the lead and the records that carry its details,
// on the caller's transaction, so the deletion commits with the failure that
// caused it.
func EraseUntoldLeadTx(ctx context.Context, tx pgx.Tx, in UntoldLead) error {
	if err := refuseAnythingButAnUntoldLead(ctx, tx, in); err != nil {
		return err
	}
	if !in.ApprovalID.IsZero() {
		if err := eraseApprovalNoticesMatching(ctx, tx, untoldLeadReason, causeUntoldLead, "id = @id", pgx.StrictNamedArgs{"id": in.ApprovalID}); err != nil {
			return err
		}
		if err := dropFromSiteRead(ctx, tx, in.ApprovalID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE approval
			   SET `+blankStagedProposal+`,
			       consumed_at = coalesce(consumed_at, now())
			 WHERE id = $1`, in.ApprovalID); err != nil {
			return fmt.Errorf("emptying the proposal the untold lead came from: %w", err)
		}
	}
	if in.LeadID.IsZero() {
		return nil
	}
	notices, err := deleteNoticeActivities(ctx, tx, in.DeliveryID, in.LeadID.UUID)
	if err != nil {
		return err
	}
	if err := redactQuestionSource(ctx, tx, "lead", []ids.UUID{in.LeadID.UUID}); err != nil {
		return err
	}
	// Before the lead row goes. Extension rows link to it by id, and would keep
	// their personal columns against a record that no longer exists.
	extensionRows, err := clearExtensionRows(ctx, tx, in.Extensions, untoldLeadReason, map[extdecl.RecordKind][]ids.UUID{
		extdecl.Lead: {in.LeadID.UUID}, extdecl.Activity: notices,
	})
	if err != nil {
		return err
	}
	if err := deleteLeadRecordsTx(ctx, tx, in.LeadID.UUID); err != nil {
		return err
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, actionErase, "lead", in.LeadID.UUID, nil, nil,
		map[string]any{
			evidenceKeyReason: untoldLeadReason, evidenceKeyCause: causeUntoldLead, "extension_rows_cleared": extensionRows,
		})
	if err != nil {
		return fmt.Errorf("tombstoning the untold lead: %w", err)
	}
	reason := untoldLeadReason
	if err := storekit.EmitEventForEntity(ctx, tx, auditID, "lead", in.LeadID.UUID,
		retentionAppliedPayload(crmcontracts.RetentionAppliedErase, nil, &reason)); err != nil {
		return err
	}
	return tombstoneCollateralScrubs(ctx, tx, "activity", notices, untoldLeadReason, causeUntoldLead)
}

// deleteNoticeActivities deletes the timeline row of the notice mail, which
// takes the delivery and its decisions by cascade: the delivery holds the
// recipient address. A row under a statutory hold stays where it is.
func deleteNoticeActivities(ctx context.Context, tx pgx.Tx, deliveryID, leadID ids.UUID) ([]ids.UUID, error) {
	if deliveryID.IsZero() {
		return nil, nil
	}
	// Only a delivery filed under this lead: a delivery id naming somebody
	// else's mail deletes nothing.
	rows, err := tx.Query(ctx, `
		DELETE FROM activity
		 WHERE id IN (SELECT o.activity_id FROM comms_outbound o
		                JOIN activity_link al ON al.activity_id = o.activity_id AND al.lead_id = $2
		               WHERE o.id = $1)
		   AND restricted_at IS NULL
		RETURNING id`, deliveryID, leadID)
	if err != nil {
		return nil, fmt.Errorf("deleting the untold lead's notice mail: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

// deleteLeadRecordsTx deletes the lead row and the records no foreign key
// reaches from it. Those are the captured original, the field stamps, the
// vectors and the communication record. The rest cascade from the row.
func deleteLeadRecordsTx(ctx context.Context, tx pgx.Tx, leadID ids.UUID) error {
	if _, err := tx.Exec(ctx, `
		DELETE FROM raw_capture rc USING lead l
		 WHERE l.id = $1 AND rc.source_system = l.source_system AND rc.source_id = l.source_id`,
		leadID); err != nil {
		return fmt.Errorf("deleting the untold lead's captured original: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM field_provenance WHERE object_type = 'lead' AND object_id = $1`, leadID); err != nil {
		return fmt.Errorf("deleting the untold lead's field stamps: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM embedding WHERE entity_type = 'lead' AND entity_id = $1`, leadID); err != nil {
		return fmt.Errorf("deleting the untold lead's vectors: %w", err)
	}
	if err := clearLeadCommunicationRecord(ctx, tx, leadID, nil); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM lead WHERE id = $1`, leadID); err != nil {
		return fmt.Errorf("deleting the untold lead: %w", err)
	}
	return nil
}

// dropFromSiteRead removes the untold contact from the site read that found
// them. The dossier keeps every contact a page published, with name, role,
// address and the snippet, and the accepted proposal names which one.
//
// Read from the proposal before it is emptied. A contact is matched on name
// and published address together, as the read keyed them.
func dropFromSiteRead(ctx context.Context, tx pgx.Tx, approvalID ids.ApprovalID) error {
	if _, err := tx.Exec(ctx, `
		UPDATE site_read sr
		   SET contacts = coalesce((
		         SELECT jsonb_agg(c.entry ORDER BY c.position)
		           FROM jsonb_array_elements(sr.contacts) WITH ORDINALITY AS c(entry, position)
		          WHERE NOT (c.entry->>'name' = a.proposed_change->>'name'
		                 AND coalesce(c.entry->>'published_email', '') =
		                     coalesce(a.proposed_change->>'published_email', ''))), '[]'::jsonb),
		       updated_at = now()
		  FROM approval a
		 WHERE a.id = $1 AND a.kind = 'site_lead'
		   AND sr.id::text = a.proposed_change->>'site_read_id'`, approvalID); err != nil {
		return fmt.Errorf("removing the untold contact from the site read: %w", err)
	}
	return nil
}

// errNotAnUntoldLead refuses an erasure pointed at anything but a website
// lead and the site_lead proposal it came from.
var errNotAnUntoldLead = errors.New("privacy: only a website lead and its site_lead proposal are erased for an untold notice")

// refuseAnythingButAnUntoldLead checks, before anything is written, what the
// accept itself recorded. A lead counts only when its notice row names it with
// this proposal, and the proposal must be a site_lead one.
func refuseAnythingButAnUntoldLead(ctx context.Context, tx pgx.Tx, in UntoldLead) error {
	var lead, approval *ids.UUID
	if !in.LeadID.IsZero() {
		lead = &in.LeadID.UUID
	}
	if !in.ApprovalID.IsZero() {
		approval = &in.ApprovalID.UUID
	}
	var mismatched bool
	if err := tx.QueryRow(ctx, `
		SELECT ($1::uuid IS NOT NULL AND NOT EXISTS (
		           SELECT 1 FROM lead_privacy_notice
		            WHERE lead_id = $1 AND approval_id IS NOT DISTINCT FROM $2))
		    OR EXISTS (SELECT 1 FROM approval WHERE id = $2 AND kind <> 'site_lead')`,
		lead, approval).Scan(&mismatched); err != nil {
		return fmt.Errorf("checking what an untold-lead erasure names: %w", err)
	}
	if mismatched {
		return errNotAnUntoldLead
	}
	return nil
}
