// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind claim H1

package gates

// Which address a contact is known by is ONE question, and it used to have
// three answers.
//
// A contact carries a LIST of addresses, each with a position and an is_primary
// flag, some archived — so naming one is a decision, and the decision is the
// order. `contacts.ReachableEmailOrder` declared it, `compose` read it, and the
// two sibling modules that ask the same question could not: a module never
// imports a sibling (ADR-0054 §3), so each spelled its own.
//
// They disagreed. activities broke ties on the row id; consent left `position`
// out altogether, so a contact who had arranged their own addresses was shown
// one on the preference centre and another on a reply, each looking right on
// its own screen. The order moved to shared/kernel/contactaddress, which is
// Tier 0 and reachable from everywhere, and this is what holds it there.
//
// WHAT IT JUDGES is a statement that picks an ADDRESS off contact_email. That
// is the question the order answers, and it is deliberately not every statement
// mentioning is_primary: capture's reply sink and signals' resolver run the
// question BACKWARDS — given an address, which contact holds it — and break
// their tie between CONTACTS. Ordering two contacts is a different decision
// that happens to read the same column, and folding them in would assert an
// answer rather than hold one.
//
// THE ARCHIVED FILTER is checked here too, and it is not in the shared constant
// on purpose. An archived address is one somebody retired: mail to it either
// bounces or reaches somebody who asked us to stop, and that is the half a
// caller must never inherit silently from a helper it did not read. So every
// caller spells it where the next reader can see it, and a statement that picks
// an address without it is a finding of its own.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	// reachableOrderHelper is the constant that IS the answer, in either of the
	// two names it is reachable under: the kernel's own, and the alias contacts
	// keeps for the four files and the compose reader that already used it.
	reachableOrderHelper = "ReachableOrder"
	reachableOrderAlias  = "ReachableEmailOrder"
	// reachableOrderOwner declares it, so its own text is the definition rather
	// than a copy of it.
	reachableOrderOwner = "internal/shared/kernel/contactaddress/contactaddress.go"
	// reachableOrderProbes is THIS file, whose planted statements are the
	// detector's own test data. Skipped because a census that reads its own
	// probes reports them as findings and can never go green — and every string
	// in it is a probe by construction, so nothing real hides behind the skip.
	reachableOrderProbes = "gates/reachableaddress_test.go"
)

var (
	// readsTheTable finds where a statement selects from contact_email. What is
	// SELECTED is then read back to the nearest preceding SELECT, in Go: RE2 has
	// no lookahead, and the select list has to be read WHOLE rather than
	// anchored on `SELECT email` — a card reads the address as one column beside
	// four others, and an anchored pattern stops seeing it the day somebody adds
	// a name to the list. Under-recognition is the one direction this must not
	// fail in.
	readsTheTable = regexp.MustCompile(`(?is)\bFROM\s+contact_email\b`)
	// aliasedArchived matches the archived filter bound to a named relation, so
	// the census can ask whether it is the ADDRESS row that was filtered.
	aliasedArchived = regexp.MustCompile(`(?is)([a-z_][a-z0-9_]*)\.archived_at\s+IS\s+NULL`)
	// bareArchived matches the filter with no relation on it, which is
	// unambiguous only when contact_email is the sole table in the statement.
	bareArchived = regexp.MustCompile(`(?is)(^|[^.\w])archived_at\s+IS\s+NULL`)
	// addressRelation finds how contact_email is named: its alias, or itself.
	addressRelation = regexp.MustCompile(`(?is)\bFROM\s+contact_email(?:\s+(?:AS\s+)?([a-z_][a-z0-9_]*))?`)
	// otherRelation finds every other table the statement reads, so a bare
	// filter can be read as the address row's only when there is no other.
	otherRelation = regexp.MustCompile(`(?is)\b(?:FROM|JOIN)\s+([a-z_][a-z0-9_]*)`)
	// picksOne is what makes a statement a DECISION rather than an enumeration.
	//
	// Erasure and the Art. 15 export read the same table and must see every row
	// including the archived ones — an export owes the subject everything held,
	// and an erasure that skipped a retired address would leave it behind. They
	// take no LIMIT, because they are not choosing. Every statement that chooses
	// ends in one, and it survives flattening while the shared constant does not
	// — so this is also what keeps a compliant caller in the corpus rather than
	// invisible to it.
	// DISTINCT ON is the other way to take one row per subject, and it decides
	// with the same ORDER BY. Left out, a picker written that way would sit
	// outside the corpus and the census would report it as clean.
	picksOne = regexp.MustCompile(`(?is)\bLIMIT\s+1\b|\bDISTINCT\s+ON\b`)
	// namesTheAddress is what makes a select list one that picks an address.
	namesTheAddress = regexp.MustCompile(`(?i)\bemail\b`)
)

