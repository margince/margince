// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Employer is the company a contact hit was found through, as the company hit
// itself names it.
type Employer struct {
	CompanyID   ids.UUID
	CompanyName string
}

// employerArmCompanies bounds the companies whose staff the arm reads. It is
// the most company hits a grouped page can show, so every company such a page
// lists seeds its contacts, while a two-letter prefix or an industry word that
// matches hundreds of accounts cannot fan out to all their employees.
const employerArmCompanies = maxPerType

// employerArmSQL is the union element that finds contacts through a company
// the query matches, carrying that company as the hit's employer. It is not
// a searchBranches row, because it is a second way to reach the contact type.
// Every reader of that table means one row per searchable entity.
//
// Each word must match the contact's own text or the company's. So "alice
// demo" finds Alice, who works at Demo. Companies matching any word seed the arm.
// The GIN index thus picks the seeds, and the per-word test reads only staff.
//
// It is a read of the contact, of the company AND of the employment pair, so it
// takes all three gates. A refusal of any drops the arm and returns "", never an
// error: the search still answers, and discloses nothing the lists would not.
// Every refusal is decided before the first argument is bound, because a bound
// parameter no statement references fails the whole search.
func employerArmSQL(ctx context.Context, query string, headPos, tailPos int, hasFragment bool,
	identifierPos identifierBinder, arg func(any) int,
) (string, error) {
	if !auth.ReadGranted(ctx, entityContact) || !auth.ReadGranted(ctx, entityCompany) {
		return "", nil
	}
	edgeScope, err := auth.EdgeReadScope(ctx, "r", arg)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	contactBranch, contactFound := branchFor(entityContact)
	companyBranch, companyFound := branchFor(entityCompany)
	if !contactFound || !companyFound {
		return "", errors.New("search: the employer arm needs both the contact and the company branch")
	}
	contactScope, err := auth.ScopeClauseFor(ctx, entityContact, "t", arg)
	if err != nil {
		return "", err
	}
	companyScope, err := auth.ScopeClauseFor(ctx, entityCompany, "o", arg)
	if err != nil {
		return "", err
	}
	// The company and contact filters are each branch's own, so a seed is a
	// company the company branch could return and a find is a contact the
	// contact branch could: the installation's own company seeds no one.
	companyWhere := conjunction("o.archived_at IS NULL", companyBranch.narrowing("o"), companyScope)
	edgeWhere := conjunction("r.kind = 'employment'", "r.archived_at IS NULL",
		employment.IsCurrentSQL("r.ended_at"), edgeScope)
	contactWhere := conjunction("t.archived_at IS NULL", contactBranch.narrowing("t"), contactScope)
	words := queryWords(query, tailPos, hasFragment, arg)
	companyWhole := companyBranch.ownMatch("o", matchExpression(entityCompany, headPos, tailPos, hasFragment), identifierPos)
	companyScore := companyBranch.ownScore("o", scoreExpression(entityCompany, "o", headPos, tailPos, hasFragment), identifierPos)
	// Wrapped, so DISTINCT ON's ORDER BY binds to the arm and not to the union.
	// The score -1/(1+rank) lies in [-1, 0), below every own-text hit's
	// scoreExpression, which is never negative, so the keyset cursor needs no
	// second ordering. DISTINCT ON keeps one row per contact: their
	// best-matching employer, the primary job first where they hold two there.
	return fmt.Sprintf(`SELECT '%[1]s'::text AS rtype, e.id, e.title, NULL::text AS snippet, e.score,
	       e.employer_id, e.employer_name
	  FROM (
	        SELECT DISTINCT ON (t.id) t.id, %[2]s AS title,
	               (-1.0 / (1.0 + m.match_rank))::float8 AS score,
	               m.id AS employer_id, m.name AS employer_name
	          FROM (
	                SELECT o.id, %[3]s AS name, o.search_tsv, %[4]s AS whole,
	                       (%[8]s + ts_rank_cd(o.search_tsv, %[9]s, 32))::float8 AS match_rank
	                  FROM company o
	                 WHERE o.id IN (%[10]s)
	               ) m
	          JOIN relationship r ON r.company_id = m.id AND %[5]s
	          JOIN contact t ON t.id = r.contact_id AND %[6]s
	         WHERE NOT %[7]s AND (m.whole OR %[11]s)
	         ORDER BY t.id, m.match_rank DESC, r.is_current_primary DESC, m.id
	       ) e`,
		contactBranch.entity, contactBranch.title, companyBranch.title,
		companyWhole, edgeWhere, contactWhere,
		contactBranch.ownMatch("t", matchExpression(entityContact, headPos, tailPos, hasFragment), identifierPos),
		companyScore, words.any(),
		words.seedsSQL(companyWhole, companyScore, companyWhere, arg(employerArmCompanies)),
		words.everyFoundIn("t", "m")), nil
}

