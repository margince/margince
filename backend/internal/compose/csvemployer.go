// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Linking an imported person to the company they work for.
//
// A contact file's company column is the one mapped target that writes nothing
// to the row it sits on. It names an EDGE — this person works at that company —
// and the two records at its ends are found in different ways: the person by the
// key the run identifies rows with, the company by its NAME, which is the only
// thing a spreadsheet carries.
//
// Resolving a company by name is the whole risk of this file, and the rule is
// deliberately unforgiving: exactly one live visible organization must normalize
// equal, or no link is written. Nothing here scores, ranks or breaks a tie. The
// argument is the one csvtargetid.go makes at length for the `id` column — the
// dedupe ladder blurs on purpose, which is right when it proposes a review to a
// human and is a way to attach a person to the wrong employer when it decides a
// write. `organization` has no unique index on its name (dedupeorg.go says so in
// as many words: "two organizations may legitimately share a name"), so an
// ambiguous name is a real state and the honest answer to it is to link nothing
// and say which name was ambiguous.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/modules/migration"
	"github.com/gradionhq/margince/backend/internal/modules/people"
	"github.com/gradionhq/margince/backend/internal/platform/auth"
	"github.com/gradionhq/margince/backend/internal/platform/database"
	"github.com/gradionhq/margince/backend/internal/platform/database/storekit"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// csvEmployerName is the column naming the company a person works for.
//
// It is not in csvTargets and never will be: that map is what a row WRITES, and
// this writes nothing to the person. It is advertised through importTargets the
// same way `id` is, and excluded from comparison the same way — see
// isNonFieldTarget, which states why both must be.
//
// Assigned FROM the source's own constant rather than spelled again, so the
// target a caller maps to and the edge the source emits are one value by
// construction — there is no second string here to fall out of step.
const csvEmployerName = migration.AssocTargetOrganizationName

// employerResolution is a name looked up, and why it produced no company when
// it did not.
type employerResolution struct {
	id     ids.OrganizationID
	found  bool
	reason string
}

// resolveEmployer answers which organization a company name means, or why it
// means none.
//
// Three outcomes, and the two failures are deliberately worded the same way as
// each other where it matters: a name matching nothing and a name matching only
// companies this caller may not see BOTH answer "no company of that name",
// because separating them would tell a caller that a company they cannot read
// exists — an existence oracle over a colleague's owner-private estate, probed
// one spreadsheet row at a time. That is the posture csvcollision.go documents.
func (w *csvWriters) resolveEmployer(ctx context.Context, name string) (employerResolution, error) {
	normalized := people.NormalizeOrgName(name)
	if normalized == "" {
		return employerResolution{reason: "the company column is empty, so the row names no employer"}, nil
	}
	index, err := w.employerIndex(ctx)
	if err != nil {
		return employerResolution{}, err
	}
	hit, ok := index[normalized]
	switch {
	case !ok:
		return employerResolution{reason: fmt.Sprintf(
			"no company named %q is in the CRM, so there was nothing to link this person to", strings.TrimSpace(name))}, nil
	case hit.count > 1:
		return employerResolution{reason: fmt.Sprintf(
			"more than one company is named %q, so which one employs this person is a question only a human can answer",
			strings.TrimSpace(name))}, nil
	default:
		return employerResolution{id: hit.id, found: true}, nil
	}
}

// employerCandidate is one normalized name's answer: the company, and how many
// companies wear that name.
//
// The count rather than the ids, because only "exactly one" links and a name
// worn by forty companies is the same answer as a name worn by two. An estate
// with a hundred thousand companies keeps a hundred thousand short strings here,
// not a hundred thousand slices.
type employerCandidate struct {
	id    ids.OrganizationID
	count int
}

