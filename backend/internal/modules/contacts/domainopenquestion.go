// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A domain whose company question is OPEN — asked, and answered by nobody.
//
// The admission ledger beside this file records decisions. This file names the
// other state a row can be in: pending with a reason, which is the machine
// saying it declined to answer rather than that it answered no. Both readings
// clear the retry cursor, so such a row is not merely waiting its turn — nothing
// will ask again until new mail arrives or somebody here does.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Why a domain's question was left open. These are the two values
// company_domain_disposition.pending_reason may carry, and they reach an
// operator as the SOURCE of an undecided entry: what stopped the machine
// deciding is the same question as what decided it, asked of a row nothing
// decided.
const (
	// PendingUnevidenced — nothing the crawl found named a company, and the
	// sender's own name did not explain the domain.
	PendingUnevidenced = "unevidenced"
	// PendingStaleEvidence — the newest mail from the domain is older than
	// staleEvidenceYears, so today's site is not evidence about the people who
	// wrote then.
	PendingStaleEvidence = "stale_evidence"
)

// DomainUndecided is the admission value an open question carries on the wire.
//
// It is never stored. The table's admission_shape constraint makes the four
// admission columns all-or-nothing, so a row that nobody decided holds NULL in
// every one of them — this word is what their absence is called to a reader,
// synthesized where the list is read.
const DomainUndecided = "undecided"

// ReopenWithheldDomain puts an open question back in the triage sweep's path.
//
// A withheld domain has no next_attempt_at: the sweep stopped offering it
// because re-crawling cannot make old mail newer, nor make an unreadable site
// readable. That is right for a machine and wrong for a contact, who may know
// something the crawl cannot — that the company moved, or that the domain is
// worth another look today. This is the verb that says so.
//
// It takes the same gate a decision does. Re-asking is not a smaller act than
// refusing: the answer this re-opens is what creates the company, and a seat
// that may not decide a domain may not queue one either.
func (s *Store) ReopenWithheldDomain(ctx context.Context, domain string) (BlockedDomain, error) {
	if err := auth.Require(ctx, entityCompany, principal.ActionUpdate); err != nil {
		return BlockedDomain{}, err
	}
	base, ok := freemail.Hostname(domain)
	if !ok {
		return BlockedDomain{}, fmt.Errorf("contacts: %q is not a domain", domain)
	}
	// The human is stamped as the domain's owner where it had none. Triage
	// refuses to mint rows for a domain nobody is accountable for, so a row
	// re-opened under nobody's name would come straight back to this list
	// having spent an attempt and answered nothing.
	owner := actingHuman(ctx)
	if owner == nil {
		return BlockedDomain{}, fmt.Errorf("contacts: re-asking about %s needs a human to answer for it", base)
	}
	var stored BlockedDomain
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The same lock the decision writer takes. Without it a suppression
		// committing between this read and the update below would leave an
		// audit row claiming a reason was cleared that the write never
		// touched — the guard's WHERE refuses a suppressed domain, so the
		// update matches nothing and the audit describes a change the table
		// does not carry.
		if err := lockDomainAdmissionTx(ctx, tx, base); err != nil {
			return err
		}
		before, err := readOpenQuestionTx(ctx, tx, base)
		if err != nil {
			return err
		}
		if before.Admission != DomainUndecided {
			// Not an open question: either a decision stands on it, or the
			// domain was settled. Both are answered, and re-asking would
			// reopen something nobody asked to reopen. A CONFLICT rather than
			// a validation fault — the domain is well formed and the request
			// is intelligible; it is the row's state that refuses.
			return fmt.Errorf("contacts: %s has an answer already; nothing to re-ask: %w", base, apperrors.ErrConflict)
		}
		// evidenceAt is nil: this re-ask is the caller's own judgement, not a
		// new message. Passing a date here would claim mail arrived that did
		// not, and GREATEST would then hold that claim against the next real
		// message's.
		if err := reopenWithheldDispositionTx(ctx, tx, base, *owner, nil); err != nil {
			return err
		}
		// The answer is the row as it STOOD, not as it is now. Re-opening
		// clears the withholding reason, and a row with neither a reason nor a
		// decision is not a standing this surface has a word for — it is a
		// domain queued for a crawl, which is why it drops out of the list
		// entirely. Re-reading it would answer with an empty admission and an
		// empty source, neither of which the contract publishes. What the
		// caller needs told is what was withheld and has now been asked again,
		// and that is exactly the row they were looking at.
		stored = before
		// Audit-only (EVT-NOEVT-3), the same door the admission decisions take:
		// capture posture is not a record change the event stream carries, and
		// it IS a decision somebody must answer for.
		_, err = storekit.Audit(ctx, tx, "update", entityCompany, before.ID,
			map[string]any{auditKeyDomain: base, "pending_reason": before.Source},
			map[string]any{auditKeyDomain: base, "pending_reason": ""})
		return err
	})
	if err != nil {
		return BlockedDomain{}, err
	}
	return stored, nil
}

// readOpenQuestionTx reads one domain the way the list does, so what a re-ask
// answers with is the same row shape the operator was looking at.
func readOpenQuestionTx(ctx context.Context, tx pgx.Tx, domain string) (BlockedDomain, error) {
	var d BlockedDomain
	var companyID *ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT id, domain, `+domainStanding+`
		  FROM company_domain_disposition WHERE domain = $1`, domain).
		Scan(&d.ID, &d.Domain, &d.Admission, &d.Reason, &d.Source, &d.DecidedAt, &companyID)
	if errors.Is(err, pgx.ErrNoRows) {
		// Nothing has ever asked about this domain, so there is no question to
		// re-open. Not-found rather than a fault: the caller named a domain
		// this installation holds no row for.
		return BlockedDomain{}, fmt.Errorf("contacts: no open question about %s: %w", domain, apperrors.ErrNotFound)
	}
	if err != nil {
		return BlockedDomain{}, fmt.Errorf("contacts: reading where %s stands: %w", domain, err)
	}
	if companyID != nil {
		// Withheld unless the caller could read that company, exactly as the
		// list withholds it. A company captured from mail is owner-PRIVATE
		// until somebody promotes it, and that privacy does not yield to
		// row_scope=all — so answering with the id here would hand a colleague
		// a pointer to a record the record's own endpoint correctly 404s.
		visible, verr := auth.VisibleTo(ctx, tx, entityCompany, *companyID)
		if verr != nil {
			return BlockedDomain{}, verr
		}
		if visible {
			typed := ids.From[ids.CompanyKind](*companyID)
			d.CompanyID = &typed
		}
	}
	return d, nil
}
