// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commissions

// Commission row visibility, which is INHERITED rather than owned.
//
// The shared row-scope helpers interpolate the tables that answer `owner_id`
// and reject a table name they do not know rather than guessing, so
// commission_entry cannot join them: there is no column for their clauses to
// test.
//
// An entry is visible when the deal it was accrued on is visible. Deriving it
// rather than copying an owner at accrual is what makes a deal reassignment
// carry its commission entries in the same query, instead of leaving them
// behind with the representative who no longer works the account.
//
// There is only one anchor. Unlike a contract, an entry cannot exist without a
// deal — deal_id is NOT NULL — so there is no organization fallback arm.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// unboundedScope is the predicate an unbounded caller contributes: they narrow
// nothing, and the arm still has to say something.
const unboundedScope = "TRUE"

// VisibleClause renders the SQL predicate admitting the commission entries this
// caller may see, for a row under the given alias. It returns the empty string
// for an unbounded caller, exactly as the shared row-scope helpers do.
//
// The anchor must be LIVE. An archived deal keeps its foreign key and its
// grants, so without the filter an entry would stay readable — and payable —
// through a deal whose own read already answers 404.
func VisibleClause(ctx context.Context, alias string, arg func(any) int) (string, error) {
	return entriesOfVisibleDeals(ctx, alias, arg, liveAnchor)
}

// RetractableClause is VisibleClause with the anchor-liveness arm dropped, and
// nothing else changed: the same row scope, so a caller still reaches only
// entries on deals they may see.
//
// It exists because that liveness is load-bearing for serving and paying an
// entry and wrong for taking one back. An accrual on a deal that has since been
// archived is exactly the one somebody needs to void, and a read that refuses it
// makes the write gate moot — the void answers not-found before it ever reaches
// a probe. Only the void path composes this; the list and the single read do
// not, so an archived deal's entries stay out of every surface that pays.
func RetractableClause(ctx context.Context, alias string, arg func(any) int) (string, error) {
	return entriesOfVisibleDeals(ctx, alias, arg, anyAnchor)
}

// The two anchor arms the pair above differs by, so the difference is a value
// rather than a second copy of the statement.
const (
	liveAnchor = "d.archived_at IS NULL AND "
	anyAnchor  = ""
)

// entriesOfVisibleDeals renders both clauses above: the entries whose deal this
// caller may see, narrowed by whatever the anchor arm requires of that deal.
func entriesOfVisibleDeals(ctx context.Context, alias string, arg func(any) int, anchor string) (string, error) {
	// The ROW scope below narrows which deals admit their entries; it does not
	// ask whether this caller may read a deal at all. Both are needed: an entry
	// names its deal and prices it, so a caller holding commission:read without
	// deal:read would learn what a deal was worth through the ledger — the
	// object grant the deal's own read would have refused.
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return "", err
	}
	dealScope, err := auth.ScopeClauseFor(ctx, "deal", "d", arg)
	if err != nil {
		return "", err
	}

	qualified := alias
	if qualified != "" {
		qualified += "."
	}
	// An unbounded caller still gets whatever the anchor arm requires: the
	// archived-anchor rule is about what the row means, not about who is asking.
	scope := dealScope
	if scope == "" {
		scope = unboundedScope
	}
	return storekit.SQLf(`EXISTS (
		SELECT 1 FROM deal d WHERE d.id = %[1]sdeal_id AND %[2]s%[3]s)`,
		qualified, anchor, scope), nil
}

// WritableEntriesForDeal narrows a CHANGE to the entries of one deal the caller
// may write, not merely see.
//
// Separate from VisibleClause because a manual share widens VISIBILITY at
// either access level: a caller holding only a `read` share of the deal passes
// the read clause, and moving their partner's money is not something a read
// share should buy. The write path asks a row probe instead, which is what
// distinguishes the two.
//
// LIVE, because what this gates COMMITS money: approving an accrual and paying
// it are claims on a deal, and an archived deal admits no new ones.
func WritableEntriesForDeal(ctx context.Context, tx pgx.Tx, deal ids.DealID) error {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return err
	}
	return auth.EnsureWritableLive(ctx, tx, "deal", deal.UUID)
}

// RetractableEntriesForDeal is its twin for taking money BACK — a void and the
// reversal a reopen sweeps through. Same authority, no liveness: a partner's
// accrual on a deal that has since been archived is exactly the one somebody
// needs to reverse, and auth.EnsureRetractable states why refusing it would
// protect nobody.
func RetractableEntriesForDeal(ctx context.Context, tx pgx.Tx, deal ids.DealID) error {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return err
	}
	return auth.EnsureRetractable(ctx, tx, "deal", deal.UUID)
}
