// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The steps of a company edit before and after its write, in the order
// writeCompanyInTx runs them.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// readCompanyToEdit runs the checks an edit owes before it reads. Then it reads
// the row it decides on, and refuses what the edit may not do to it.
func readCompanyToEdit(
	ctx context.Context, tx pgx.Tx, id ids.CompanyID, in UpdateCompanyInput, active []fieldcatalog.Column,
) (crmcontracts.Company, error) {
	if err := auth.EnsureChangeable(ctx, tx, "company", id.UUID); err != nil {
		return crmcontracts.Company{}, err
	}
	if err := lockCompanyNameWritesForEdit(ctx, tx, in); err != nil {
		return crmcontracts.Company{}, err
	}
	current, err := readCompanyRow(ctx, tx, id, storekit.LiveOnly, active)
	if err != nil {
		return crmcontracts.Company{}, fmt.Errorf("read company before update: %w", err)
	}
	if err := refuseUnreadableCompany(current, in); err != nil {
		return crmcontracts.Company{}, err
	}
	if err := checkCompanyEditText(ctx, current, in); err != nil {
		return crmcontracts.Company{}, err
	}
	return current, auth.EnsureOwnerHandOn(ctx, tx, (*ids.UUID)(current.OwnerId), in.OwnerID)
}

// followCompanyWrite is what a landed company write owes in its transaction:
// the moved address, the record of the write, and a renamed anchor.
func (s *Store) followCompanyWrite(
	ctx context.Context, tx pgx.Tx, id ids.CompanyID, p *storekit.Patch, in UpdateCompanyInput, by string,
) error {
	if err := s.relocateIfAddressMoved(ctx, tx, id, p); err != nil {
		return err
	}
	if err := recordCompanyUpdate(ctx, tx, id, p, in, by); err != nil {
		return err
	}
	if _, renamed := p.Moved()[fieldDisplayName]; renamed {
		return s.followAnchorNameTx(ctx, tx, id)
	}
	return nil
}

// companyPatchRefusal names the constraint a company write broke, or wraps the
// error when it broke none this edit knows.
func companyPatchRefusal(err error, in UpdateCompanyInput) error {
	if constraint, ok := storekit.CheckViolation(err); ok && constraint == "company_owner_private_names_its_owner" {
		return &RequiredFieldError{Field: filterOwnerID}
	}
	if constraint, ok := storekit.UniqueViolation(err); ok && constraint == companyLinkedInKey {
		return &DuplicateCompanyLinkedInError{URL: deref(in.LinkedInURL)}
	}
	if constraint, ok := storekit.UniqueViolation(err); ok && constraint == companySourceKey {
		return fmt.Errorf("a company with this source_system and source_id already exists: %w", apperrors.ErrConflict)
	}
	return fmt.Errorf("apply company patch: %w", err)
}
