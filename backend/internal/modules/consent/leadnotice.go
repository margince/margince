// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The Art. 14 notice a website lead is owed, sent before anything else. It is
// the privacy-notice mail, wording and link a contact gets, sent to a lead.
//
// lead_privacy_notice is the proof row: wording, language and delivery. It
// names nobody, so it is all that is left when a failed notice deletes the lead.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// Why a website lead's notice was not given. The table's CHECK holds the same
// three words.
const (
	// LeadNoticeNoAddress: the page published no address to send it to.
	LeadNoticeNoAddress = "no_address"
	// LeadNoticeCollision: another source's lead already holds the address,
	// so the website's copy of the contact is not kept.
	LeadNoticeCollision = "collision"
	// LeadNoticeRefused: the notice could not be staged.
	LeadNoticeRefused = "refused"
	// LeadNoticeBounced: the mail was sent and the mailbox refused it.
	LeadNoticeBounced = "bounced"
)

// ErrNoMailLane refuses any controller mail while the installation cannot send
// its own. Each of these messages carries a one-time link that is mailed and
// never returned. One that cannot be sent is one nobody can use.
var ErrNoMailLane = fmt.Errorf("%w: this installation cannot send its own mail yet; "+
	"configure the relay, public URL and key vault",
	apperrors.ErrConflict)

// ErrNoNoticeLane is that refusal with the lead's own consequence named: the
// lead would be held with nobody told.
var ErrNoNoticeLane = fmt.Errorf("%w, so a website lead cannot be told it is held",
	ErrNoMailLane)

// errNotAWebsiteLead refuses a notice to any lead but the one the accepted
// website-lead proposal captured.
var errNotAWebsiteLead = fmt.Errorf("%w: only a lead read off a company's website is sent this notice",
	apperrors.ErrConflict)

// errNotARedeemedSiteLead refuses a failure record for anything but an
// accepted website-lead proposal redeemed in the caller's transaction.
var errNotARedeemedSiteLead = fmt.Errorf("%w: a failed website-lead notice is recorded only "+
	"against the accepted site_lead proposal it came from", apperrors.ErrConflict)

// errLeadHasNoAddress refuses a notice to a lead with no address on the row.
var errLeadHasNoAddress = fmt.Errorf("%w: the lead has no address to send the notice to",
	apperrors.ErrConflict)

// CanTellLeads reports whether this store can send a website lead its notice.
func (s *Store) CanTellLeads() bool {
	return s.canSendConfirm()
}

// LeadNotice names the lead to tell and the accepted proposal it came from.
type LeadNotice struct {
	LeadID     ids.LeadID
	ApprovalID ids.ApprovalID
}

// IssueLeadPrivacyNotice mints the lead's notice link, stages the mail and
// writes the proof row, all in one transaction. It joins a transaction the
// caller holds, so the lead's capture and its notice commit together.
//
// A lead that already has a notice row is told nothing more, and the answer is
// false. A second accept of the same lead is not a second disclosure.
func (s *Store) IssueLeadPrivacyNotice(ctx context.Context, in LeadNotice) (bool, error) {
	if !s.canSendConfirm() {
		return false, ErrNoNoticeLane
	}
	token, err := newConfirmToken()
	if err != nil {
		return false, err
	}
	var told bool
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		address, already, err := leadNoticeAddressTx(ctx, tx, in)
		if err != nil || already {
			return err
		}
		language := s.mailLanguage(ctx, tx)
		tokenRowID, expires, err := s.mintLeadNoticeTokenTx(ctx, tx, in.LeadID, address, hashPublicToken(token), language)
		if err != nil {
			return err
		}
		deliveryID, err := s.stageConfirmMail(ctx, tx, confirmMailInput{
			leadID: in.LeadID, recipient: address, kind: LinkPrivacyNotice, language: language,
			tokenRowID: tokenRowID, link: s.confirmLink(token), expiresAt: expires,
		})
		if err != nil {
			return err
		}
		if deliveryID.IsZero() {
			return ErrNoNoticeLane
		}
		told = true
		return writeLeadNoticeTx(ctx, tx, leadNoticeRow{
			leadID: &in.LeadID, approvalID: in.ApprovalID, deliveryID: &deliveryID,
			locale: language, state: "queued",
		})
	})
	return told, err
}

