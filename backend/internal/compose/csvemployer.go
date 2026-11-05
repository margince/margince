// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Import previews and writes share the contacts module's employer matching rules.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// csvEmployerName is the column naming the company a contact works for.
//
// It is not in csvTargets and never will be: that map is what a row WRITES, and
// this writes nothing to the contact. It is advertised through importTargets the
// same way `id` is, and excluded from comparison the same way — see
// isNonFieldTarget, which states why both must be.
//
// Assigned FROM the source's own constant rather than spelled again, so the
// target a caller maps to and the edge the source emits are one value by
// construction — there is no second string here to fall out of step.
const csvEmployerName = migration.AssocTargetCompanyName

// employerResolution is a name looked up, and why it produced no company when
// it did not.
type employerResolution struct {
	id    ids.CompanyID
	found bool
	why   migration.Reason
}

// resolveEmployer answers which company a company name means, or why it
// means none.
//
// Three outcomes, and the two failures are deliberately worded the same way as
// each other where it matters: a name matching nothing and a name matching only
// companies this caller may not see BOTH answer "no company of that name",
// because separating them would tell a caller that a company they cannot read
// exists — an existence oracle over a colleague's owner-private estate, probed
// one spreadsheet row at a time. That is the posture csvcollision.go documents.
func (w *csvWriters) resolveEmployer(ctx context.Context, name string) (employerResolution, error) {
	normalized := employerKey(name)
	if normalized == "" {
		return employerResolution{why: employerColumnEmptyWhy}, nil
	}
	hit, err := w.links.employerFor(ctx, normalized)
	if err != nil {
		return employerResolution{}, err
	}
	switch {
	case hit.count == 0:
		return employerResolution{why: migration.Why(migration.ReasonEmployerNotFound, "name", strings.TrimSpace(name))}, nil
	case hit.count > 1:
		return employerResolution{why: ambiguousEmployerWhy(name)}, nil
	default:
		return employerResolution{id: hit.id, found: true}, nil
	}
}

// ambiguousEmployerWhy is the answer for a name more than one visible
// company wears.
func ambiguousEmployerWhy(name string) migration.Reason {
	return migration.Why(migration.ReasonLinkAmbiguous, "name", strings.TrimSpace(name))
}

// employerKey folds a company name for comparison: case and whitespace, and
// nothing else.
//
// ITS TWIN IS f_fold_import_name IN SQL, and they cannot share an
// implementation — one runs on a spreadsheet cell in this process, the other
// inside an index Postgres maintains. They are held equal instead, by
// TestTheImportNameFoldAgreesInBothLanguages, which feeds the awkward cases
// through both and fails in either direction. A drift here is not a crash: it
// is a file that stops linking, silently, for names nobody thought to try.
//
// Deliberately weaker than NormalizeCompanyName. That one strips legal suffixes so a
// review queue can ask a human about `Acme Inc` and `Acme GmbH`; using it here
// would answer that question by writing, and the two names are two companies
// until somebody says otherwise. A file spelling a company differently from the
// CRM does not link, which is a missing link the report names rather than a wrong
// one nobody sees.
func employerKey(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(name), " "))
}

// employerCandidate is one folded name's answer: the company, and whether more
// than one wears the name.
//
// The read caps the count at two (see estateEmployer). Zero means no company
// of that name, which is also the answer for a name only hidden companies wear.
//
// pending marks an answer that counts a company of this upload. Only the dry
// run sees one, and its id may be zero.
type employerCandidate struct {
	id      ids.CompanyID
	count   int
	pending bool
}

// employerFor answers one folded name as the run will see it once this
// upload's companies have landed.
func (r *linkResolver) employerFor(ctx context.Context, key string) (employerCandidate, error) {
	hit, err := r.estateEmployer(ctx, key)
	if err != nil {
		return employerCandidate{}, err
	}
	return r.names[key].over(hit), nil
}

// estateEmployer caches the visible name matches for one import preview.
func (r *linkResolver) estateEmployer(ctx context.Context, key string) (employerCandidate, error) {
	if hit, ok := r.employers[key]; ok {
		return hit, nil
	}
	var found employerCandidate
	if err := database.WithWorkspaceTx(ctx, r.pool, func(tx pgx.Tx) error {
		matches, err := contacts.EmployerNameMatchesTx(ctx, tx, key)
		if err != nil {
			return err
		}
		found.count = len(matches)
		if len(matches) > 0 {
			found.id = matches[0]
		}
		return nil
	}); err != nil {
		return employerCandidate{}, fmt.Errorf("import: %w", err)
	}
	r.employers[key] = found
	return found, nil
}

