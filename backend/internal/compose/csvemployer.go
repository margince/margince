// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Linking an imported contact to the company they work for.
//
// A contact file's company column is the one mapped target that writes nothing
// to the row it sits on. It names an EDGE — this contact works at that company —
// and the two records at its ends are found in different ways: the contact by the
// key the run identifies rows with, the company by its NAME, which is the only
// thing a spreadsheet carries.
//
// Resolving a company by name is the whole risk of this file, and the rule is
// deliberately unforgiving: exactly one live visible company must carry the
// name AS WRITTEN, or no link is written. Nothing here scores, ranks or breaks a
// tie. The argument is the one csvtargetid.go makes at length for the `id`
// column — the dedupe ladder blurs on purpose, which is right when it proposes a
// review to a human and is a way to attach a contact to the wrong employer when
// it decides a write.
//
// NormalizeCompanyName is deliberately NOT the comparison, and that is the one thing
// to keep straight when reading this file. It strips legal suffixes, so `Acme
// Inc` and `Acme GmbH` normalize alike — which is exactly right for asking "should
// a human look at these two" and exactly wrong for deciding a write, because
// those are two different legal entities and picking either would be a guess. The
// comparison here folds case and spacing and nothing else.
//
// `company` has no unique index on its name (dedupecompany.go says so in as many
// words: "two companies may legitimately share a name"), so an ambiguous name
// is a real state and the honest answer is to link nothing and say which name was
// ambiguous.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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

// estateEmployer answers which company wears one folded name, cached per name,
// so the work scales with the file's names. `LIMIT 2` is the whole count: only
// a single match links. The answer is a snapshot, so a company created mid-run
// reads unresolved, as in collidesWithExisting. The worst case is a missing
// link the report names, never a link to the wrong company.
func (r *linkResolver) estateEmployer(ctx context.Context, key string) (employerCandidate, error) {
	if hit, ok := r.employers[key]; ok {
		return hit, nil
	}
	var found employerCandidate
	if err := database.WithWorkspaceTx(ctx, r.pool, func(tx pgx.Tx) error {
		// Scoped to what this caller may READ, so a company they cannot see
		// contributes nothing — neither a match nor an ambiguity.
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		keyPos := arg(key)
		scope, err := auth.ScopeClauseFor(ctx, "company", "o", arg)
		if err != nil {
			return err
		}
		// An empty clause is "every row this caller may read", not "no rows".
		visible := "TRUE"
		if scope != "" {
			visible = scope
		}
		// Both names are matched: a file may spell a company by its trading
		// name or its registered one, and the CRM may hold either. One company
		// answering on both is still ONE ROW — the OR is a predicate over the
		// row, not a join — so a company whose two names fold alike cannot make
		// itself ambiguous with itself. The whole-estate index this replaces
		// needed a per-company `seen` set to say the same thing, because it
		// counted names rather than companies.
		rows, err := tx.Query(ctx, storekit.SQLf(`
			SELECT o.id
			  FROM company o
			 WHERE o.archived_at IS NULL AND o.merged_into_id IS NULL AND %[2]s
			   AND (f_fold_import_name(o.display_name) = $%[1]d
			     OR f_fold_import_name(o.legal_name) = $%[1]d)
			 LIMIT 2`, keyPos, visible), args...)
		if err != nil {
			return fmt.Errorf("reading the companies to link against: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.CompanyID
			if err := rows.Scan(&id); err != nil {
				return fmt.Errorf("reading a company row: %w", err)
			}
			found.count++
			if found.count == 1 {
				found.id = id
			}
		}
		return rows.Err()
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
	resolved, err := w.resolveEmployer(ctx, a.ToID)
	if err != nil {
		return migration.AssocResult{}, err
	}
	if !resolved.found {
		return notLinkedFor(resolved.why), nil
	}
	return w.writeEmployment(ctx, contactID, resolved.id, w.provenanceOf(a.FromID+"→"+a.ToID))
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
	return w.writeEmployment(ctx, contactID, ids.From[ids.CompanyKind](answer.ID), w.provenanceOf(a.FromID+"→"+a.ToID))
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

// writeEmployment writes one contact→company employment edge. Any error but a
// conflict stops the run: an edge that failed for an unknown reason did not land.
func (w *csvWriters) writeEmployment(
	ctx context.Context, contactID ids.UUID, companyID ids.CompanyID, source string,
) (migration.AssocResult, error) {
	pid := ids.From[ids.ContactKind](contactID)
	if _, err := w.contacts.CreateRelationship(ctx, contacts.CreateRelationshipInput{
		Kind:      relationshipKindEmployment,
		ContactID: &pid,
		CompanyID: &companyID,
		// Stated rather than left nil: a company column on a contact row means
		// THE employer, singular, and a caller who said so must not have the
		// store's own rule decide otherwise.
		IsCurrentPrimary: ptrTrue(),
		Source:           source,
	}); err != nil {
		if errors.Is(err, apperrors.ErrConflict) {
			// The edge is unique per (contact, company), so a conflict means
			// one is already there. Reported as APPLIED because the run's promise
			// is the end state — this contact is linked to this company — and a
			// resumed run replaying its association phase must converge rather
			// than accumulate failures.
			//
			// It does NOT distinguish an edge this run wrote from one a human
			// wrote last year: both leave the estate in the state the file asked
			// for, and telling them apart would need a read the writer does not
			// take. What it must not do is claim a write that did not happen, so
			// the disclosure says linked rather than created.
			return migration.AssocResult{Applied: true}, nil
		}
		return migration.AssocResult{}, fmt.Errorf("import: linking %s to %s: %w", pid, companyID, err)
	}
	return migration.AssocResult{Applied: true}, nil
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
	if w.object == migration.ObjectDeal {
		return w.dealLinkProblemOf(ctx, row)
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
		if err != nil || resolved.found {
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

func ptrTrue() *bool { t := true; return &t }