// typedWords holds one tsquery per word, for the employer arm to test one at a
// time. A finished word matches whole and the fragment as a prefix.
type typedWords []string

// queryWords binds each finished word of a plain-word query on its own and
// reuses the fragment's binding. Words split at whitespace, as
// websearch_to_tsquery splits a query that has no operators.
func queryWords(query string, tailPos int, hasFragment bool, arg func(any) int) typedWords {
	head, _ := splitTypedQuery(query)
	var words typedWords
	for word := range strings.FieldsSeq(head) {
		words = append(words, "("+simpleWholeWords(arg(word))+")")
	}
	if hasFragment {
		words = append(words, prefixArmSQL(tailPos))
	}
	return words
}

// employerArmSeededWords bounds the per-word seed scans, so a pasted paragraph
// costs at most this many index scans. A later word still has to match.
const employerArmSeededWords = 6

// seedsSQL selects the seed company ids: the best whole-query matches, and
// each seeded word's own best matches, every set capped at the bound limit.
// A per-word cap keeps "alice demo" seeding Demo GmbH when twenty companies
// carry "alice", as a one-word search for "demo" would.
func (w typedWords) seedsSQL(whole, wholeScore, companyWhere string, limitPos int) string {
	seeds := []string{fmt.Sprintf(`(SELECT o.id FROM company o WHERE %s AND %s ORDER BY %s DESC, o.id LIMIT $%d)`,
		whole, companyWhere, wholeScore, limitPos)}
	for _, word := range w[:min(len(w), employerArmSeededWords)] {
		seeds = append(seeds, fmt.Sprintf(
			`(SELECT o.id FROM company o WHERE o.search_tsv @@ %[1]s AND %[2]s ORDER BY ts_rank_cd(o.search_tsv, %[1]s, 32) DESC, o.id LIMIT $%[3]d)`,
			word, companyWhere, limitPos))
	}
	return strings.Join(seeds, " UNION ")
}

// any is the tsquery a company matching at least one word satisfies.
func (w typedWords) any() string {
	return "(" + strings.Join(w, " || ") + ")"
}

// everyFoundIn holds when each word is in the contact's text or the company's.
// A word that parses to no lexeme, such as a lone "&", asks nothing, as it asks
// nothing of the whole-query match.
func (w typedWords) everyFoundIn(contactAlias, companyAlias string) string {
	clauses := make([]string, 0, len(w))
	for _, word := range w {
		clauses = append(clauses, fmt.Sprintf("numnode(%[1]s) = 0 OR %[2]s.search_tsv @@ %[1]s OR %[3]s.search_tsv @@ %[1]s",
			word, contactAlias, companyAlias))
	}
	return "(" + conjunction(clauses...) + ")"
}

// conjunction ANDs the clauses that are present, each parenthesised so a
// scope's own OR cannot bind across its neighbours.
func conjunction(clauses ...string) string {
	present := make([]string, 0, len(clauses))
	for _, clause := range clauses {
		if clause != "" {
			present = append(present, "("+clause+")")
		}
	}
	if len(present) == 0 {
		return "true"
	}
	return strings.Join(present, " AND ")
}
