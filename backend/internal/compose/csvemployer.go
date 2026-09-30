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
	id     ids.CompanyID
	found  bool
	reason string
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
		return employerResolution{reason: "the company column is empty, so the row names no employer"}, nil
	}
	hit, err := w.employerFor(ctx, normalized)
	if err != nil {
		return employerResolution{}, err
	}
	switch {
	case hit.count == 0:
		return employerResolution{reason: fmt.Sprintf(
			"no company named %q is in the CRM, so there was nothing to link this contact to", strings.TrimSpace(name))}, nil
	case hit.count > 1:
		return employerResolution{reason: fmt.Sprintf(
			"more than one company is named %q, so which one employs this contact is a question only a human can answer",
			strings.TrimSpace(name))}, nil
	default:
		return employerResolution{id: hit.id, found: true}, nil
	}
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
// The count is capped at two by the read that fills it, because only "exactly
// one" links and a name worn by forty companies is the same answer as one worn
// by two. Zero means no company of that name — which, deliberately, is also
// what a name worn only by companies this caller cannot see answers.
type employerCandidate struct {
	id    ids.CompanyID
	count int
}

// employerFor answers ONE folded name: which company wears it, and whether
// more than one does. Cached, so a file naming the same employer on nineteen
// thousand rows asks once.
//
// It replaces an index of the whole estate. That shape read every visible
// company into a Go map, once for the dry run and again for the commit, so a
// ten-row file against a workspace holding hundreds of thousands of companies
// loaded all of them — and a run that linked nothing paid exactly the same.
// The upload cap bounds the FILE; nothing bounded this. Now the work is
// bounded by the distinct names in the file, which is what the caller actually
// asked about.
//
// LIMIT 2 is the whole count. Only "exactly one" links, and a name worn by
// forty companies is the same answer as one worn by two — so the read stops as
// soon as it can tell those apart, and a common name never drags a page of rows
// back to be counted.
//
// The answer is a SNAPSHOT per name, taken when the first row needs it. A
// company created by someone else while the run is in flight is not in it, so a
// link that could have resolved is reported as unresolved instead. That is the
// same staleness collidesWithExisting already accepts and for the same reason:
// the failure mode is a missing link the report names, never a link to the
// wrong company.
func (w *csvWriters) employerFor(ctx context.Context, key string) (employerCandidate, error) {
	if hit, ok := w.employers[key]; ok {
		return hit, nil
	}
	var found employerCandidate
	if err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
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
	if w.employers == nil {
		w.employers = map[string]employerCandidate{}
	}
	w.employers[key] = found
	return found, nil
}

// linkEmployer writes one contact→company employment edge.
//
// The `From` endpoint resolves through the run's identity map, which knows the
// contact this run just landed. The `To` endpoint is a NAME and resolves through
// resolveEmployer, exactly once per distinct name thanks to the cached index.
//
// A conflict is convergence, not a failure: the edge is unique per (contact,
// company), and the engine's association phase is replayed whole by a
// resumed run. Every other error still stops the run, because an edge that
// failed for an unknown reason is not an edge anybody should assume landed.
func (w *csvWriters) linkEmployer(ctx context.Context, a migration.Assoc) (migration.AssocResult, error) {
	contactID, found, err := w.lookup(ctx, migration.ObjectContact, a.FromID)
	if err != nil {
		return migration.AssocResult{}, err
	}
	if !found {
		// The row never landed — refused, skipped, or not yet committed. The
		// contact is the thing missing, so that is what the report says.
		return migration.AssocResult{Reason: fmt.Sprintf(
			"the contact identified by %q was not imported, so there was nobody to link to %q", a.FromID, a.ToID)}, nil
	}
	resolved, err := w.resolveEmployer(ctx, a.ToID)
	if err != nil {
		return migration.AssocResult{}, err
	}
	if !resolved.found {
		return migration.AssocResult{Reason: resolved.reason}, nil
	}

	pid := ids.From[ids.ContactKind](contactID)
	oid := resolved.id
	if _, err := w.contacts.CreateRelationship(ctx, contacts.CreateRelationshipInput{
		Kind:      relationshipKindEmployment,
		ContactID: &pid,
		CompanyID: &oid,
		// Stated rather than left nil: a company column on a contact row means
		// THE employer, singular, and a caller who said so must not have the
		// store's own rule decide otherwise.
		IsCurrentPrimary: ptrTrue(),
		Source:           w.provenanceOf(a.FromID + "→" + a.ToID),
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
		return migration.AssocResult{}, fmt.Errorf("import: linking %s to %q: %w", a.FromID, a.ToID, err)
	}
	return migration.AssocResult{Applied: true}, nil
}

// relationshipKindEmployment is the edge a company column means.
const relationshipKindEmployment = "employment"

func ptrTrue() *bool { t := true; return &t }
