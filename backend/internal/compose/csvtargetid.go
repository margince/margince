// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A row that names the record it IS.
//
// The correction workflow: export the companies with their ids, edit the file,
// import it back. A row carrying an `id` column updates that record — no
// matching, no scoring, no tie-break.
//
// This exists because matching by NAME cannot be made safe for overwriting. The
// dedupe ladder answers "should a human look at these two?" and blurs to do it:
// it strips legal forms, so `Acme Inc` and `Acme GmbH` are one string; it scores
// a trading name against a registered one; and where several records tie it
// picks the lowest uuid. Every blur is free when the answer is "show a human"
// and is a way to destroy the wrong company when the answer decides a write.
// Three attempts at fencing those off found three ways through.
//
// An id names one record or no record, and the refusals below are the whole of
// what can go wrong with it.

import (
	"context"
	"fmt"
	"strings"

	"github.com/gradionhq/margince/backend/internal/modules/migration"
	"github.com/gradionhq/margince/backend/internal/platform/database/storekit"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// targetID is the record a row names, and whether it named one at all.
type targetID struct {
	id     ids.UUID
	named  bool
	reason string // why a named id cannot be used, "" when it can
}

// targetIDOf reads the `id` column and resolves it against the estate.
//
// Three refusals, each returned as a row-level reason rather than an error,
// because one bad line in a spreadsheet is a thing to fix in the file and not a
// reason to fail the run:
//
//   - unparseable: the column holds something that is not an id at all;
//   - not found: a well-formed id no company in this workspace has;
//   - not visible: a company the caller may not read.
//
// The last two answer the SAME sentence deliberately. Distinguishing them would
// tell a caller that an id they cannot read nonetheless exists — an oracle over
// a colleague's owner-private capture, one row at a time — and the honest thing
// to say is the thing that is true from where they stand: no company you can see
// has this id.
func (w *csvWriters) targetIDOf(ctx context.Context, row migration.Row) targetID {
	raw := strings.TrimSpace(textFields(row.Fields)[csvTargetID])
	if raw == "" {
		return targetID{}
	}
	if w.object != migration.ObjectOrganization {
		return targetID{named: true, reason: fmt.Sprintf(
			"only a company row can name an %q; a %s row is identified by its own key",
			csvTargetID, w.object)}
	}
	parsed, err := ids.Parse(raw)
	if err != nil {
		// The parse failure is the ANSWER, not a fault to propagate: one bad cell
		// in a spreadsheet is a thing to fix in the file, and failing the whole
		// run over it would tell a person nothing about which line to look at.
		return targetID{named: true, reason: fmt.Sprintf(
			"%q is not a company id; export the companies to get theirs, or leave the column empty "+
				"to import this row as a new company", raw)}
	}
	// GetOrganization runs under the caller's own row scope, so a company they
	// may not read answers not-found here exactly as it would anywhere else.
	// That is what makes one sentence cover both cases honestly.
	// Likewise: a read that does not answer means no company this caller can see
	// has that id, which is the sentence the report shows. Distinguishing "gone"
	// from "not yours" would tell a caller that an id they cannot read exists.
	if _, err := w.people.GetOrganization(ctx, ids.From[ids.OrganizationKind](parsed),
		storekit.LiveOnly); err != nil {
		return targetID{named: true, reason: fmt.Sprintf(
			"no company you can see has the id %q; it may have been archived, merged away, or belong "+
				"to a workspace this file did not come from", raw)}
	}
	return targetID{id: parsed, named: true}
}
