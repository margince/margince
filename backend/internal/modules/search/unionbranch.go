// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// unionBranch is one element of the search union.
type unionBranch struct {
	entity string
	sql    string
	// own, when set, is the same branch with only the rows that earn a text rank
	// or an identifier tier. Every other row of sql scores at most unrankedCeiling.
	own             string
	unrankedCeiling float64
	// fallbackOf names the branch whose own rows, once they fill the page, rank
	// above every row of this one: the employer arm under the contacts.
	fallbackOf string
}

type addressAssociation bool

// branchQuery is what every branch matches and scores against.
type branchQuery struct {
	headPos, tailPos int
	hasFragment      bool
	identifierPos    identifierBinder
	addressCompanies addressAssociation
}

// admittedBranchSQL builds one ranked SELECT per requested-and-admitted
// entity type. A hit is a read twice over: object RBAC first, then the row
// scope. A role without contact.read gets no contact hits, as in the lists.
func admittedBranchSQL(ctx context.Context, types []string, headPos, tailPos int, hasFragment bool,
	identifierPos identifierBinder, addressCompanies addressAssociation, arg func(any) int,
) ([]unionBranch, error) {
	q := branchQuery{headPos: headPos, tailPos: tailPos, hasFragment: hasFragment, identifierPos: identifierPos, addressCompanies: addressCompanies}
	var branches []unionBranch
	for _, branch := range searchBranches {
		if !slices.Contains(types, branch.entity) {
			continue
		}
		sql, admitted, err := branchSelect(ctx, branch, q, everyIdentifier, arg)
		if err != nil {
			return nil, err
		}
		if admitted {
			branches = append(branches, unionBranch{entity: branch.entity, sql: sql})
		}
	}
	return branches, nil
}

// withOwnMatches gives every identifier branch its own rows, for a grouped page
// of one plain word (plainWord).
func withOwnMatches(ctx context.Context, branches []unionBranch, q branchQuery, arg func(any) int) error {
	for i, union := range branches {
		branch, found := branchFor(union.entity)
		if !found || branch.identifier.Table == "" {
			continue
		}
		own, _, err := branchSelect(ctx, branch, q, rankedIdentifier, arg)
		if err != nil {
			return err
		}
		branches[i].own, branches[i].unrankedCeiling = own, branch.unrankedScoreCeiling()
	}
	return nil
}

// branchSelect renders one branch, admitting identifier rows as arm says.
// admitted is false when object RBAC refuses the type.
func branchSelect(ctx context.Context, branch searchBranch, q branchQuery, arm identifierArm, arg func(any) int) (string, bool, error) {
	scope, admitted, err := branchScope(ctx, branch, "t", arg)
	if err != nil || !admitted {
		return "", admitted, err
	}
	// What this branch matches and how it scores live in typedquery.go, with
	// each entity's parse configurations and the rule that only the fragment widens.
	tsquery := matchExpression(branch.entity, q.headPos, q.tailPos, q.hasFragment)

	snippet, err := branch.excerpt(ctx, arg)
	if err != nil {
		return "", false, err
	}
	where := branch.ownMatch("t", tsquery, q.identifierPos, arm) + " AND t.archived_at IS NULL"
	if narrowing := branch.narrowing("t"); narrowing != "" {
		where += " AND " + narrowing
	}
	if scope != "" {
		where += " AND " + scope
	}
	score := branch.ownScore("t", scoreExpression(branch.entity, "t", q.headPos, q.tailPos, q.hasFragment), q.identifierPos)
	if branch.entity == entityCompany && q.addressCompanies {
		associated := sqlFalse
		if q.identifierPos.addressPrefix {
			match, err := companyAddressMatch(ctx, "t", q.identifierPos.position(false), arg)
			if err != nil {
				return "", false, err
			}
			associated = match
		}
		candidates := fmt.Sprintf("t.id=ANY(ARRAY(SELECT lexical.id FROM company lexical WHERE lexical.search_tsv @@ %s UNION SELECT identified.id FROM company identified WHERE %s UNION SELECT t.id FROM company t WHERE %s))",
			tsquery, branch.identifierMatch("identified", q.identifierPos, arm), associated)
		where = candidates + " AND t.archived_at IS NULL AND " + conjunction(branch.narrowing("t"), scope)
		score = fmt.Sprintf("(%s + %d * (%s)::int)", score, companyAssociationScore, associated)
	}
	source := branch.rowSource(ctx, where, arg)
	return fmt.Sprintf(
		`SELECT '%s'::text AS rtype, t.id, %s AS title, %s AS snippet,
		        %s::float8 AS score, %s, %s AS identifier_match
		 FROM %s`,
		branch.entity, branch.title, snippet,
		score,
		noEmployer, branch.identifierEvidence("t", q.identifierPos), source), true, nil
}

// unionSQL joins the elements of a union for the ranked shape.
func unionSQL(branches []unionBranch) string {
	elements := make([]string, len(branches))
	for i, branch := range branches {
		elements[i] = branch.sql
	}
	return strings.Join(elements, " UNION ALL ")
}

// searchUnion builds every element one search ranks: the admitted branches,
// the employer arm, and the declared extension kinds.
func searchUnion(ctx context.Context, in Input, query string, types []string, arg func(any) int) ([]unionBranch, error) {
	headPos, tailPos, hasFragment := bindTypedQuery(query, arg)
	identifierPos := bindIdentifier(query, arg)
	q := branchQuery{headPos: headPos, tailPos: tailPos, hasFragment: hasFragment, identifierPos: identifierPos, addressCompanies: addressAssociation(in.AddressCompanies)}
	branches, err := admittedBranchSQL(ctx, types, headPos, tailPos, hasFragment, identifierPos, q.addressCompanies, arg)
	if err != nil {
		return nil, err
	}
	if in.PerType != nil && plainWord(query) {
		if err := withOwnMatches(ctx, branches, q, arg); err != nil {
			return nil, err
		}
	}
	if in.WithEmployees && slices.Contains(types, entityContact) && !carriesOperators(query) {
		arm, err := employerArmSQL(ctx, query, headPos, tailPos, hasFragment, identifierPos, arg)
		if err != nil {
			return nil, err
		}
		if arm != "" {
			branches = append(branches, unionBranch{entity: entityContact, sql: arm, fallbackOf: entityContact})
		}
	}
	extensions, err := admittedExtensionSQL(ctx, types, query, headPos, tailPos, arg)
	if err != nil {
		return nil, err
	}
	for _, extension := range extensions {
		branches = append(branches, unionBranch{sql: extension})
	}
	return branches, nil
}

// plainWord reports a query of one word made of letters and digits alone. For
// it, a row the text rank or the whole-word bonus scores is one the name index
// matches. More words, an apostrophe or an operator each break that, and a
// row found only by a contained address could then outscore an own match.
func plainWord(query string) bool {
	words := strings.Fields(query)
	return len(words) == 1 && !strings.ContainsFunc(words[0], func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsMark(r)
	})
}
