// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The name this installation issues documents under, when a human has said so.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// ConfirmedIssuerLegalName is the anchor company's legal name when a human
// stands behind it, and the empty string when nobody does yet.
//
// An offer names two companies and, until this read existed, sourced them by
// two different rules: the BUYER block took `legal_name` off a company record
// with its whole provenance sidecar behind it, while the ISSUER — the
// party making the legal claim on the same page — took a settings value with no
// provenance at all. One document, two companies, and the one with less
// evidence was the one signing it.
//
// VERIFIED, not merely present, and that is the whole safety of it. Enrichment
// reads a public website and proposes a legal name; a confirmation is a human
// agreeing to it (company_evidence_write.go stamps verified_at, verified_by and
// source=human together). Printing an unconfirmed proposal would let a site
// read re-brand the next quote an installation sends, which is exactly the
// silent change this read is shaped to refuse.
//
// The empty string means "keep printing what you were printing". A caller that
// treated it as an error would take down the render of an installation that has
// simply not confirmed anything.
//
// NO COMPANY GRANT is asked, and that is a ruling rather than an omission
// (gates/companyreaders_test.go records it). The company object governs reading
// the accounts an installation SELLS TO. This read answers what the
// installation itself is called on its own documents — a fact every seat
// already reads off every quote it opens, and one a rep sending an offer must
// be able to resolve without also holding the account list.
func ConfirmedIssuerLegalName(ctx context.Context, tx pgx.Tx) (string, error) {
	companyID, err := anchorCompany(ctx, tx, false)
	if errors.Is(err, apperrors.ErrNotFound) {
		// No anchor company: an installation that has not described itself yet.
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var legalName *string
	err = tx.QueryRow(ctx, `
		SELECT c.legal_name
		  FROM company c
		  JOIN company_profile_field f ON f.company_id = c.id AND f.field = 'legal_name'
		 WHERE c.id = $1 AND f.verified_at IS NOT NULL`, companyID).Scan(&legalName)
	if errors.Is(err, pgx.ErrNoRows) {
		// Either nothing proposed a legal name, or nobody has confirmed the
		// proposal. Both mean the same thing here.
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read the anchor company's confirmed legal name: %w", err)
	}
	if legalName == nil {
		// The sidecar was confirmed and the column is empty — a confirmation of
		// a removal. Nothing to print.
		return "", nil
	}
	return *legalName, nil
}
