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
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A match the caller MAY NOT SEE is answered as no match at all, so the row goes
// on to create.
//
// Skipping it instead is the intuitive choice and it leaks. Hiding which company
// was hit is not enough: the outcome still says one was, and "your row was not
// created" answers "is this company in your CRM" — for a colleague's
// owner-private capture, probed one CSV row at a time, against a finished report
// readable on the import_run grant alone.
//
// What creating costs is a twin of a record the caller cannot see, which the
// dedupe review queue picks up like any other and a merge resolves. What skipping
// costs is a disclosure no merge undoes.
//
// Not leads: a lead's identity is its email, which the store's own unique key
// already refuses, so there is no silent twin to warn about.
//
// A read-only transaction, and NOT DedupeCompanyForCreate — that one takes
// a write lock to serialize concurrent creates, which a preview has no business
// holding. The answer can go stale between the preview and the commit; the
// create path runs the locking version itself and its answer is the one that
// decides.
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
		if err != nil || match.Decision == contacts.DecisionNoMatch {
			return err
		}
		// Every candidate over the threshold is asked, not only the winner. The
		// winner is ranked by score and id, not by who may read it. So a hidden
		// record ranked first would mask a visible one behind it. No early exit
		// either: the number of queries would tell where the visible one sits.
		for _, candidate := range candidatesOf(match) {
			seen, err := auth.VisibleTo(ctx, tx, "company", candidate.UUID)
			if err != nil {
				return err
			}
			visible = visible || seen
		}
		return nil
	}); err != nil {
		return false, fmt.Errorf("import: checking %q against the companies already held: %w",
			candidate.DisplayName, err)
	}
	return visible, nil
}

// candidatesOf answers every company the ladder matched, best first.
//
// Held by: TestEveryMatchedCandidateIsAskedAboutNotJustTheWinner
// (backend/internal/compose/csvcollisionvisibility_test.go)
//
// Ranked is the full set at or above the review threshold and is what a
// visibility question must be asked of. It is empty for an exact (domain)
// collision, which carries its answer in CompanyID instead — so that one is
// added when Ranked has nothing, and never twice.
func candidatesOf(match contacts.CompanyMatch) []ids.CompanyID {
	if len(match.Ranked) == 0 {
		return []ids.CompanyID{match.CompanyID}
	}
	out := make([]ids.CompanyID, 0, len(match.Ranked))
	for _, scored := range match.Ranked {
		out = append(out, scored.CompanyID)
	}
	return out
}

// fileCreates are the records earlier rows of a test run's file will create.
type fileCreates struct {
	companies []contacts.NewCompany
	contacts  []contacts.NewContact
}

// rememberCreate files a row the test run says will create a record.
func (w *csvWriters) rememberCreate(row migration.Row) {
	fields := textFields(row.Fields)
	switch w.object {
	case migration.ObjectCompany:
		held := newCompanyOf(fields)
		held.Domains = []string{strings.TrimSpace(fields[fieldDomain])}
		w.fileCreates.companies = append(w.fileCreates.companies, held)
	case migration.ObjectContact:
		held := w.contactCandidateOf(fields)
		w.fileCreates.contacts = append(w.fileCreates.contacts, contacts.NewContact{FullName: held.FullName, Emails: held.Emails})
	}
}

// collidesInFile answers whether the import, reaching this row, meets an
// earlier new record of the file as its duplicate. The caller sees each one.
func (w *csvWriters) collidesInFile(row migration.Row) bool {
	fields := textFields(row.Fields)
	switch w.object {
	case migration.ObjectCompany:
		asked := newCompanyOf(fields)
		return slices.ContainsFunc(w.fileCreates.companies, func(held contacts.NewCompany) bool { return contacts.CompanyLookAlike(asked, held) })
	case migration.ObjectContact:
		asked := w.contactCandidateOf(fields)
		return slices.ContainsFunc(w.fileCreates.contacts, func(held contacts.NewContact) bool { return contacts.ContactLookAlike(asked, held) })
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
