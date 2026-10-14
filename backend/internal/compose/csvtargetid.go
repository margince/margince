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
	"strings"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// targetID is the record a row names, and whether it named one at all.
type targetID struct {
	id    ids.UUID
	named bool
	why   migration.Reason // why a named id cannot be used, empty when it can
}

// targetIDOf reads the `id` column and resolves it against the estate.
//
// Three refusals, each a row-level reason and not an error. One bad line is
// a fix in the file, not a reason to fail the run:
//
//   - unparseable: the column holds something that is not an id at all;
//   - not found: a well-formed id no company in this workspace has;
//   - not visible: a company the caller may not read.
//
// The last two share one sentence. Two would make an oracle: they would tell a
// caller that an id they cannot read exists.
func (w *csvWriters) targetIDOf(ctx context.Context, row migration.Row) targetID {
	raw := strings.TrimSpace(textFields(row.Fields)[csvTargetID])
	if raw == "" {
		return targetID{}
	}
	if w.object != migration.ObjectCompany {
		return targetID{named: true, why: migration.Why(migration.ReasonIdOnNonCompany,
			"column", csvTargetID, "object", w.object)}
	}
	parsed, err := ids.Parse(raw)
	if err != nil {
		// A bad cell is a line to fix in the file, not a fault of the run.
		return targetID{named: true, why: migration.Why(migration.ReasonIdNotACompanyId, "value", raw)}
	}
	// GetCompany runs under the caller's own row scope, so a company they may not
	// read answers not-found, as it would anywhere else. "Gone" and "not yours"
	// share one sentence so the answer is no oracle.
	if _, err := w.contacts.GetCompany(ctx, ids.From[ids.CompanyKind](parsed),
		storekit.LiveOnly); err != nil {
		return targetID{named: true, why: migration.Why(migration.ReasonIdNotVisible, "value", raw)}
	}
	if why := w.refTakenBy(ctx, row.ExternalID, parsed); why.Code != "" {
		return targetID{named: true, why: why}
	}
	return targetID{id: parsed, named: true}
}

// refTakenBy says why a ref cannot be bound to the named company: an earlier
// import already maps it to another one. Every slice and resume must resolve
// one ref to one record. An undone run's key is free, and the bind takes it over.
func (w *csvWriters) refTakenBy(ctx context.Context, ref string, named ids.UUID) migration.Reason {
	if ref == "" {
		return migration.Reason{}
	}
	binding, found, err := w.bindingOf(ctx, ref)
	if err != nil || !found || binding.RunStatus == migration.StatusUndone || binding.NativeID == named {
		return migration.Reason{}
	}
	return migration.Why(migration.ReasonRefTakenByAnotherCompany, "ref", ref)
}