// leadNoticeAddressTx locks the live lead the notice is for and reads the
// address it goes to, and whether the lead was already sent one.
//
// The lead must be the one this redeemed site_lead proposal captured: its
// source key is the proposal's natural key. A request can write neither.
func leadNoticeAddressTx(ctx context.Context, tx pgx.Tx, in LeadNotice) (string, bool, error) {
	leadID := in.LeadID
	if err := auth.Require(ctx, entityLead, principal.ActionRead); err != nil {
		return "", false, err
	}
	if err := auth.EnsureVisibleLive(ctx, tx, entityLead, leadID.UUID); err != nil {
		return "", false, err
	}
	if err := auth.LockSubjectLive(ctx, tx, entityLead, leadID.UUID); err != nil {
		return "", false, err
	}
	var address *string
	var already, fromAWebsite bool
	if err := tx.QueryRow(ctx, `
		SELECT l.email, EXISTS (SELECT 1 FROM lead_privacy_notice n WHERE n.lead_id = l.id),
		       l.source_system IS NOT DISTINCT FROM $2 AND EXISTS (
		           SELECT 1 FROM approval a
		            WHERE a.id = $3 AND a.kind = 'site_lead' AND a.consumed_at IS NOT NULL
		              AND a.proposed_change->>'natural_key' = l.source_id)
		  FROM lead l WHERE l.id = $1`, leadID, provenance.WebsiteLeadSource, in.ApprovalID).
		Scan(&address, &already, &fromAWebsite); err != nil {
		return "", false, fmt.Errorf("consent: reading where the lead's notice goes: %w", err)
	}
	if !fromAWebsite {
		return "", false, errNotAWebsiteLead
	}
	if already {
		return "", true, nil
	}
	if address == nil || strings.TrimSpace(*address) == "" {
		return "", false, errLeadHasNoAddress
	}
	return *address, false, nil
}

// mintLeadNoticeTokenTx writes the lead's notice link. The audit row leaves
// the address out: if the notice fails the lead is deleted, and an address in
// the append-only audit would outlive it.
func (s *Store) mintLeadNoticeTokenTx(
	ctx context.Context, tx pgx.Tx, leadID ids.LeadID, address, tokenHash, language string,
) (ids.UUID, time.Time, error) {
	issued := s.now().UTC()
	expires := issued.Add(confirmTokenTTL)
	var tokenRowID ids.UUID
	args := []any{leadID, tokenHash, address, issued, expires, LinkPrivacyNotice, language}
	if err := tx.QueryRow(ctx, `
		INSERT INTO confirm_token (lead_id, token_hash, delivered_to, issued_at, expires_at, kind, question_locale)
		VALUES (`+storekit.Placeholders(args)+`)
		RETURNING id`, args...).Scan(&tokenRowID); err != nil {
		return ids.UUID{}, time.Time{}, fmt.Errorf("consent: minting the lead's notice link: %w", err)
	}
	if _, err := storekit.AuditEvent(ctx, tx, "create", "confirm_token", tokenRowID, map[string]any{
		subjectColumnLead: leadID, "expires_at": expires, auditKeyKind: LinkPrivacyNotice,
	}); err != nil {
		return ids.UUID{}, time.Time{}, err
	}
	return tokenRowID, expires, nil
}

// leadNoticeRow is one row of lead_privacy_notice.
type leadNoticeRow struct {
	leadID     *ids.LeadID
	approvalID ids.ApprovalID
	deliveryID *ids.UUID
	locale     string
	state      string
	failure    string
}