func TestOneAnswerToWhichAddressAContactIsKnownBy(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	var ordered, unfiltered []string
	judged := 0
	for _, path := range handWrittenGoSources(t) {
		if at := filepath.ToSlash(path); at == reachableOrderOwner || at == reachableOrderProbes {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		scope := helperScope{
			qualifier: importAliasOf(file, "github.com/margince/margince/backend/internal/shared/kernel/contactaddress"),
			inside:    file.Name != nil && file.Name.Name == "contactaddress",
			names:     map[string]bool{reachableOrderHelper: true, reachableOrderAlias: true},
		}
		for _, decl := range file.Decls {
			for _, picker := range addressStatements(decl, scope) {
				judged++
				if !picker.usesTheOrder {
					ordered = append(ordered, fmt.Sprintf("%s: %s", path, firstAddressLine(picker.sql)))
				}
				if !filtersTheAddressRow(picker.sql) {
					unfiltered = append(unfiltered, fmt.Sprintf("%s: %s", path, firstAddressLine(picker.sql)))
				}
			}
		}
	}
	// A census that judged nothing certifies nothing, and this one has an
	// obvious way to go quiet: the table could be renamed, or the walk could
	// stop flattening concatenations, and either would report a clean tree.
	if judged < 5 {
		t.Fatalf("only %d statement(s) picking an address off contact_email were judged, so this "+
			"census covered almost nothing — the walk is broken, not the tree", judged)
	}
	if len(ordered) > 0 {
		t.Errorf("these statements pick an address without using the one answer:\n  %s\n\n"+
			"Call contactaddress.ReachableOrder. Spelling the ORDER BY by hand shows the same "+
			"contact two different addresses on two screens, each looking right on its own — and "+
			"leaving it out altogether is worse, because then the planner chooses.",
			strings.Join(ordered, "\n  "))
	}
	if len(unfiltered) > 0 {
		t.Errorf("these statements pick an address without excluding archived ones:\n  %s\n\n"+
			"An archived address is one somebody RETIRED. Mail to it either bounces or reaches "+
			"somebody who asked us to stop, and the filter is deliberately not inside the shared "+
			"constant so that every caller spells it where the next reader can see it.",
			strings.Join(unfiltered, "\n  "))
	}
}

// addressStatements returns the statements in one declaration that pick an
// address off contact_email.
//
// Flattened first, for the reason employmentcurrency_test.go gives at length: a
// statement that calls the shared constant is three AST nodes, and judging each
// alone splits the question in half — the piece naming the table no longer
// carries the ORDER BY, so the gate passes over exactly the shape every
// compliant site now has.
func addressStatements(decl ast.Decl, owner helperScope) []addressStatement {
	seen := map[ast.Node]bool{}
	var out []addressStatement
	ast.Inspect(decl, func(n ast.Node) bool {
		if n == nil || seen[n] {
			return false
		}
		text, ok := flattenSQL(n, seen, owner)
		if !ok {
			return true
		}
		usesTheOrder := mentionsTheSharedOrder(n)
		for _, statement := range sqlStatements(text) {
			if picksAnAddress(statement) {
				out = append(out, addressStatement{sql: statement, usesTheOrder: usesTheOrder})
			}
		}
		return true
	})
	return out
}

// addressStatement is one picker, and whether it reached the shared order.
//
// THE FLAG CANNOT BE READ OFF THE TEXT, which is the hole this closes. The
// constant is a Go identifier, so a statement using it carries no ORDER BY in
// the flattened SQL — and neither does a statement with no order at all, nor
// one ordering by `position` alone. Judging the text made those three
// indistinguishable, so a picker that never consulted the shared answer passed
// the census that exists to make it consult one.
type addressStatement struct {
	sql          string
	usesTheOrder bool
}

// mentionsTheSharedOrder reports whether the expression reaches the one answer,
// under either of the two names it is reachable by.
func mentionsTheSharedOrder(n ast.Node) bool {
	found := false
	ast.Inspect(n, func(node ast.Node) bool {
		if found {
			return false
		}
		switch v := node.(type) {
		case *ast.SelectorExpr:
			if v.Sel != nil && (v.Sel.Name == reachableOrderHelper || v.Sel.Name == reachableOrderAlias) {
				found = true
			}
		case *ast.Ident:
			if v.Name == reachableOrderHelper || v.Name == reachableOrderAlias {
				found = true
			}
		}
		return !found
	})
	return found
}

// picksAnAddress reports whether a statement selects an address off
// contact_email, as distinct from selecting something else from it.
//
// The distinction is the whole of this gate's scope. capture's reply sink and
// signals' resolver read the SAME table to answer the opposite question — given
// an address, which contact holds it — and break their tie between contacts.
// That is a different decision about the same column, and judging it here would
// assert an answer to it rather than hold one.
//
// And it must CHOOSE one. A statement with no LIMIT is enumerating rather than
// deciding — see picksOne.
func picksAnAddress(statement string) bool {
	for _, at := range readsTheTable.FindAllStringIndex(statement, -1) {
		opens := strings.LastIndex(strings.ToUpper(statement[:at[0]]), "SELECT")
		if opens < 0 {
			continue
		}
		if namesTheAddress.MatchString(statement[opens:at[0]]) {
			return picksOne.MatchString(statement)
		}
	}
	return false
}

// filtersTheAddressRow reports whether the statement excludes archived rows OF
// THE ADDRESS TABLE, rather than of something it happens to join.
//
// The loose form matched `archived_at IS NULL` anywhere, so
//
//	SELECT ce.email FROM contact c JOIN contact_email ce ON ce.contact_id = c.id
//	 WHERE c.archived_at IS NULL LIMIT 1
//
// read as filtered while still returning a retired address. The filter is the
// one thing deliberately left out of the shared constant, so a census that
// cannot tell which relation it binds to is not holding the half it kept.
func filtersTheAddressRow(statement string) bool {
	names := addressRelation.FindStringSubmatch(statement)
	if names == nil {
		return false
	}
	relation := names[1]
	if relation == "" {
		relation = "contact_email"
	}
	for _, at := range aliasedArchived.FindAllStringSubmatch(statement, -1) {
		if at[1] == relation {
			return true
		}
	}
	// A BARE FILTER counts only when nothing else could own it. One table in
	// the statement means `archived_at` can only be the address row's; a second
	// relation makes it a guess, and guessing in the permissive direction is
	// how a retired address reaches a mailbox.
	return bareArchived.MatchString(statement) && readsOneRelation(statement)
}

// readsOneRelation reports whether the statement reads contact_email and no
// other relation, which is the case where a bare archived filter cannot belong
// to anything else.
func readsOneRelation(statement string) bool {
	seen := map[string]bool{}
	for _, at := range otherRelation.FindAllStringSubmatch(statement, -1) {
		seen[strings.ToLower(at[1])] = true
	}
	return len(seen) == 1 && seen["contact_email"]
}

// firstAddressLine is the line a reader should open the file at.
func firstAddressLine(sql string) string {
	for _, line := range strings.Split(sql, "\n") {
		if trimmed := strings.TrimSpace(line); strings.Contains(strings.ToLower(trimmed), "contact_email") {
			return trimmed
		}
	}
	return strings.TrimSpace(sql)
}

// THE DETECTOR, asked about every shape it has to read.
//
// The census above can only report what this recognises, and it is armed at
// zero findings — so a reader that quietly stopped seeing one shape would take
// the whole gate to PASS with nothing failing. These are the distinctions it
// has to keep making, planted rather than argued.
func TestWhatCountsAsPickingAnAddress(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		sql   string
		picks bool
	}{
		{"the plain picker", `SELECT email FROM contact_email WHERE contact_id = $1 LIMIT 1`, true},
		{
			"aliased, as a scalar subquery",
			`SELECT coalesce((SELECT pe.email FROM contact_email pe WHERE pe.contact_id = p.id LIMIT 1), '')`, true,
		},
		// A card reads the address as one column beside others; a reader
		// anchored on `SELECT email` stops seeing it the day a name is added.
		{
			"the address among other columns",
			`SELECT c.full_name, pe.email FROM contact_email pe JOIN contact c ON c.id = pe.contact_id LIMIT 1`, true,
		},
		{
			"one row per subject without a LIMIT",
			`SELECT DISTINCT ON (contact_id) email FROM contact_email ORDER BY contact_id, is_primary DESC`, true,
		},
		// The question run BACKWARDS: given an address, which contact holds it.
		// Its tie is between contacts, and folding it in would assert an answer
		// to a decision nobody has made.
		{
			"resolving a contact from an address",
			`SELECT pe.contact_id FROM contact_email pe WHERE pe.email = $1 ORDER BY pe.is_primary DESC LIMIT 1`, false,
		},
		// Erasure and the Art. 15 export read every row, archived included.
		// They are not choosing, so there is no order for them to get wrong.
		{
			"enumerating every address",
			`SELECT email FROM contact_email WHERE contact_id = $1`, false,
		},
		{
			"a statement about another table",
			`SELECT phone FROM contact_phone WHERE contact_id = $1 ORDER BY is_primary DESC LIMIT 1`, false,
		},
	} {
		if got := picksAnAddress(c.sql); got != c.picks {
			t.Errorf("%s: picksAnAddress = %v, want %v", c.name, got, c.picks)
		}
	}

	// THE ARCHIVED ARM, asked both ways and about the relation it binds to. A
	// gate that only ever answered "no finding" would pass every census above
	// for the wrong reason.
	for _, c := range []struct {
		name     string
		sql      string
		filtered bool
	}{
		{
			"the address row, unaliased and alone",
			`SELECT email FROM contact_email WHERE contact_id = $1 AND archived_at IS NULL LIMIT 1`, true,
		},
		{
			"the address row, by its alias",
			`SELECT pe.email FROM contact_email pe WHERE pe.contact_id = $1 AND pe.archived_at IS NULL LIMIT 1`, true,
		},
		{
			"no filter at all",
			`SELECT email FROM contact_email WHERE contact_id = $1 LIMIT 1`, false,
		},
		// The shape the loose form got wrong: the CONTACT is filtered and the
		// address row is not, so a retired address still comes back.
		{
			"a joined table's filter, not the address row's",
			`SELECT ce.email FROM contact c JOIN contact_email ce ON ce.contact_id = c.id WHERE c.archived_at IS NULL LIMIT 1`, false,
		},
	} {
		if got := filtersTheAddressRow(c.sql); got != c.filtered {
			t.Errorf("%s: filtersTheAddressRow = %v, want %v", c.name, got, c.filtered)
		}
	}
}
