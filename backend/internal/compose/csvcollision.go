// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Whether a row names a record the caller can see that the estate holds, or
// one an earlier row of the same file will create. The import meets that
// earlier record stored. The test run writes nothing and carries it instead.

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A match the caller may not see is answered as no match, so the row creates.
// Skipping it would leak: "your row was not created" tells the caller, one row
// at a time, that a colleague's private record exists. A twin costs a merge in
// the review queue. A disclosure is one no merge undoes. Not leads: the
// store's unique key on a lead's email already refuses a twin.
//
// A read-only transaction, not DedupeCompanyForCreate, whose write lock a
// preview has no business holding. The create path runs the locking version.
func (w *csvWriters) collidesWithExisting(ctx context.Context, row migration.Row) (bool, error) {
	switch w.object {
	case migration.ObjectCompany:
		return w.companyCollides(ctx, row)
	case migration.ObjectContact:
		return w.contactCollides(ctx, row)
	default:
		return false, nil
	}
}

func (w *csvWriters) companyCollides(ctx context.Context, row migration.Row) (bool, error) {
	named := newCompanyOf(textFields(row.Fields))
	candidate := contacts.CompanyCandidate{DisplayName: named.DisplayName, LegalName: named.LegalName, Address: named.Address}
	if candidate.DisplayName == "" && candidate.LegalName == "" {
		return false, nil
	}
	var visible bool
	if err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		match, err := contacts.DedupeCompany(ctx, tx, candidate)
		if match = match.Without(w.links.batch.Changes); err != nil || match.Decision == contacts.DecisionNoMatch {
			return err
		}
		visible, err = contacts.AnyCompanyMatchVisible(ctx, tx, match)
		return err
	}); err != nil {
		return false, fmt.Errorf("import: checking %q against the companies already held: %w",
			candidate.DisplayName, err)
	}
	return visible, nil
}

// rememberRow files a row the test run says will create or change a record,
// as the import leaves that record for the rows after it.
func (w *csvWriters) rememberRow(row migration.Row, outcome predictedOutcome) {
	fields, batch := textFields(row.Fields), &w.links.batch
	switch {
	case outcome == predictUpdate && w.predictedChange != nil:
		w.predictedChange(batch)
	case outcome != predictCreate && outcome != predictCollides:
	case w.object == migration.ObjectCompany:
		held := newCompanyOf(fields)
		held.Ref, held.Domains = row.ExternalID, []string{canonicalFor(fieldDomain, fields[fieldDomain])}
		batch.AddCompany(held)
	case w.object == migration.ObjectContact:
		employer := w.predictedEmployers[row.ExternalID]
		if employer.Effect == contacts.ContactEmployerCreateEffect {
			batch.AddCompany(contacts.NewCompany{DisplayName: employer.CompanyName, Domains: employer.DomainNames()})
		}
		held := w.contactCandidateOf(fields)
		batch.AddContact(contacts.NewContact{FullName: held.FullName, Emails: held.Emails, EmployerDomains: w.employerDomainsOf(row)})
	}
}

// employerDomainsOf are the domains of the company a contact row is employed
// at once it lands. That is a stored company, else the one of the file it names.
func (w *csvWriters) employerDomainsOf(row migration.Row) []string {
	employer := w.predictedEmployers[row.ExternalID]
	if employer.CompanyID != nil {
		return employer.EmployerDomains
	}
	ref, name := strings.TrimSpace(textFields(row.Fields)[csvTargetCompanyRef]), employerKey(employer.CompanyName)
	held, _ := w.links.batch.Company(func(c contacts.NewCompany) bool {
		return ref != "" && (c.Ref == ref || slices.Contains(c.Domains, ref)) ||
			name != "" && (employerKey(c.DisplayName) == name || employerKey(c.LegalName) == name)
	})
	return held.Domains
}

// changeOf is the stored record id as the predicted update leaves it.
func (w *csvWriters) changeOf(ctx context.Context, id ids.UUID, changed map[string]string, current []byte) (func(*contacts.Batch), error) {
	switch w.object {
	case migration.ObjectCompany:
		address, _, err := addressMergedOnto(current, addressFrom(changed), changed)
		if err != nil {
			return nil, err
		}
		held, err := contacts.CompanyAfterUpdate(id, current, strings.TrimSpace(changed[fieldDisplayName]),
			strings.TrimSpace(changed[fieldLegalName]), canonicalFor(fieldDomain, changed[fieldDomain]), address)
		return func(b *contacts.Batch) { b.AddCompany(held) }, err
	case migration.ObjectContact:
		var held contacts.NewContact
		err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) (err error) {
			held, err = contacts.ContactAfterUpdate(ctx, tx, ids.From[ids.ContactKind](id), current, contactUpdateFrom(changed, nil))
			return err
		})
		return func(b *contacts.Batch) { b.AddContact(held) }, err
	}
	return nil, nil
}

// collidesInFile answers whether the import, reaching this row, meets a record
// an earlier row of the file creates or changes as its duplicate.
func (w *csvWriters) collidesInFile(row migration.Row) bool {
	fields := textFields(row.Fields)
	switch w.object {
	case migration.ObjectCompany:
		return w.links.batch.CompanyLookAlike(newCompanyOf(fields))
	case migration.ObjectContact:
		return w.links.batch.ContactLookAlike(w.contactCandidateOf(fields))
	}
	return false
}

// newCompanyOf is the company a row names, as the duplicate check reads it.
func newCompanyOf(fields map[string]string) contacts.NewCompany {
	return contacts.NewCompany{
		DisplayName: strings.TrimSpace(fields[fieldDisplayName]),
		LegalName:   strings.TrimSpace(fields[fieldLegalName]), Address: addressFrom(fields),
	}
}

// contactCandidateOf is the contact a row names, as the duplicate check reads
// it. It files no pair, and its mail list is the one contactCollides read.
func (w *csvWriters) contactCandidateOf(fields map[string]string) contacts.ContactCandidate {
	var emails []string
	if email := strings.TrimSpace(fields[fieldEmail]); email != "" {
		emails = []string{email}
	}
	return contacts.ContactCandidate{FullName: contactFullName(fields), Emails: emails, ConsumerMail: w.consumerMail}
}
