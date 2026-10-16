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
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// inFileLedger is what the dry run has counted as landing so far.
//
// Nothing here is filtered for visibility. The commit's ladder sees every
// earlier row, because contacts and companies are identity tables any reader
// sees whole (auth's identityTables). Only capture privacy narrows one, and
// the typed create an import uses writes contacts.TypedCreateVisibility.
type inFileLedger struct {
	writers *csvWriters
	// holders maps each unique key an earlier row will hold to the record
	// holding it: "row:<ref>" for a create, "id:<uuid>" for an update.
	holders map[string]string
	// freedBy maps each key earlier updates free to the records that held it.
	// The estate still shows them holding it, so a probe leaves them out.
	freedBy map[string][]ids.UUID
	// claimed and freeing are the row under prediction's keys, filed once it
	// is known to land.
	claimed     []rowKey
	claimant    string
	freeing     []string
	freer       ids.UUID
	companies   contacts.CompanyBatch
	contactRows contacts.ContactBatch
}

func newInFileLedger(ctx context.Context, writers *csvWriters) (*inFileLedger, error) {
	ledger := &inFileLedger{writers: writers, holders: map[string]string{}, freedBy: map[string][]ids.UUID{}}
	if writers.object != migration.ObjectContact {
		return ledger, nil
	}
	// Read up front, because the contact ladder judges a shared mail domain by
	// the workspace's own list and record runs outside a transaction.
	if err := database.WithWorkspaceTx(ctx, writers.pool, func(tx pgx.Tx) error {
		_, err := writers.consumerMailList(ctx, tx)
		return err
	}); err != nil {
		return nil, fmt.Errorf("import: reading the consumer mail list: %w", err)
	}
	return ledger, nil
}

// claim notes the keys the row under prediction would hold, and for which
// record. A nil ledger is the commit's, which has no earlier rows to carry.
func (f *inFileLedger) claim(holder string, keys []rowKey) {
	if f != nil {
		f.claimant, f.claimed = holder, keys
	}
}

// release notes the keys the update under prediction frees from record id.
func (f *inFileLedger) release(id ids.UUID, keys []string) {
	if f != nil {
		f.freer, f.freeing = id, keys
	}
}

// leaveOut is except plus the records earlier updates freed key from.
func (f *inFileLedger) leaveOut(key string, except []ids.UUID) []ids.UUID {
	if f == nil || len(f.freedBy[key]) == 0 {
		return except
	}
	return append(slices.Clone(except), f.freedBy[key]...)
}

// heldByAnother reports whether an earlier row's record other than holder
// will hold key.
func (f *inFileLedger) heldByAnother(key, holder string) bool {
	if f == nil {
		return false
	}
	held, ok := f.holders[key]
	return ok && held != holder
}

// reconsider answers what the commit does with a row the estate alone said
// would create. By then the file's earlier rows are in the estate too, and
// the collision ladder may meet one of them.
func (f *inFileLedger) reconsider(
	ctx context.Context, row migration.Row, outcome predictedOutcome, problem *migration.SkippedRow,
) (predictedOutcome, *migration.SkippedRow, error) {
	if outcome != predictCreate {
		return outcome, problem, nil
	}
	rival, err := f.hasRival(ctx, textFields(row.Fields))
	if err != nil || !rival {
		return outcome, problem, err
	}
	if f.writers.onDuplicate == string(crmcontracts.ImportOnDuplicateSkip) {
		return predictCollidesSkipped, nil, nil
	}
	return predictCollides, problem, nil
}

// record files the row's claimed keys when the prediction says it lands. It
// files the names the row lands under as rivals for later rows, by record. A
// record two rows write is then met under the names it was written last.
func (f *inFileLedger) record(row migration.Row, outcome predictedOutcome) {
	claimed, claimant, freeing, freer := f.claimed, f.claimant, f.freeing, f.freer
	f.claimed, f.claimant, f.freeing, f.freer = nil, "", nil, ids.UUID{}
	if outcome != predictCreate && outcome != predictCollides && outcome != predictUpdate {
		return
	}
	// Freed before claimed: an update that only recases a company's URL frees
	// and claims one key.
	for _, key := range freeing {
		f.freedBy[key] = append(f.freedBy[key], freer)
		if f.holders[key] == claimant {
			delete(f.holders, key)
		}
	}
	for _, key := range claimed {
		f.holders[key.ledger] = claimant
	}
	fields := textFields(row.Fields)
	switch f.writers.object {
	case migration.ObjectCompany:
		held := companyCandidateOf(fields)
		if domain := canonicalFor(fieldDomain, fields[fieldDomain]); domain != "" {
			held.Domains = []string{domain}
		}
		f.companies.Put(claimant, held)
	case migration.ObjectContact:
		f.contactRows.Put(claimant, contactCandidateOf(fields, f.writers.consumerMail))
	}
}

// hasRival answers whether the ladder would meet an earlier create as this
// row's duplicate. The scorers run first, so a row they clear opens no
// transaction.
func (f *inFileLedger) hasRival(ctx context.Context, fields map[string]string) (bool, error) {
	var admit func(tx pgx.Tx) (bool, error)
	switch f.writers.object {
	case migration.ObjectCompany:
		rivals := f.companies.Rivals(companyCandidateOf(fields))
		if settled, rival := rivals.Settled(); settled {
			return rival, nil
		}
		admit = func(tx pgx.Tx) (bool, error) { return rivals.Admitted(ctx, tx) }
	case migration.ObjectContact:
		candidate := contactCandidateOf(fields, f.writers.consumerMail)
		rivals := f.contactRows.Rivals(candidate)
		if !rivals.Any() {
			return false, nil
		}
		admit = func(tx pgx.Tx) (bool, error) { return contactOutranksEstate(ctx, tx, rivals, candidate) }
	default:
		return false, nil
	}
	var rival bool
	if err := database.WithWorkspaceTx(ctx, f.writers.pool, func(tx pgx.Tx) error {
		var err error
		rival, err = admit(tx)
		return err
	}); err != nil {
		return false, fmt.Errorf("import: checking a row against the file's earlier rows: %w", err)
	}
	return rival, nil
}

// contactOutranksEstate weighs the best earlier create against the estate.
// DedupeContact keeps one winner, and the estate's answer here was no visible
// collision. So an estate contact scoring at least as high wins the commit's
// ladder, being invisible, and hides the earlier create. A tie goes to the
// lower id, which the older estate record holds.
func contactOutranksEstate(ctx context.Context, tx pgx.Tx, rivals contacts.ContactRivals, candidate contacts.ContactCandidate) (bool, error) {
	best, err := rivals.Best(ctx, tx)
	if err != nil || best == 0 {
		return false, err
	}
	estate, err := contacts.DedupeContact(ctx, tx, candidate)
	if err != nil {
		return false, err
	}
	return estate.Decision == contacts.DecisionNoMatch || estate.Confidence < best, nil
}
