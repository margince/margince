// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

// What a searchable entity IS, and who is allowed to see one.
//
// The branch table and the two gates every branch passes live together here
// because they are one decision read three ways: the lexical union, the vector
// lane and the plan compiler each build their own SQL from these same rows, and
// a branch declared apart from the scope rule it obeys is a branch somebody
// adds without the rule.

import (
	"context"
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// searchBranches declares one UNION branch per searchable entity: the
// scoped tables, their display title, and whether the caller's
// row-scope rides the owner predicate or the activity link walk. A new
// searchable entity is one row here — the query builder derives the
// rest.
type searchBranch struct {
	entity       string
	table        string
	title        string
	snippet      string
	activityWalk bool
	// workspaceWide marks a branch whose rows carry no owner: the tag
	// vocabulary is one word list the whole workspace shares, so a seat that
	// may read it reads all of it and there is no per-row predicate to
	// render. Asking ScopeClauseFor for one would be an error rather than an
	// empty clause, which is why the branch says so rather than passing "".
	workspaceWide bool
	// textOnly marks a branch that answers the name search and NOTHING else.
	// A tag is a word, not a record: it has no fields to plan a structured
	// query over, no neighbours to walk in the graph, and no prose to embed.
	// The three surfaces that reason about records therefore skip it, and the
	// gate that requires a contract binding per searchable entity skips it for
	// the same reason — deriving a vocabulary for a word would describe a
	// record that does not exist.
	textOnly bool
	// extraWhere narrows DISCOVERY on this branch, beyond archived_at and the
	// row scope. The by-id graph anchor read (graph.go) deliberately does not
	// apply it: a record named by id is not being discovered, and the own
	// company stays readable everywhere it is asked for by name.
	// The organization branch uses it to keep the installation's own company
	// out of results: search is how people find accounts, and the company
	// running the CRM is not one to find (ADR-0082/A127). It stays reachable
	// by id, and the company page is where it is read.
	//
	// It carries a %s for the ALIAS rather than a fixed one, because a query
	// plan's traversal reads two record types in one statement and the
	// narrowing belongs to whichever of them is being discovered. A fixed
	// alias silently narrowed the wrong table.
	extraWhere string
	// snippetFor, when set, renders the excerpt for this caller in place of
	// `snippet` — for an excerpt that reads a SECOND record and so owes that
	// record's own gate. `snippet` stays the ungated floor it falls back to.
	snippetFor func(ctx context.Context, fallback string, arg func(any) int) (string, error)
}

// projectSnippet is `key · company`, with the company named only to a caller
// who may read that organization: naming the account behind a project is a
// read of the organization row, and a searcher with no organization grant,
// or one outside the row's scope — a capture-private company another rep
// owns — gets the key alone. coalesce keeps the key when the scoped subselect
// finds no row; concat_ws skips a NULL key rather than printing the dot.
func projectSnippet(ctx context.Context, fallback string, arg func(any) int) (string, error) {
	// A denied organization grant is the key-only excerpt, not a refusal:
	// the hit is the project's, which the caller may read.
	if denied := auth.Require(ctx, "organization", principal.ActionRead); denied != nil {
		if !errors.Is(denied, apperrors.ErrPermissionDenied) {
			return "", denied
		}
		return fallback, nil
	}
	scope, err := auth.ScopeClauseFor(ctx, "organization", "o", arg)
	if err != nil {
		return "", err
	}
	if scope != "" {
		scope = " AND " + scope
	}
	return fmt.Sprintf(`coalesce((SELECT concat_ws(' · ', t.key, o.display_name) FROM organization o
			WHERE o.id = t.organization_id AND o.archived_at IS NULL%s), %s)`, scope, fallback), nil
}

// excerpt renders the branch's snippet expression for this caller.
func (b searchBranch) excerpt(ctx context.Context, arg func(any) int) (string, error) {
	if b.snippetFor == nil {
		return b.snippet, nil
	}
	return b.snippetFor(ctx, b.snippet, arg)
}

// narrowing renders this branch's discovery narrowing for one alias, and the
// empty string when the branch has none.
func (b searchBranch) narrowing(alias string) string {
	if b.extraWhere == "" {
		return ""
	}
	return fmt.Sprintf(b.extraWhere, alias)
}

// branchScope is the ONE admission + row-scope resolution every union
// branch (lexical and vector alike) runs: object RBAC hides a denied
// type silently, then the branch carries the caller's scope clause.
//
// The alias is a PARAMETER because a query plan's traversal reads two
// record types in one statement — the target as `t`, the hop as `h`. A
// clause rendered against the wrong alias filters the wrong table, and
// deciding whether a deal is visible by asking whether the caller may
// see the deal, when the question was whether they may see the
// organization behind it, is a visibility rule answering about a
// different row.
func branchScope(ctx context.Context, branch searchBranch, alias string, arg func(any) int) (scope string, admitted bool, err error) {
	if auth.Require(ctx, branch.entity, principal.ActionRead) != nil {
		return "", false, nil
	}
	switch {
	case branch.workspaceWide:
		// No row predicate at all: every seat that may read the vocabulary
		// reads all of it.
		scope = ""
	case branch.activityWalk:
		scope, err = auth.ActivityContentClause(ctx, alias, arg)
	default:
		scope, err = auth.ScopeClauseFor(ctx, branch.entity, alias, arg)
	}
	return scope, true, err
}

var searchBranches = []searchBranch{
	{entity: "person", table: "person", title: "full_name", snippet: "NULL"},
	{entity: "organization", table: "organization", title: "display_name", snippet: "NULL", extraWhere: "NOT %s.is_anchor"},
	{entity: "deal", table: "deal", title: "name", snippet: "NULL"},
	{entity: "lead", table: "lead", title: "coalesce(full_name, company_name, email)", snippet: "NULL"},
	// A project's name alone does not say which account's work it is, and two
	// accounts can run a "Phase 2". The excerpt is the key and the company,
	// which is how a person tells the hits apart; see projectSnippet for the
	// gate the company name passes first.
	{entity: "project", table: "project", title: "name", snippet: "t.key", snippetFor: projectSnippet},
	// The catalog a quote is priced from. A rep reaching for a line item knows
	// it by its name or by the sku printed on the offer in front of them, so the
	// sku is the excerpt as well as an 'A'-weighted match arm — a hit showing
	// only the name would leave two variants of one product indistinguishable
	// in the result list.
	//
	// workspaceWide because the catalog carries no owner: a price list is one
	// list the whole workspace sells from, so a seat that may read it reads all
	// of it and there is no per-row predicate to render. `active` is
	// deliberately NOT narrowed — a discontinued product still appears on last
	// quarter's offers, and a rep looking one up is usually holding one of
	// them. archived_at, which the union applies to every branch, is the
	// liveness question that does bear on discovery.
	{entity: "product", table: "product", title: "name", snippet: "t.sku", workspaceWide: true},
	// The layouts an offer is built from. `name` is the only text this table
	// holds — `layout` is jsonb, and a template's body is authored structure
	// rather than prose anybody would search for — so there is no excerpt to
	// draw and the branch says NULL rather than inventing one.
	{entity: "offer_template", table: "offer_template", title: "name", snippet: "NULL", workspaceWide: true},
	// `entity` stays a literal and `table` takes the constant, which looks
	// inconsistent and is not: TestContextAnchorEnumMatchesTheSearchableEntities
	// AST-parses the `entity` values and can only read literals, while goconst
	// counts the repeats. Splitting them satisfies both without a waiver.
	//
	// The title folds the provider in ahead of the kind: since ADR-0107/A158 a
	// message's kind is the bare word "message", so a subject-less chat would
	// render identically for every transport. coalesce falls through to the kind
	// for everything that never travelled on one.
	// A tag is a word, not a record, and it is what a person types when they
	// mean "show me the accounts we called Key Account". Finding the word is
	// the step before finding the records, and without it a reader has to know
	// the vocabulary already.
	//
	// Archived words are excluded: a retired tag is not in the picker, so a hit
	// on one leads to a page that cannot be acted on.
	{entity: "tag", table: "tag", title: "name", snippet: "NULL", workspaceWide: true, textOnly: true, extraWhere: "%s.archived_at IS NULL"},
	{entity: "activity", table: entityActivity, title: "coalesce(subject, channel_provider, kind)", snippet: "left(coalesce(body, ''), 200)", activityWalk: true},
}

// SearchedTables names every physical table the search union reads, derived
// from the branch table rather than restated beside it. The PERF-3 structural
// proof — that each of them defines a GIN index over its search_tsv column —
// asks the question of THIS list, so a branch added without an index fails the
// proof instead of quietly not being asked about. A hand-kept copy of this list
// had already fallen two branches behind, which is the failure mode a census
// cannot report: it reads a smaller set, finds nothing wrong, and passes.
func SearchedTables() []string {
	tables := make([]string, 0, len(searchBranches))
	for _, branch := range searchBranches {
		tables = append(tables, branch.table)
	}
	return tables
}
