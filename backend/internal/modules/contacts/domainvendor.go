// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A site that reads as a mail or hosting vendor. The site cannot tell the
// vendor's own staff from its customers, so the verdict waits. The colleague
// whose mail brought the domain in creates the company or keeps the verdict.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// WithholdVendorDomain opens the vendor question instead of settling the
// domain as a provider. It reports false for a domain a colleague already
// admitted: that yes outranks the verdict, so the caller reads the site on.
func (s *Store) WithholdVendorDomain(ctx context.Context, domain, evidence string, readID ids.UUID) (bool, error) {
	if err := auth.Require(ctx, entityCompany, principal.ActionUpdate); err != nil {
		return false, err
	}
	held := false
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		prior, known, err := readDispositionTx(ctx, tx, domain)
		if err != nil {
			return err
		}
		if known && prior.Admission == DomainAdmitted {
			return nil
		}
		held = true
		return withholdDomainTx(ctx, tx, domain, PendingVendorVerdict, evidence, readID)
	})
	return held, err
}

// KeepDomainAsVendor answers an open vendor question with the vendor verdict.
// Only a reader of the question may answer it, so a seat cannot settle a
// colleague's question by guessing the domain.
func (s *Store) KeepDomainAsVendor(ctx context.Context, domain string) error {
	if err := auth.Require(ctx, entityCompany, principal.ActionUpdate); err != nil {
		return err
	}
	base, ok := freemail.Hostname(domain)
	if !ok {
		return fmt.Errorf("contacts: %q is not a domain: %w", domain, apperrors.ErrInvalidArgument)
	}
	reader := actingHuman(ctx)
	if reader == nil {
		return apperrors.ErrPermissionDenied
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		prior, known, err := readDispositionTx(ctx, tx, base)
		if err != nil {
			return err
		}
		decides, err := s.holdsOpenQuestionTx(ctx, tx, base, *reader)
		if err != nil {
			return err
		}
		if !known || !decides {
			return fmt.Errorf("contacts: no question about %s for this reader: %w", base, apperrors.ErrNotFound)
		}
		var args []any
		arg := func(value any) string { args = append(args, value); return fmt.Sprintf("$%d", len(args)) }
		status, source, row, reason := arg(DomainProvider), arg(DomainSourceHuman), arg(base), arg(PendingVendorVerdict)
		var id ids.UUID
		err = tx.QueryRow(ctx, `UPDATE company_domain_disposition
   SET status = `+status+`, source = `+source+`, pending_reason = NULL, next_attempt_at = NULL, updated_at = now()
 WHERE domain = `+row+` AND status = 'pending' AND pending_reason = `+reason+` RETURNING id`, args...).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			// The row is locked, so only its own state refuses: no open vendor question.
			return fmt.Errorf("contacts: %s has no open vendor question: %w", base, apperrors.ErrConflict)
		}
		if err != nil {
			return fmt.Errorf("contacts: keeping %s as a vendor: %w", base, err)
		}
		_, err = storekit.Audit(ctx, tx, "update", entityCompany, id,
			map[string]any{auditKeyDomain: base, auditKeyStatus: prior.Status, auditKeySource: prior.Source, domainPendingReasonKey: prior.PendingReason},
			map[string]any{auditKeyDomain: base, auditKeyStatus: DomainProvider, auditKeySource: DomainSourceHuman, domainPendingReasonKey: ""})
		return err
	})
}

// vendorMailSQL selects the inbound mail a contact at the domain sent inside
// the shared waiting horizon (storekit.WaitingHorizonSQL), archived or not.
// The From participant names the sender, never the page's contact.
//
// The host is matched through idx_aparticipant_sender_domain_reversed. Without
// it every question would read all inbound mail in the horizon.
func vendorMailSQL(mail, domain string, asOf time.Time, arg func(any) int) string {
	instant := fmt.Sprintf("$%d::timestamptz", arg(asOf))
	return mail + `.kind = 'email' AND ` + mail + `.direction = 'inbound'
 AND EXISTS (SELECT 1 FROM activity_participant vendor_sender
  WHERE vendor_sender.activity_id = ` + mail + `.id AND vendor_sender.role = 'from'
  AND vendor_sender.address_domain_folded IS NOT NULL
  AND ` + owndomain.CoversIndexedSQL("vendor_sender.address_domain_folded", domain) + `
  AND EXISTS (SELECT 1 FROM contact_email vendor_contact WHERE vendor_contact.email = lower(vendor_sender.address) AND vendor_contact.archived_at IS NULL))
 AND ` + mail + `.occurred_at <= ` + instant + `
 AND ` + mail + `.occurred_at >= ` + instant + ` - make_interval(days => ` + storekit.WaitingHorizonSQL(asOf, arg) + `)`
}

// vendorDeciderSQL decides who answers a vendor question. The newest staff
// mail controls, for every reader alike, and its sole direct To seat decides.
// Same-instant mail naming another seat or none leaves no decider; an older
// mail never takes over. The decider needs activity read, and that same mail
// must be current and inside their scope (auth.ActivityContentClause).
func vendorDeciderSQL(ctx context.Context, domain, reader, identities string, asOf time.Time, arg func(any) int) (string, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); errors.Is(err, apperrors.ErrPermissionDenied) {
		// Still names the bound identities, so their placeholder keeps its type.
		return "(" + identities + " IS NULL AND false)", nil
	} else if err != nil {
		return "", err
	}
	sole := storekit.SoleDirectMailSeatSQL("vendor_mail", identities)
	readable, err := auth.ActivityContentClause(ctx, "vendor_mail", arg)
	if err != nil {
		return "", err
	}
	controlling := `vendor_mail.occurred_at = (SELECT max(vendor_latest.occurred_at) FROM activity vendor_latest WHERE ` +
		vendorMailSQL("vendor_latest", domain, asOf, arg) + `)`
	decider := `(SELECT CASE WHEN bool_and(vendor_seat.seat IS NOT NULL) AND count(DISTINCT vendor_seat.seat) = 1
   THEN (array_agg(vendor_seat.seat))[1] END
 FROM (SELECT ` + sole + ` AS seat FROM activity vendor_mail WHERE ` + vendorMailSQL("vendor_mail", domain, asOf, arg) + ` AND ` + controlling + `) vendor_seat)`
	return `(` + decider + ` = ` + reader + ` AND EXISTS (SELECT 1 FROM activity vendor_mail WHERE ` + vendorMailSQL("vendor_mail", domain, asOf, arg) + `
 AND ` + controlling + ` AND ` + sole + ` = ` + reader + `
 AND ` + storekit.CurrentSourceMailClause("vendor_mail.id", asOf, arg) + ` AND ` + readable + `))`, nil
}

// openQuestionForReaderSQL is the shared test of whether a reader holds an
// open domain question. Every pending reason belongs to one decider, the sole
// evidenced direct recipient of the newest current staff mail the reader may
// read (vendorDeciderSQL). Whose mailbox received the mail decides nothing: a
// question with no decider stays on the record (domainStandingWhere).
func openQuestionForReaderSQL(ctx context.Context, domain, reader, identities string, asOf time.Time, arg func(any) int) (string, error) {
	decider, err := vendorDeciderSQL(ctx, domain, reader, identities, asOf, arg)
	if err != nil {
		return "", err
	}
	return `d.pending_reason IS NOT NULL AND ` + decider, nil
}