// employerIndex builds the normalized-name lookup ONCE per run.
//
// Per row it would be one full scan per person — 19,000 scans for a file that
// size. The index is bounded by the number of companies in the estate, not by
// the number of rows in the file.
//
// It is a SNAPSHOT, taken when the first row needs it. A company created by
// someone else while the run is in flight is not in it, so a link that could
// have resolved is reported as unresolved instead. That is the same staleness
// collidesWithExisting already accepts and for the same reason: the failure mode
// is a missing link the report names, never a link to the wrong company.
func (w *csvWriters) employerIndex(ctx context.Context) (map[string]employerCandidate, error) {
	if w.employers != nil {
		return w.employers, nil
	}
	index := map[string]employerCandidate{}
	if err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		// Scoped to what this caller may READ, so an organization they cannot see
		// contributes nothing — neither a match nor an ambiguity.
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		scope, err := auth.ScopeClauseFor(ctx, "organization", "o", arg)
		if err != nil {
			return err
		}
		// An empty clause is "every row this caller may read", not "no rows".
		visible := "TRUE"
		if scope != "" {
			visible = scope
		}
		rows, err := tx.Query(ctx, storekit.SQLf(`
			SELECT o.id, o.display_name, o.legal_name
			  FROM organization o
			 WHERE o.archived_at IS NULL AND o.merged_into_id IS NULL AND %s`, visible), args...)
		if err != nil {
			return fmt.Errorf("reading the companies to link against: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.OrganizationID
			var display string
			var legal *string
			if err := rows.Scan(&id, &display, &legal); err != nil {
				return fmt.Errorf("reading a company row: %w", err)
			}
			// Both names are indexed: a file may spell a company by its trading
			// name or its registered one, and the CRM may hold either.
			names := []string{display}
			if legal != nil {
				names = append(names, *legal)
			}
			seen := map[string]bool{}
			for _, raw := range names {
				key := people.NormalizeOrgName(raw)
				if key == "" || seen[key] {
					// One company answering to one key once: a company whose
					// display and legal names normalize alike must not make
					// itself ambiguous with itself.
					continue
				}
				seen[key] = true
				entry := index[key]
				entry.count++
				if entry.count == 1 {
					entry.id = id
				}
				index[key] = entry
			}
		}
		return rows.Err()
	}); err != nil {
		return nil, fmt.Errorf("import: %w", err)
	}
	w.employers = index
	return index, nil
}

// linkEmployer writes one person→organization employment edge.
//
// The `From` endpoint resolves through the run's identity map, which knows the
// person this run just landed. The `To` endpoint is a NAME and resolves through
// resolveEmployer, exactly once per distinct name thanks to the cached index.
//
// A conflict is convergence, not a failure: the edge is unique per (person,
// organization), and the engine's association phase is replayed whole by a
// resumed run. Every other error still stops the run, because an edge that
// failed for an unknown reason is not an edge anybody should assume landed.
func (w *csvWriters) linkEmployer(ctx context.Context, a migration.Assoc) (migration.AssocResult, error) {
	personID, found, err := w.lookup(ctx, migration.ObjectPerson, a.FromID)
	if err != nil {
		return migration.AssocResult{}, err
	}
	if !found {
		// The row never landed — refused, skipped, or not yet committed. The
		// person is the thing missing, so that is what the report says.
		return migration.AssocResult{Reason: fmt.Sprintf(
			"the person identified by %q was not imported, so there was nobody to link to %q", a.FromID, a.ToID)}, nil
	}
	resolved, err := w.resolveEmployer(ctx, a.ToID)
	if err != nil {
		return migration.AssocResult{}, err
	}
	if !resolved.found {
		return migration.AssocResult{Reason: resolved.reason}, nil
	}

	pid := ids.From[ids.PersonKind](personID)
	oid := resolved.id
	if _, err := w.people.CreateRelationship(ctx, people.CreateRelationshipInput{
		Kind:           relationshipKindEmployment,
		PersonID:       &pid,
		OrganizationID: &oid,
		// Stated rather than left nil: a company column on a contact row means
		// THE employer, singular, and a caller who said so must not have the
		// store's own rule decide otherwise.
		IsCurrentPrimary: ptrTrue(),
		Source:           w.provenanceOf(a.FromID + "→" + a.ToID),
	}); err != nil {
		if errors.Is(err, apperrors.ErrConflict) {
			// Already linked: a resumed run replaying its association phase
			// re-offers an edge that landed on the earlier attempt.
			return migration.AssocResult{Applied: true}, nil
		}
		return migration.AssocResult{}, fmt.Errorf("import: linking %s to %q: %w", a.FromID, a.ToID, err)
	}
	return migration.AssocResult{Applied: true}, nil
}

// relationshipKindEmployment is the edge a company column means.
const relationshipKindEmployment = "employment"

func ptrTrue() *bool { t := true; return &t }
