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

// vendorDeciderSQL holds a vendor question to the one seat that owes it now.
// That is the sole direct To seat (storekit.SoleDirectMailSeatSQL) of a current
// inbound mail from a contact at the domain (storekit.CurrentSourceMailClause).
// Mailbox delivery, a copy, or an old reader row is no such evidence. A
// question with no such seat stays on the record only.
func vendorDeciderSQL(domain, reader, identities string, asOf time.Time, arg func(any) int) string {
	host := `split_part(lower(vendor_sender.address), '@', 2)`
	return `EXISTS (SELECT 1 FROM activity vendor_mail JOIN activity_participant vendor_sender
 ON vendor_sender.activity_id = vendor_mail.id AND vendor_sender.role = 'from'
 WHERE vendor_mail.kind = 'email' AND vendor_mail.direction = 'inbound'
 AND (` + host + ` = ` + domain + ` OR ` + host + ` LIKE '%.' || ` + domain + `)
 AND EXISTS (SELECT 1 FROM contact_email vendor_contact WHERE vendor_contact.email = lower(vendor_sender.address) AND vendor_contact.archived_at IS NULL)
 AND ` + storekit.CurrentSourceMailClause("vendor_mail.id", asOf, arg) + `
 AND ` + storekit.SoleDirectMailSeatSQL("vendor_mail", identities) + ` = ` + reader + `)`
}

// openQuestionForReaderSQL is the shared test of whether a reader holds an
// open domain question. Every reason needs a reader row; a vendor question
// also needs its decider.
func openQuestionForReaderSQL(domain, reader, identities string, asOf time.Time, arg func(any) int) string {
	return `d.pending_reason IS NOT NULL AND EXISTS (SELECT 1 FROM company_domain_question_reader r WHERE r.domain = ` + domain + ` AND r.reader_id = ` + reader + `)
 AND (d.pending_reason <> '` + PendingVendorVerdict + `' OR ` + vendorDeciderSQL(domain, reader, identities, asOf, arg) + `)`
}
