// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the importer does about a row naming a record the estate ALREADY holds.
//
// One concept, three modes, and the whole of it here: the ladder's answer
// (collision), whether that answer is certain enough to write onto (writable),
// what the preview may say about it (discloses), and what the commit will do
// (predictCollision, mirroring Ensure branch for branch).
//
// It sits apart from the writers because the question is not "how do I land a
// row" but "whose row is this, and may I touch it" — a privacy and identity
// question that happens to be answered on the write path.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/modules/migration"
	"github.com/gradionhq/margince/backend/internal/modules/people"
	"github.com/gradionhq/margince/backend/internal/platform/auth"
	"github.com/gradionhq/margince/backend/internal/platform/database"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// collidesWithExisting asks whether the CRM already holds the company this row
// names, through the SAME ladder the create path runs (PO-F-2). It reads and
// writes nothing.
//
// discloseOnly separates the two questions this answers. The PREVIEW asks it to
// tell a person something, so a match they cannot see must not be mentioned.
// The `on_duplicate: skip` path asks it to DECIDE, and there visibility is
// irrelevant — see the branch below.
//
// Only organizations: a lead's identity is its email, which the store's own
// unique key already refuses, so there is no silent twin to warn about.
//
// A read-only transaction, and NOT DedupeOrganizationForCreate — that one takes
// a write lock to serialize concurrent creates, which a preview has no business
// holding. The answer can therefore go stale between the preview and the commit.
//
// For a row that goes on to CREATE, that staleness costs nothing: the create
// path runs the locking version itself (people/creatededupe.go) and its answer
// is the one that decides.
//
// For a row that goes on to UPDATE it is a real race, stated rather than
// papered over: a company created by somebody else between this read and the
// write is not seen, so the row creates a twin instead of updating. That is the
// same outcome `on_duplicate: create` produces on purpose, it is repairable by
// merging, and it is strictly better than the alternative — taking the write
// lock here would hold it across the whole file, serializing every
// organization-name writer in the workspace behind one import.
//
// What the race can NEVER do is overwrite the wrong record: the id written to
// comes from this same read, and writable() has already required the names to
// match outright.
func (w *csvWriters) collidesWithExisting(ctx context.Context, row migration.Row, discloseOnly bool) (collision, error) {
	if w.object != migration.ObjectOrganization {
		return collision{}, nil
	}
	fields := textFields(row.Fields)
	candidate := people.OrganizationCandidate{
		DisplayName: strings.TrimSpace(fields[fieldDisplayName]),
		LegalName:   strings.TrimSpace(fields[fieldLegalName]),
	}
	if candidate.DisplayName == "" && candidate.LegalName == "" {
		return collision{}, nil
	}
	var found collision
	if err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		match, err := people.DedupeOrganization(ctx, tx, candidate)
		if err != nil || match.Decision == people.DecisionNoMatch {
			return err
		}
		found = collision{
			hit:      true,
			id:       match.OrganizationID.UUID,
			decision: match.Decision,
		}
		// EVERY candidate the ladder ranked, not only the winner. Which of them
		// the row names is the question writable() has to answer, and the winner
		// alone cannot answer it: the tie-break is the lowest uuid.
		found.ranked = match.Ranked
		found.rowDisplay, found.rowLegal = candidate.DisplayName, candidate.LegalName
		// The ladder reads every organization, by design — it is the write path's
		// collision check, and a create must not mint a twin of a row the caller
		// happens not to be allowed to see.
		//
		// A DISCLOSURE is the opposite: telling this caller "that company is
		// already here" about a row they cannot read turns the importer into an
		// oracle for a colleague's owner-private capture, which even an admin may
		// not read (rowscope.go).
		//
		// So visibility is read either way and the CALLER decides what to do with
		// it. Deciding whether to skip a row is not a disclosure; naming the
		// company in a report is, and a report is readable on the import_run
		// grant alone.
		found.visible, err = auth.VisibleTo(ctx, tx, "organization", match.OrganizationID.UUID)
		return err
	}); err != nil {
		return collision{}, fmt.Errorf("import: checking %q against the companies already held: %w", candidate.DisplayName, err)
	}
	if discloseOnly && !found.visible {
		// The caller may not know this record exists, so as far as they are
		// concerned it does not.
		return collision{}, nil
	}
	return found, nil
}

