// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Whether a row the CRM holds no record for may land as a new one. A company
// row on a held domain may land on its holder instead. The test run and the import both
// ask here, so they reach one verdict per row. The run's ledger carries the
// file's earlier rows to the test run, which writes nothing.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/compose/importkeys"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// linkedInClaimed reports whether the store refused a write because another
// live company or lead holds its LinkedIn URL.
func linkedInClaimed(err error) bool {
	if _, ok := errors.AsType[*contacts.DuplicateCompanyLinkedInError](err); ok {
		return true
	}
	_, ok := errors.AsType[*contacts.DuplicateLeadLinkedInError](err)
	return ok
}

// keysOf spells the keys a row of this writer's type holds, the way the store
// compares them. A company's LinkedIn URL and domain compare without case. A
// lead's or contact's LinkedIn URL keeps the case of its path.
func (w *csvWriters) keysOf(row migration.Row, typed map[string]any) map[importkeys.Kind]string {
	fields := textFields(row.Fields)
	keys := map[importkeys.Kind]string{}
	if url, ok := typed[fieldLinkedIn].(string); ok {
		keys[importkeys.LinkedIn] = strings.TrimSpace(url)
	}
	switch w.object {
	case migration.ObjectContact, migration.ObjectLead:
		keys[importkeys.Email] = canonicalFor(fieldEmail, fields[fieldEmail])
	case migration.ObjectCompany:
		keys[importkeys.Domain] = canonicalFor(fieldDomain, fields[fieldDomain])
		keys[importkeys.LinkedIn] = strings.ToLower(keys[importkeys.LinkedIn])
	}
	return keys
}

// claimRow records a row that lands, so a later row of the run meets its keys.
func (w *csvWriters) claimRow(row migration.Row, typed map[string]any) {
	if w.links.claims == nil {
		w.links.claims = importkeys.Ledger{}
	}
	w.links.claims.Claim(w.object, w.keysOf(row, typed))
}

// movedKeyKinds are the columns whose change moves a key, in the order the
// store's refusals are asked.
var movedKeyKinds = []struct {
	field string
	kind  importkeys.Kind
}{{fieldEmail, importkeys.Email}, {fieldLinkedIn, importkeys.LinkedIn}, {fieldDomain, importkeys.Domain}}

// movedKeyRefusal says why an update cannot move a key onto its record: an
// earlier row of the run, or another stored record, already holds it. The test
// run and the import both ask here before they write, so the preview promises
// only an update the import makes. The stored lookups are the ones the store's
// own uniqueness checks use, and they leave out the record's own values.
func (w *csvWriters) movedKeyRefusal(ctx context.Context, id ids.UUID, row migration.Row, typed map[string]any, changed map[string]string) (migration.Reason, error) {
	var moved []importkeys.Kind
	for _, key := range movedKeyKinds {
		if _, ok := changed[key.field]; ok {
			moved = append(moved, key.kind)
		}
	}
	held, found, err := importkeys.MovedKeyHeld(ctx, w.pool, w.links.claims, w.object, id, w.keysOf(row, typed), moved)
	switch {
	case err != nil || !found:
		return migration.Reason{}, err
	case held.Kind == importkeys.LinkedIn:
		return linkedInClaimedWhy, nil
	case held.Kind == importkeys.Domain:
		return domainClaimedWhy, nil
	case held.Object == migration.ObjectContact:
		return duplicateEmailWhy, nil
	case w.object == migration.ObjectLead:
		return naturalKeyTakenWhy, nil
	}
	return contactEmailClaimedWhy, nil
}

// keyCheck is one key the create door refuses: held by an earlier row of the
// run, or by a stored record.
type keyCheck struct {
	why    migration.Reason
	inFile bool
	stored func() (bool, error)
}

// keyChecks are the refusals the create door of the row's type makes, in its order.
func (w *csvWriters) keyChecks(ctx context.Context, row migration.Row, typed map[string]any) []keyCheck {
	keys, ledger := w.keysOf(row, typed), w.links.claims
	email, url := strings.TrimSpace(textFields(row.Fields)[fieldEmail]), keys[importkeys.LinkedIn]
	linkedIn := keyCheck{
		linkedInClaimedWhy, ledger.Held(importkeys.LinkedIn, url, w.object),
		func() (bool, error) { return importkeys.LinkedInHeld(ctx, w.pool, w.object, url) },
	}
	switch w.object {
	case migration.ObjectContact:
		return []keyCheck{{
			contactEmailClaimedWhy,
			ledger.Held(importkeys.Email, keys[importkeys.Email], migration.ObjectContact, migration.ObjectLead),
			func() (bool, error) { return w.contactEmailAlreadyHeld(ctx, row) },
		}}
	case migration.ObjectLead:
		return []keyCheck{
			{
				naturalKeyTakenWhy, ledger.Held(importkeys.Email, keys[importkeys.Email], migration.ObjectLead),
				func() (bool, error) { return importkeys.LeadEmailHeld(ctx, w.pool, email) },
			},
			linkedIn,
			{
				contactHoldsLeadWhy, ledger.Held(importkeys.Email, keys[importkeys.Email], migration.ObjectContact) ||
					ledger.Held(importkeys.LinkedIn, url, migration.ObjectContact),
				func() (bool, error) { return importkeys.ContactHoldsLeadKeys(ctx, w.pool, email, url) },
			},
		}
	case migration.ObjectCompany:
		domain := keys[importkeys.Domain]
		return []keyCheck{{
			domainClaimedWhy, ledger.Held(importkeys.Domain, domain, migration.ObjectCompany),
			func() (bool, error) { return importkeys.DomainHeld(ctx, w.pool, domain) },
		}, linkedIn}
	}
	return nil
}