// linkEmployer writes one contact→company employment edge.
//
// The `From` endpoint resolves through the run's identity map, which knows the
// contact this run just landed. The `To` endpoint is a NAME and resolves through
// resolveEmployer, exactly once per distinct name thanks to the cached index.
func (w *csvWriters) linkEmployer(ctx context.Context, a migration.Assoc) (migration.AssocResult, error) {
	contactID, missing, err := w.landedContact(ctx, a)
	if err != nil {
		return migration.AssocResult{}, err
	}
	if missing.Code != "" {
		return notLinkedFor(missing), nil
	}
	return w.resolveImportedEmployer(ctx, contactID, contacts.EmployerChoice{CompanyName: &a.ToID, Source: w.provenanceOf(a.FromID + "→" + a.ToID)})
}

// linkCompanyRef writes the edge a company_ref cell names, through the run's
// one link resolver, so it finds what the row check found.
func (w *csvWriters) linkCompanyRef(ctx context.Context, a migration.Assoc) (migration.AssocResult, error) {
	contactID, missing, err := w.landedContact(ctx, a)
	if err != nil {
		return migration.AssocResult{}, err
	}
	if missing.Code != "" {
		return notLinkedFor(missing), nil
	}
	answer, err := w.links.resolveCompany(ctx, a.ToID)
	if err != nil {
		return migration.AssocResult{}, err
	}
	if !answer.Found {
		return notLinkedFor(answer.Why), nil
	}
	companyID := ids.From[ids.CompanyKind](answer.ID)
	return w.resolveImportedEmployer(ctx, contactID, contacts.EmployerChoice{CompanyID: &companyID, Source: w.provenanceOf(a.FromID + "→" + a.ToID)})
}

// landedContact is the contact an edge starts at, or why there is none: the
// row was refused, skipped, or not yet committed.
func (w *csvWriters) landedContact(ctx context.Context, a migration.Assoc) (ids.UUID, migration.Reason, error) {
	contactID, found, err := w.lookup(ctx, migration.ObjectContact, a.FromID)
	if errors.Is(err, errBoundRecordArchived) {
		found, err = false, nil
	}
	if err != nil {
		return ids.UUID{}, migration.Reason{}, err
	}
	if !found {
		return ids.UUID{}, migration.Why(migration.ReasonLinkContactNotImported, "from", a.FromID, "name", a.ToID), nil
	}
	return contactID, migration.Reason{}, nil
}

// rowLink is what a row's employer cells say about the row: nothing, a warning
// it lands with, or a reason it is skipped.
type rowLink struct {
	skip    bool
	problem *migration.SkippedRow
	// links are the ids a deal row's link cells resolved to, by target.
	links map[string]any
}

// linkProblemOf checks a contact row's employer before the row lands, so the
// run's broken-link policy decides the row and not only its edge. The dry run
// and the commit both ask here. A company_ref is checked in every run and wins
// over a company name, which is checked only in a ZIP run.
func (w *csvWriters) linkProblemOf(ctx context.Context, row migration.Row) (rowLink, error) {
	switch w.object {
	case migration.ObjectDeal:
		return w.dealLinkProblemOf(ctx, row)
	case migration.ObjectActivity:
		return w.activityLinkProblemOf(ctx, row)
	}
	fields := textFields(row.Fields)
	if ref := strings.TrimSpace(fields[csvTargetCompanyRef]); w.companyRefColumn != "" && ref != "" {
		answer, err := w.links.resolveCompany(ctx, ref)
		if err != nil || answer.resolves() {
			return rowLink{}, err
		}
		return w.brokenLink(row, w.companyRefColumn, ref, answer.Why), nil
	}
	if name := strings.TrimSpace(fields[csvEmployerName]); w.companyNameColumn != "" && name != "" {
		resolved, err := w.resolveEmployer(ctx, name)
		if err != nil || resolved.found || resolved.why.Code == migration.ReasonEmployerNotFound {
			return rowLink{}, err
		}
		return w.brokenLink(row, w.companyNameColumn, name, resolved.why), nil
	}
	return rowLink{}, nil
}

// brokenLink is the run's policy applied to a link cell that found nothing.
func (w *csvWriters) brokenLink(row migration.Row, column, value string, why migration.Reason) rowLink {
	problem := migration.SkippedRow{
		ExternalID: row.ExternalID, Line: row.Line, Column: column,
		Kind: migration.ProblemBrokenLink, Value: value,
	}.Because(why)
	if w.onBrokenLink != string(crmcontracts.ImportOnBrokenLinkDropLink) {
		return rowLink{skip: true, problem: &problem}
	}
	problem.Kind = migration.ProblemDroppedLink
	problem = problem.Because(why.WithoutTheLink())
	return rowLink{problem: &problem}
}

// relationshipKindEmployment is the edge a company column means.
const relationshipKindEmployment = "employment"