// The PREVIEW always applies the visibility filter, whatever the mode.
//
// Deciding and disclosing are two different acts, and only the second is a
// privacy question. The commit DECIDES: whether to skip a row turns on the
// incumbent existing, not on who may read it — creating a twin of a row the
// caller cannot see is exactly the duplicate a `skip` run asked to avoid, and
// skipping it reveals nothing the caller did not already put in their own file.
// That is why Ensure passes discloseOnly=false for `skip`.
//
// The preview DISCLOSES. Every mode's report counts the row in `duplicates` and
// names it with a reason saying a company of that name is already here — so a
// preview that answered on an invisible match would turn the importer into an
// existence oracle for a colleague's owner-private capture, which even an admin
// may not read (rowscope.go). A caller could probe the estate one CSV row at a
// time.
//
// An earlier draft passed the mode through to the preview, reasoning about the
// decision rather than the report. That was the oracle: `skip` previews reported
// invisible companies as duplicates.
//
// This leaves ONE case where the preview and the commit differ, and it is the
// only safe direction for them to: a row colliding with a company the caller
// cannot see previews as a create and commits as a skip (or, under `update`, as
// a create rather than an overwrite). The preview understates what the run will
// leave alone. It never promises a write the commit then declines, and it never
// confirms the existence of a record the caller may not read — which is the
// trade this makes deliberately, because the alternative discloses.

// predictCollision answers what the commit will do with a row the ladder
// matched, per mode. It mirrors Ensure branch for branch, because a preview that
// promises one outcome and a commit that produces another is the defect this
// pair of functions exists to prevent.
func (w *csvWriters) predictCollision(
	ctx context.Context, c collision, row migration.Row,
) (predictedOutcome, error) {
	switch w.onDuplicate {
	case string(crmcontracts.Skip):
		return predictCollidesSkipped, nil
	case string(crmcontracts.Update):
		if !c.writable() {
			return predictCollidesUnfit, nil
		}
		// The same comparison reconcile will make. Asking it HERE is what lets
		// the preview say "94 already here, 12 of them nothing changes" rather
		// than promising 94 writes and performing 82.
		current, err := w.read(ctx, c.writableID())
		if err != nil {
			return predictCreate, err
		}
		changed, err := changedFields(current, textFields(row.Fields))
		if err != nil {
			return predictCreate, err
		}
		if len(changed) == 0 {
			return predictCollidesUnchanged, nil
		}
		return predictCollidesUpdate, nil
	default:
		return predictCollides, nil
	}
}

// collision is the ladder's answer about one row, carried rather than collapsed
// to a boolean.
//
// The id is what `on_duplicate: update` writes onto, and the decision is what
// says whether writing is honest at all. Those two used to be discarded because
// every caller only asked "is there a duplicate" — a question that has the same
// answer for a domain match and for a name that merely looks similar, which is
// exactly the distinction an update must not lose.
type collision struct {
	hit      bool
	id       ids.UUID
	decision people.DedupeDecision
	// ranked is every candidate the ladder scored at or above its threshold,
	// best first. writable() needs the whole set rather than the winner: the
	// winner is chosen by lowest uuid among equals, which decides nothing about
	// which company a row means.
	ranked []people.OrganizationCandidateScore
	// rowDisplay and rowLegal are the names the ROW supplied. Kept because
	// OrganizationCandidateScore.MatchedField names only the stored side, so the
	// axis a pairing used cannot be read from the score alone.
	rowDisplay, rowLegal string
	// visible reports whether the caller may see the incumbent. A collision the
	// caller cannot see is returned as no collision at all, so it is neither
	// disclosed nor written to.
	visible bool
}