// newRecordRefusal says why the row cannot land as a new record, or the zero
// reason when it can. A row with no name comes first: the store has nothing
// to call the record by, whatever its keys are.
func (w *csvWriters) newRecordRefusal(ctx context.Context, row migration.Row, typed map[string]any) (migration.Reason, error) {
	fields := textFields(row.Fields)
	switch {
	case w.object == migration.ObjectContact && contactFullName(fields) == "":
		return contactUnnamedWhy, nil
	case w.object == migration.ObjectCompany && strings.TrimSpace(fields[fieldDisplayName]) == "":
		return companyNameEmptyWhy, nil
	}
	for _, check := range w.keyChecks(ctx, row, typed) {
		if check.inFile {
			return check.why, nil
		}
		held, err := check.stored()
		if err != nil {
			return migration.Reason{}, err
		}
		if held {
			return check.why, nil
		}
	}
	return migration.Reason{}, nil
}

// domainHolderOf finds the company a company row lands on when no id column
// and no binding name one. It is the live company holding the row's domain,
// if the caller may change it. A domain an earlier row of this run claims is no match.
// The test run has no company yet to file that second row onto.
func (w *csvWriters) domainHolderOf(ctx context.Context, row migration.Row) (ids.UUID, bool, error) {
	domain := w.keysOf(row, nil)[importkeys.Domain]
	if w.object != migration.ObjectCompany || domain == "" || w.links.claims.Held(importkeys.Domain, domain, w.object) {
		return ids.UUID{}, false, nil
	}
	return importkeys.WritableDomainHolder(ctx, w.pool, domain)
}

// domainMatched is the warning a row filed onto its domain's holder lands
// with. It carries the domain as its value, so the report keeps it apart from
// the row's other warnings.
func domainMatched(row migration.Row) migration.SkippedRow {
	domain := canonicalFor(fieldDomain, textFields(row.Fields)[fieldDomain])
	warning := migration.SkippedRow{ExternalID: row.ExternalID, Line: row.Line, Value: domain}
	return warning.Because(migration.Why(migration.ReasonCompanyDomainMatched, "value", domain))
}

// landOnDomainHolder binds the row's ref to the company holding its domain and
// reconciles it, as a row naming that company by id would be.
func (w *csvWriters) landOnDomainHolder(ctx context.Context, row migration.Row, id ids.UUID, typed map[string]any) (migration.EnsureResult, error) {
	if err := w.reviewedTarget(row.ExternalID, id); err != nil {
		return migration.EnsureResult{}, err
	}
	if err := w.bind(ctx, row, id); err != nil {
		return migration.EnsureResult{}, err
	}
	landed, err := w.reconcile(ctx, id, row, typed)
	if err == nil && !landed.Skipped {
		landed.Warnings = append(landed.Warnings, domainMatched(row))
	}
	return landed, err
}

// predictDomainHolder is landOnDomainHolder without writing.
func (w *csvWriters) predictDomainHolder(ctx context.Context, row migration.Row, id ids.UUID, typed map[string]any) (predictedOutcome, migration.Reason, error) {
	outcome, why, err := w.reconcilePrediction(ctx, id, row, typed)
	if err != nil || outcome == predictUnwritable {
		return outcome, why, err
	}
	w.predictedWarnings = append(w.predictedWarnings, domainMatched(row))
	return outcome, why, nil
}

// landingChanges is the fields a row would change on a record it reaches.
// The file overwrites only a record this importer created or a row names by
// id. Any other record keeps every stored value, the row fills only empty
// fields, and the reason names what was kept.
func (w *csvWriters) landingChanges(ctx context.Context, row migration.Row, current []byte, typed map[string]any) (map[string]string, migration.Reason, error) {
	changed, err := changedFields(current, textFields(row.Fields), typed)
	if err != nil || strings.TrimSpace(textFields(row.Fields)[csvTargetID]) != "" {
		return changed, migration.Reason{}, err
	}
	binding, bound, err := w.bindingOf(ctx, row.ExternalID)
	owned := bound && binding.RunStatus != migration.StatusUndone && binding.Origin != migration.OriginBound
	if err != nil || owned {
		return changed, migration.Reason{}, err
	}
	var stored map[string]json.RawMessage
	if err := json.Unmarshal(current, &stored); err != nil {
		return nil, migration.Reason{}, fmt.Errorf("import: reading the stored record: %w", err)
	}
	var kept []string
	for field := range changed {
		if textOf(storedValue(stored, field)) != "" {
			delete(changed, field)
			kept = append(kept, field)
		}
	}
	if len(kept) == 0 {
		return changed, migration.Reason{}, nil
	}
	slices.Sort(kept)
	return changed, migration.Why(migration.ReasonStoredValuesKept, "column", strings.Join(kept, ", ")), nil
}
