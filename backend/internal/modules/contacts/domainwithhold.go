// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Why a domain's question is left OPEN rather than answered.
//
// Both readings here refuse to mint a company and say so on the row, and both
// keep status='pending' deliberately: pending is the one value every due-scan,
// queue-mark and exhaustion query treats as open, so a withheld domain stays
// retryable without teaching five other queries a second word for the same
// thing. What differs is the reason — nothing evidenced a company at all, or
// the evidence is too old to mint from today's site.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// markDispositionUnevidenced records that a domain's question is still open
// because nothing evidenced a company — the site could not be read, and the
// sender's name did not explain the domain.
//
// It keeps status='pending' deliberately. Pending is the ONE value every
// due-scan, queue-mark and exhaustion query treats as open, so a withheld
// domain stays retryable and stays visible without teaching five other queries
// a second word for the same thing.
//
// Guarded on pending so it cannot overwrite an answer a HUMAN settled: an admin
// who confirmed the company owns that verdict, and a later sweep finding the
// site still unreadable must not quietly reopen their decision.
func markDispositionUnevidenced(ctx context.Context, tx pgx.Tx, domain, evidence string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE company_domain_disposition
		   SET pending_reason = 'unevidenced', evidence = NULLIF($2, ''),
		       next_attempt_at = NULL, updated_at = now()
		 WHERE domain = $1 AND status = $3`,
		domain, evidence, DomainPending); err != nil {
		return fmt.Errorf("contacts: recording that %s evidenced no company: %w", domain, err)
	}
	return nil
}

// withholdForStaleEvidence leaves a company verdict unanswered when the newest
// mail from the domain is older than staleEvidenceYears, and reports whether it
// did.
//
// The row stays PENDING with a reason, rather than being settled: settling it
// would stop every later message re-asking, and a domain that starts writing
// again is exactly the case that should get its company. reopenWithheldDisposition
// clears this reason when new mail arrives, which is what makes the withholding
// temporary rather than a quiet refusal.
//
// Nil evidence is fresh. A row that predates the column, or a message nothing
// could date, must not lose its company over a value nobody wrote.
func withholdForStaleEvidence(
	ctx context.Context, tx pgx.Tx, in ResolveDomainTriageInput, prior DomainDisposition,
) (bool, error) {
	if prior.LastEvidenceAt == nil {
		return false, nil
	}
	// Age is measured from now at the earliest. newestEvidenceFor already refuses
	// to RECORD a future date — the header's date is the sender's to type — so
	// this floor is for a row some other writer left ahead of the clock. A
	// negative age would otherwise read as the freshest evidence possible and
	// wave the question through, which is the one direction this gate must not
	// fail in.
	age := time.Since(*prior.LastEvidenceAt)
	if age < 0 {
		age = 0
	}
	if age < staleEvidenceYears*365*24*time.Hour {
		return false, nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE company_domain_disposition
		   SET pending_reason = 'stale_evidence',
		       -- The cursor is cleared for the same reason 'unevidenced' clears
		       -- it: re-crawling cannot make old mail newer, so the sweep must
		       -- stop offering this domain until a message rearms it.
		       next_attempt_at = NULL,
		       updated_at = now()
		 WHERE domain = $1 AND status = 'pending'`, in.Domain); err != nil {
		return false, fmt.Errorf("contacts: withholding %s for stale evidence: %w", in.Domain, err)
	}
	return true, nil
}