// writeLeadNoticeTx inserts the proof row for the privacy_notice wording this
// build sends, and audits it. Neither carries a name or an address.
func writeLeadNoticeTx(ctx context.Context, tx pgx.Tx, row leadNoticeRow) error {
	version := controllerTemplates[TemplatePrivacyNotice].version
	var id ids.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO lead_privacy_notice
		    (lead_id, approval_id, delivery_id, template_key, template_version, locale, state, failure,
		     queued_at, failed_at)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, NULLIF($8, ''),
		        CASE WHEN $7 = 'queued' THEN now() END, CASE WHEN $7 = 'failed' THEN now() END)
		RETURNING id`,
		row.leadID, nullableApproval(row.approvalID), row.deliveryID, TemplatePrivacyNotice, version,
		row.locale, row.state, row.failure).Scan(&id); err != nil {
		return fmt.Errorf("consent: recording the website lead's notice: %w", err)
	}
	auditID, err := storekit.AuditEvent(ctx, tx, "create", tableLeadPrivacyNotice, id, map[string]any{
		fieldState: row.state, "failure": row.failure, "template_version": version, "locale": row.locale,
	})
	if err != nil {
		return err
	}
	recorded := crmcontracts.InternalEventLeadPrivacyNoticeCreated{
		State: crmcontracts.InternalEventLeadPrivacyNoticeCreatedState(row.state),
	}
	if row.failure != "" {
		failure := crmcontracts.InternalEventLeadPrivacyNoticeCreatedFailure(row.failure)
		recorded.Failure = &failure
	}
	return storekit.EmitPipelinePayload(ctx, tx, auditID, recorded)
}

// tableLeadPrivacyNotice names the proof table on its audit rows.
const tableLeadPrivacyNotice = "lead_privacy_notice"

// nullableApproval keeps approval_id NULL where no proposal is named.
func nullableApproval(id ids.ApprovalID) *ids.ApprovalID {
	if id.IsZero() {
		return nil
	}
	return &id
}

// RecordLeadNoticeFailedTx records a notice that failed before any lead was
// kept. The page published no address, another lead holds it, or staging was
// refused and the capture rolled back.
func RecordLeadNoticeFailedTx(ctx context.Context, tx pgx.Tx, approvalID ids.ApprovalID, failure string) error {
	if failure != LeadNoticeNoAddress && failure != LeadNoticeCollision && failure != LeadNoticeRefused {
		return fmt.Errorf("consent: %q is not a reason a notice fails before the lead is kept", failure)
	}
	var redeemedSiteLead bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM approval
		                WHERE id = $1 AND kind = 'site_lead' AND consumed_at IS NOT NULL)`,
		approvalID).Scan(&redeemedSiteLead); err != nil {
		return fmt.Errorf("consent: reading the proposal a failed notice came from: %w", err)
	}
	if !redeemedSiteLead {
		return errNotARedeemedSiteLead
	}
	return writeLeadNoticeTx(ctx, tx, leadNoticeRow{approvalID: approvalID, state: "failed", failure: failure})
}

// BouncedLeadNotice names what a bounced website lead's notice leaves to be
// deleted: the lead and the proposal it came from.
type BouncedLeadNotice struct {
	LeadID     ids.LeadID
	ApprovalID ids.ApprovalID
	DeliveryID ids.UUID
}

// LeadNoticeBouncedTx marks the notice this delivery carried as bounced and
// answers which lead it was for. False means the delivery carried no website
// lead's notice, which is the ordinary answer for a bounce.
//
// Runs on the bounce's own transaction, so the mark and the deletion of the
// lead that follows commit with the report that caused them.
func LeadNoticeBouncedTx(ctx context.Context, tx pgx.Tx, deliveryID ids.UUID) (BouncedLeadNotice, bool, error) {
	if deliveryID.IsZero() {
		return BouncedLeadNotice{}, false, nil
	}
	var leadID *ids.LeadID
	var approvalID *ids.ApprovalID
	var id ids.UUID
	err := tx.QueryRow(ctx, `
		UPDATE lead_privacy_notice
		   SET state = 'failed', failure = $2, failed_at = now()
		 WHERE delivery_id = $1 AND state = 'queued'
		RETURNING id, lead_id, approval_id`, deliveryID, LeadNoticeBounced).Scan(&id, &leadID, &approvalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return BouncedLeadNotice{}, false, nil
	}
	if err != nil {
		return BouncedLeadNotice{}, false, fmt.Errorf("consent: marking the website lead's notice bounced: %w", err)
	}
	auditID, err := storekit.Audit(ctx, tx, "update", tableLeadPrivacyNotice, id,
		map[string]any{fieldState: "queued"},
		map[string]any{fieldState: "failed", "failure": LeadNoticeBounced})
	if err != nil {
		return BouncedLeadNotice{}, false, err
	}
	if err := storekit.EmitPipelinePayload(ctx, tx, auditID,
		crmcontracts.InternalEventLeadPrivacyNoticeFailed{}); err != nil {
		return BouncedLeadNotice{}, false, err
	}
	out := BouncedLeadNotice{DeliveryID: deliveryID}
	if leadID != nil {
		out.LeadID = *leadID
	}
	if approvalID != nil {
		out.ApprovalID = *approvalID
	}
	return out, true, nil
}