// writable reports whether `on_duplicate: update` may write onto this match.
//
// Two things qualify, and both are identities rather than scores:
//
//   - exact_collision — a domain match.
//   - a name the incumbent SHARES, suffix included (people.SameOrganizationName).
//
// Everything else is refused, and the interesting refusal is the near miss. The
// dedupe ladder strips the trailing legal suffix before scoring, so "Acme Inc"
// and "Acme GmbH" reach it as the same string and score 1.0. That is correct for
// the ladder — they are worth showing a human — and fuzzyOrganization says why
// they are not a merge: different legal entities are a human's call. An update
// run that wrote on that score would perform the merge the ladder deliberately
// refused to, silently and with no way back.
//
// So the suffix is compared rather than discarded. This is stricter than the
// ladder on purpose: proposing a match and overwriting a record are different
// acts, and only one of them is reversible.
//
// Two earlier drafts were wrong in opposite directions. Requiring
// exact_collision alone was safe and useless — that tier needs a domain, and a
// spreadsheet of company names carries none, so the mode refused every row it
// existed for. Accepting confidence 1.0 was useful and unsafe, for the reason
// above, and additionally because nameSimilarity caps its inputs at 256 runes:
// two different names sharing a long prefix score 1.0 as a capped score, not as
// a claim about identity.
func (c collision) writable() bool {
	if !c.hit {
		return false
	}
	if c.decision == people.DecisionExactCollision {
		return true
	}
	// Exactly ONE candidate whose name the row matches outright. Both halves
	// carry weight.
	//
	// The name axis has no unique index, and DedupeOrganizationForCreate says
	// why: two organisations may legitimately share a name. When several do,
	// fuzzyOrganization ranks them and breaks the tie on the lowest uuid — fine
	// for proposing a review pair, and no basis at all for overwriting one of
	// them. The file said "Kestrel Data"; it did not say WHICH.
	//
	// Counting matches across the whole ranked set rather than trusting
	// Ranked[0] also closes the case where a suffix variant outranks a genuine
	// exact match on uuid order: the exact one is still found, and the variant
	// is still not counted.
	var writable int
	for _, r := range c.ranked {
		if sameOnTheSameAxis(r, c.rowDisplay, c.rowLegal) {
			writable++
		}
	}
	return writable == 1
}

// writableID is the record an update run writes onto: the ONE candidate whose
// name the row matches outright, not the ladder's ranked winner.
//
// Those differ, and the difference matters. The winner is the highest score with
// the lowest uuid breaking ties, so a suffix variant scoring equally can outrank
// a genuine exact match. Writing onto Ranked[0] would then edit the wrong
// company while writable() had correctly found the right one.
//
// Only meaningful when writable() is true; it answers the collision's own id for
// a domain match, which has no ranked set.
func (c collision) writableID() ids.UUID {
	if c.decision == people.DecisionExactCollision {
		return c.id
	}
	for _, r := range c.ranked {
		if sameOnTheSameAxis(r, c.rowDisplay, c.rowLegal) {
			return r.OrganizationID.UUID
		}
	}
	return c.id
}

// sameOnTheSameAxis reports whether a ranked pairing is an outright name match
// on ONE axis — the row's display name against the incumbent's display name, or
// legal against legal.
//
// bestOrgNamePairing scores all four combinations and keeps the best, and its
// MatchedField names only the STORED side. So a row's display name matching an
// incumbent's REGISTERED name comes back as `legal_name`, indistinguishable from
// a genuine legal-to-legal hit unless the candidate side is checked too — which
// is why this takes the row's own two values rather than trusting the label.
//
// A cross-axis hit is a good reason to show a human two records. It is not a
// statement that the two names are the same name, and treating it as one lets an
// update overwrite an incumbent's legal_name from a row that only ever named a
// trading name.
func sameOnTheSameAxis(r people.OrganizationCandidateScore, rowDisplay, rowLegal string) bool {
	var rowSide string
	switch r.MatchedField {
	case fieldDisplayName:
		rowSide = rowDisplay
	case fieldLegalName:
		rowSide = rowLegal
	default:
		return false
	}
	// The ladder's candidate value must BE the row's value on that same axis,
	// and the two must be the same name.
	return people.SameOrganizationName(rowSide, r.CandidateValue) &&
		people.SameOrganizationName(r.CandidateValue, r.IncumbentValue)
}
