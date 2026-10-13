// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The rows of the file the dry run has already counted as landing.
//
// The commit lands rows one at a time. When it reaches a row, every earlier
// row of the same file is already in the estate. The collision ladder and the
// key checks meet it there. The dry run writes nothing, so it carries those
// earlier rows here.

import (
	"context"
	"fmt"
	"strings"

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
	// holders maps each estate-wide unique key an earlier row will hold (an
	// address, a domain, a LinkedIn URL) to the record holding it. The store
	// refuses the key to any other record, whoever holds it.
	holders     map[string]string
	companies   contacts.CompanyBatch
	contactRows contacts.ContactBatch
	// consumerMail is read once per walk, because the contact ladder judges a
	// shared mail domain by the workspace's own list.
	consumerMail *freemail.Matcher
}

// heldKey is one unique key a row claims, with the sentence the commit
// refuses a second holder with on each path.
type heldKey struct {
	key, onCreate, onUpdate string
}

func newInFileCreates(ctx context.Context, writers *csvWriters) (*inFileCreates, error) {
	ledger := &inFileCreates{writers: writers, holders: map[string]string{}}
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
// would land or be skipped as a duplicate. By then the file's earlier rows are
// in the estate too.
//
// The order is Ensure's: a taken key first, then the collision check.
func (f *inFileCreates) reconsider(
	ctx context.Context, row migration.Row, outcome predictedOutcome, problem *migration.SkippedRow,
) (predictedOutcome, *migration.SkippedRow, error) {
	switch outcome {
	case predictCreate, predictCollides, predictCollidesSkipped, predictUpdate:
	default:
		return outcome, problem, nil
	}
	fields := textFields(row.Fields)
	if reason := f.takenKeyReason(row, fields, outcome == predictUpdate); reason != "" {
		return predictUnwritable, &migration.SkippedRow{ExternalID: row.ExternalID, Line: row.Line, Reason: reason}, nil
	}
	if outcome != predictCreate {
		return outcome, problem, nil
	}
	rival, err := f.hasRival(ctx, fields)
	if err != nil || !rival {
		return outcome, problem, err
	}
	if f.writers.onDuplicate == string(crmcontracts.ImportOnDuplicateSkip) {
		return predictCollidesSkipped, nil, nil
	}
	return predictCollides, problem, nil
}

// takenKeyReason is the refusal for a key of this row an earlier row's record
// already holds, or "".
func (f *inFileCreates) takenKeyReason(row migration.Row, fields map[string]string, update bool) string {
	holder := recordOf(row, fields)
	for _, k := range f.keysOf(fields) {
		if held, ok := f.holders[k.key]; ok && held != holder {
			if update {
				return k.onUpdate
			}
			return k.onCreate
		}
	}
	return ""
}

// record files a row the prediction says will land. A row that updates files
// its keys too: once it commits, the estate holds them for its record.
func (f *inFileCreates) record(row migration.Row, outcome predictedOutcome) {
	if outcome != predictCreate && outcome != predictCollides && outcome != predictUpdate {
		return
	}
	fields := textFields(row.Fields)
	holder := recordOf(row, fields)
	for _, k := range f.keysOf(fields) {
		f.holders[k.key] = holder
	}
	if outcome == predictUpdate {
		return
	}
	switch f.writers.object {
	case migration.ObjectCompany:
		held := companyCandidateOf(fields)
		if domain := canonicalFor(fieldDomain, fields[fieldDomain]); domain != "" {
			held.Domains = []string{domain}
		}
		f.companies.Add(held)
	case migration.ObjectContact:
		f.contactRows.Add(contactCandidateOf(fields, f.consumerMail))
	}
}

// recordOf names the record a row writes: the company its id column names,
// else the one its external id lands or updates.
func recordOf(row migration.Row, fields map[string]string) string {
	if named := strings.ToLower(strings.TrimSpace(fields[csvTargetID])); named != "" {
		return "id:" + named
	}
	return "row:" + row.ExternalID
}

// keysOf is the row's estate-wide unique keys, spelled as the store holds
// them. A contact's LinkedIn handle is not a key.
func (f *inFileCreates) keysOf(fields map[string]string) []heldKey {
	var keys []heldKey
	object := f.writers.object
	if email := canonicalFor(fieldEmail, fields[fieldEmail]); email != "" {
		switch object {
		case migration.ObjectContact:
			keys = append(keys, heldKey{"email:" + email, contactEmailClaimedReason, skipReasonDuplicateEmail})
		case migration.ObjectLead:
			keys = append(keys, heldKey{"email:" + email, leadEmailClaimedReason, leadEmailClaimedReason})
		}
	}
	if object == migration.ObjectCompany {
		if domain := canonicalFor(fieldDomain, fields[fieldDomain]); domain != "" {
			keys = append(keys, heldKey{"domain:" + domain, domainClaimedReason, domainClaimedReason})
		}
	}
	if object == migration.ObjectContact {
		return keys
	}
	text := strings.TrimSpace(fields[fieldLinkedIn])
	if url, err := storedSpelling(object, fieldLinkedIn, text); text != "" && err == nil {
		keys = append(keys, heldKey{"linkedin:" + strings.ToLower(url), linkedInClaimedReason, linkedInClaimedReason})
	}
	return keys
}

// hasRival answers whether the ladder would meet an earlier create as this
// row's duplicate.
func (f *inFileCreates) hasRival(ctx context.Context, fields map[string]string) (bool, error) {
	var rival bool
	err := database.WithWorkspaceTx(ctx, f.writers.pool, func(tx pgx.Tx) error {
		var err error
		switch f.writers.object {
		case migration.ObjectCompany:
			rival, err = f.companies.HasRival(ctx, tx, companyCandidateOf(fields))
		case migration.ObjectContact:
			rival, err = f.contactHasRival(ctx, tx, contactCandidateOf(fields, f.consumerMail))
		}
		return err
	})
	if err != nil {
		return false, fmt.Errorf("import: checking a row against the file's earlier rows: %w", err)
	}
	return rival, nil
}

// contactHasRival weighs the best earlier create against the estate.
// DedupeContact keeps one winner, and the estate's answer here was no visible
// collision. So an estate contact scoring at least as high wins the commit's
// ladder, being invisible, and hides the earlier create. A tie goes to the
// lower id, which the older estate record holds.
func (f *inFileCreates) contactHasRival(ctx context.Context, tx pgx.Tx, candidate contacts.ContactCandidate) (bool, error) {
	best, err := f.contactRows.BestRival(ctx, tx, candidate)
	if err != nil || best == 0 {
		return false, err
	}
	estate, err := contacts.DedupeContact(ctx, tx, candidate)
	if err != nil {
		return false, err
	}
	return estate.Decision == contacts.DecisionNoMatch || estate.Confidence < best, nil
}
