// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The rows of the file the dry run has already counted as landing.
//
// The commit lands rows one at a time. When it reaches a row, every earlier
// create of the same file is already in the estate. The collision ladder and
// the key checks meet it there. The dry run writes nothing, so it carries
// those earlier creates here.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/freemail"
)

// inFileCreates is what the dry run has counted as landing so far.
//
// Every earlier create is one the caller may see: the import writes it under
// the caller's own principal. So, unlike the estate, nothing here is filtered
// for visibility.
type inFileCreates struct {
	writers *csvWriters
	// keys are the estate-wide unique keys the earlier creates hold: a
	// contact's address, a company's domain. A later row naming one is refused
	// by the store whoever holds it.
	keys        map[string]bool
	companies   contacts.CompanyBatch
	contactRows contacts.ContactBatch
	// consumerMail is read once per walk, because the contact ladder judges a
	// shared mail domain by the workspace's own list.
	consumerMail *freemail.Matcher
}

func newInFileCreates(ctx context.Context, writers *csvWriters) (*inFileCreates, error) {
	ledger := &inFileCreates{writers: writers, keys: map[string]bool{}}
	if writers.object != migration.ObjectContact {
		return ledger, nil
	}
	if err := database.WithWorkspaceTx(ctx, writers.pool, func(tx pgx.Tx) error {
		matcher, err := capture.MatcherTx(ctx, tx)
		ledger.consumerMail = matcher
		return err
	}); err != nil {
		return nil, fmt.Errorf("import: reading the consumer mail list: %w", err)
	}
	return ledger, nil
}

// reconsider answers what the commit does with a row the estate alone said
// would land. By then the file's earlier creates are in the estate too.
//
// The order is Ensure's: a taken key first, then the collision check.
func (f *inFileCreates) reconsider(row migration.Row, outcome predictedOutcome, refusal string) (predictedOutcome, string) {
	if outcome != predictCreate && outcome != predictCollides {
		return outcome, refusal
	}
	fields := textFields(row.Fields)
	if key, reason := f.keyOf(fields); key != "" && f.keys[key] {
		return predictUnwritable, reason
	}
	if outcome == predictCollides || !f.hasRival(fields) {
		return outcome, refusal
	}
	if f.writers.onDuplicate == string(crmcontracts.ImportOnDuplicateSkip) {
		return predictCollidesSkipped, ""
	}
	return predictCollides, ""
}

// record files a row the prediction says will land.
func (f *inFileCreates) record(row migration.Row, outcome predictedOutcome) {
	if outcome != predictCreate && outcome != predictCollides {
		return
	}
	fields := textFields(row.Fields)
	key, _ := f.keyOf(fields)
	if key != "" {
		f.keys[key] = true
	}
	switch f.writers.object {
	case migration.ObjectCompany:
		held := companyCandidateOf(fields)
		if key != "" {
			held.Domains = []string{key}
		}
		f.companies.Add(held)
	case migration.ObjectContact:
		f.contactRows.Add(contactCandidateOf(fields, f.consumerMail))
	}
}

// keyOf is the row's estate-wide unique key, spelled as the store holds it,
// with the sentence the commit refuses a second holder with.
func (f *inFileCreates) keyOf(fields map[string]string) (string, string) {
	switch f.writers.object {
	case migration.ObjectCompany:
		if domain := canonicalFor(fieldDomain, fields[fieldDomain]); domain != "" {
			return domain, domainClaimedReason
		}
	case migration.ObjectContact:
		if email := canonicalFor(fieldEmail, fields[fieldEmail]); email != "" {
			return email, contactEmailClaimedReason
		}
	}
	return "", ""
}

// hasRival answers whether the ladder would meet an earlier create as this
// row's duplicate.
func (f *inFileCreates) hasRival(fields map[string]string) bool {
	switch f.writers.object {
	case migration.ObjectCompany:
		return f.companies.HasRival(companyCandidateOf(fields))
	case migration.ObjectContact:
		return f.contactRows.HasRival(contactCandidateOf(fields, f.consumerMail))
	}
	return false
}
