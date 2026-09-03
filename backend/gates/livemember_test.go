// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind claim H1

package gates

// "Someone who still works here" is `status = 'active' AND archived_at IS
// NULL` on app_user, and TWO functions in two different packages each called
// themselves the ONE spelling of it while the tree held about twenty copies.
//
// org360's said so; search's said so as well, and its own comment recorded
// that org360 had already spelled it that way — so the second author knew
// about the first copy and minted a third anyway. Neither had anything holding
// it. That is the false-uniqueness pair CLAUDE.md names: the next author greps,
// finds a comment claiming the question is settled, and stops looking.
//
// identity owns app_user, so identity.LiveMemberSQL is the definition. This
// gate judges STATEMENTS that read app_user, in two arms:
//
//   - A statement constraining app_user's liveness must not name BOTH halves
//     itself — it calls the helper, or it is ratified as unable to.
//   - A statement naming ONE half without the other is the half-spelling, and
//     it is the actual regression: deactivation sets status and leaves
//     archived_at NULL, so `archived_at IS NULL` alone goes on offering a
//     departed colleague. It found one live defect — a LinkedIn reach count
//     that offered a deactivated colleague as a warm intro.
//
// What it deliberately does NOT judge: a statement that reads app_user with no
// liveness constraint at all. Plenty legitimately do — resolving a row by id to
// render a name does not care whether the person still works here, and a gate
// that demanded they all filter would be asserting an answer nobody gave. It
// also does not judge another table that happens to carry the same two column
// names: `voice_profile_version` is filtered by `status = 'active' AND
// archived_at IS NULL` in two places and is not a workforce question at all.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	liveMemberHelper = "LiveMemberSQL"
	// activatableMemberHelper is the SECOND definition livemember.go owns, and it
	// is registered here for the same reason the first is: an unregistered helper
	// call flattens to its ARGUMENTS rather than to a marker, so the statement
	// around it renders with no halves at all and passes unjudged. Under-recognition
	// is the one way this census must not break — it would read a smaller tree,
	// report PASS, and leave no failing assertion to notice.
	activatableMemberHelper = "ActivatableMemberSQL"
	liveMemberOwner         = "internal/modules/identity/livemember.go"
	identityPath            = "github.com/margince/margince/backend/internal/modules/identity"
)

// cannotReachIdentity ratifies the statements that spell the predicate out
// because the module DAG forbids them the helper.
//
// identity owns app_user and a module never imports a sibling (ADR-0054 §3).
// compose may reach identity and now does — org360 and the extension-job seam
// both call the helper — but seven statements sit in sibling modules, and the
// predicate would have to move tier before they could adopt it. That is an
// architecture decision with an owner rather than something to smuggle in here.
//
// Each entry is a FILE and not a package, which is narrower but NOT narrow
// enough to say what an earlier version of this comment said: a new
// hand-spelled copy added to one of these files is excused along with the ones
// already there. The key ratifies the file. Splitting it finer would mean
// keying on statement text, which changes with every whitespace edit and
// ratifies nothing stable.
var cannotReachIdentity = gatekit.Waive(map[string]string{
	"internal/modules/activities/audience.go":        "activities cannot import identity (ADR-0054 §3); the predicate must move tier first",
	"internal/modules/activities/lifecycle.go":       "activities cannot import identity (ADR-0054 §3); the predicate must move tier first",
	"internal/modules/dealrooms/store_public.go":     "dealrooms cannot import identity (ADR-0054 §3); the predicate must move tier first",
	"internal/modules/capture/owneridentitystore.go": "capture cannot import identity (ADR-0054 §3); the predicate must move tier first",
	"internal/modules/people/counterpartyname.go":    "people cannot import identity (ADR-0054 §3); the predicate must move tier first",
	"internal/modules/people/leadrouting.go":         "people cannot import identity (ADR-0054 §3); the predicate must move tier first",
	"internal/modules/people/linkedinmatch.go":       "people cannot import identity (ADR-0054 §3); the predicate must move tier first",
	"internal/modules/projects/surface.go":           "projects cannot import identity (ADR-0054 §3); the predicate must move tier first",
	"internal/modules/projects/transfer.go":          "projects cannot import identity (ADR-0054 §3); the predicate must move tier first",
	"internal/modules/search/graphedge.go":           "search cannot import identity (ADR-0054 §3); the predicate must move tier first",
})

// deliberatelyNotLiveness ratifies the half-spellings that are not answering
// the workforce question at all.
//
// overlay's user-map eligibility USED to be `NOT is_agent AND archived_at IS
// NULL` and is no longer: #2592 tightened it to the same pair this file names,
// which overlay's own mappableSeatSQL now renders because a module cannot import
// identity (ADR-0054 §3). Its two files are ratified below as spellings rather
// than as divergences.
//
// The reason it was a divergence and stopped being one is worth keeping: a
// DEACTIVATED seat is not archived, so all three overlay sites went on offering
// it a mapping — while listUserMapSQL's own comment justified excluding archived
// users as "a seat that no longer logs in", which is exactly as true of a
// deactivated one. The predicate disagreed with its own stated reason, which is
// what made it a defect rather than a preference.
var deliberatelyNotLiveness = gatekit.Waive(map[string]string{
	"internal/modules/identity/roster.go":                                      "listUsersAllQuery is the ADMIN roster and says so: every non-archived member REGARDLESS of status, because a deactivated member has to be visible to reactivate",
	"internal/modules/identity/reset.go":                                       "OperatorResetPassword is the operator CLI's lockout path, never exposed over HTTP; it must reach an account whose status is not active, which is what administrator lockout means. Login itself calls the helper (lockout.go)",
	"internal/compose/integration/agentaccess/oauth_grant_integration_test.go": "the fixture deactivates every seat that CAN still act, to prove revocation binds mid-session; the archived half would narrow nothing, because every seat in it was created by the harness moments earlier and none is archived",
})

// namesTheSeatRatherThanOffersIt ratifies the reads that resolve a seat by the
// id their caller was handed and deliberately admit a deactivated one.
//
// These are NOT the offer defect. A departed colleague's name still has to
// render on the work they did, their locale still decides how a stored string
// is formatted, and an admin still has to be able to inspect and change a
// deactivated seat — that is how somebody comes back. Requiring liveness here
// would blank a name on last quarter's activity, which is a worse answer than
// the one this gate exists to prevent.
//
// One entry is a defect rather than a decision, and it says so: dealrooms opens
// a room with a steward who cannot act. That changes who may open a room and
// with what, so it is issue margince/margince#2596 rather than a change made
// here on a guess — and it stays listed so it reads as open rather than settled.
var namesTheSeatRatherThanOffersIt = gatekit.Waive(map[string]string{
	"internal/modules/dealrooms/preview.go":      "renders a steward's name and address on a room that already exists; a departed colleague's name is still the right label on what they did",
	"internal/modules/dealrooms/room_write.go":   "DEFECT, not a decision: a deactivated seat can be a new room's steward. Product call on who may be one, so it is issue 2596",
	"internal/modules/identity/access.go":        "UserAccess evaluates an EXISTING member's roles and teams as they stand, which an admin needs precisely when the seat is deactivated",
	"internal/modules/identity/actoridentity.go": "resolves the display name and address of whoever performed a past action; the actor of an audit row does not stop having a name",
	"internal/modules/identity/seatnames.go":     "answers \"what is this id called\" for ids the caller already holds; a name that blanks on deactivation makes historical rows unreadable",
	"internal/modules/identity/userlocale.go":    "reads a seat's locale to format a stored string; the formatting of last month's number does not depend on whether they still work here",
	"internal/modules/identity/users.go":         "ChangeUserRole reads what the target IS because an agent seat holds no role; changing a deactivated member's role is how an admin prepares a reactivation",
})

// appUserAlias finds what app_user is called in a statement, so a sibling
// table's archived_at is not read as app_user's.
//
// `FROM project p JOIN app_user u` puts two archived_at columns in scope and
// only one of them is a workforce question. An unaliased `FROM app_user` binds
// the bare column names instead.
var appUserAlias = regexp.MustCompile(`(?i)\bapp_user\b(?:\s+AS)?\s+([a-z]\w*)`)

// readsAppUser matches the table itself and not a column named after it: the
// trailing `\b` already refuses `app_user_id`, since an underscore is a word
// character. A statement that only carries the foreign key is not reading the
// row, and asking it about liveness would report a pairing nobody wrote.
//
// CASE-INSENSITIVE, like every other pattern here. Postgres folds an unquoted
// identifier, so `FROM APP_USER` is the same relation and a case-sensitive
// census would have passed straight over it — the same way `as` slipped past
// an uppercase-only `AS`. Nothing in this tree writes it that way today, which
// is exactly why nobody would notice the day something did.
var readsAppUser = regexp.MustCompile(`(?i)\bapp_user\b`)

func TestOnlyOneSpellingOfALiveMember(t *testing.T) {
	t.Parallel()
	// A ratification that stops matching covers a site that has moved or been
	// fixed, and leaving it in place quietly re-exempts whatever takes its name.
	defer cannotReachIdentity.AssertAllMatched(t)
	defer deliberatelyNotLiveness.AssertAllMatched(t)
	defer namesTheSeatRatherThanOffersIt.AssertAllMatched(t)

	fset := token.NewFileSet()
	var copies, activatableCopies, halves []string
	judged, constrained := 0, 0
	for _, path := range handWrittenGoSources(t) {
		slash := filepath.ToSlash(path)
		// The definition itself, and this file, which plants deliberate defects
		// below as the detector's own evidence.
		if slash == liveMemberOwner || filepath.Base(path) == "livemember_test.go" {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		scope := helperScope{
			qualifier: importAliasOf(file, identityPath),
			inside:    file.Name != nil && file.Name.Name == "identity",
			names:     map[string]bool{liveMemberHelper: true, activatableMemberHelper: true},
		}
		for _, decl := range file.Decls {
			for _, sql := range appUserStatements(decl, scope) {
				judged++
				if st, ar := liveMemberHalves(sql); st || ar {
					constrained++
				}
				switch liveMemberVerdict(sql) {
				case "copy":
					// No exemption for "it also calls the helper". The helper
					// renders as a marker and contributes NO literal halves, so
					// a compliant statement never reaches here — and excusing
					// one that does would make calling the definition a licence
					// to write a second predicate beside it.
					if cannotReachIdentity.Waived(t, slash) {
						continue
					}
					copies = append(copies, fmt.Sprintf("%s: %s", path, firstLiveMemberLine(sql)))
				case "activatable-copy":
					if cannotReachIdentity.Waived(t, slash) {
						continue
					}
					activatableCopies = append(activatableCopies, fmt.Sprintf("%s: %s", path, firstLiveMemberLine(sql)))
				case "half":
					if deliberatelyNotLiveness.Waived(t, slash) ||
						namesTheSeatRatherThanOffersIt.Waived(t, slash) {
						continue
					}
					halves = append(halves, fmt.Sprintf("%s: %s", path, firstLiveMemberLine(sql)))
				}
			}
		}
	}
	// A census that judged nothing certifies nothing, and the two floors are
	// separate on purpose: the walk can find app_user statements while the
	// half-detection is broken and reports every one of them as unconstrained.
	// Both floors sit far below the real counts so they catch a broken walk
	// rather than a changing tree.
	if judged < 40 {
		t.Fatalf("only %d app_user statement(s) were judged, so this census covered almost nothing", judged)
	}
	if constrained < 10 {
		t.Fatalf("only %d of %d app_user statements were seen to constrain liveness at all, so the halves are not being read", constrained, judged)
	}
	if len(copies) > 0 {
		t.Errorf("these statements spell \"someone who still works here\" out themselves:\n  %s\n\n"+
			"identity.%s is the definition and identity owns app_user. compose may call it; a sibling "+
			"module may not (ADR-0054 §3) and is ratified by name in this gate with that reason.",
			strings.Join(copies, "\n  "), liveMemberHelper)
	}
	if len(activatableCopies) > 0 {
		t.Errorf("these statements spell \"may still become active\" out themselves:\n  %s\n\n"+
			"identity.%s is the definition. Do NOT answer this by adding status = 'active': that is "+
			"the set which EXCLUDES an invited member, and it would refuse an invitation its own link "+
			"was minted to redeem.",
			strings.Join(activatableCopies, "\n  "), activatableMemberHelper)
	}
	if len(halves) > 0 {
		t.Errorf("these statements constrain app_user by ONE half of liveness:\n  %s\n\n"+
			"Deactivating an account sets status and leaves archived_at NULL, so archived_at alone goes "+
			"on offering a departed colleague — and status alone keeps an archived one. Name both halves, "+
			"or ratify the statement here saying which set it means instead.",
			strings.Join(halves, "\n  "))
	}
}

// TestEveryLiveMemberAliasIsALiteral holds the one thing LiveMemberSQL cannot
// hold itself: its argument is formatted into the statement as an identifier,
// not bound as a parameter, so a caller that passed request input would inject.
// Every call site names a table alias it wrote itself, and this says so.
func TestEveryLiveMemberAliasIsALiteral(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	var findings []string
	calls := 0
	for _, path := range handWrittenGoSources(t) {
		// A test drives the function with its own table, which is the point of
		// a table: livemember_test.go passes tc.alias and injects into nothing,
		// because the string it builds is compared rather than executed.
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		// Callees, plus the declaration itself: `func LiveMemberSQL(...)` names
		// the helper without using it, and reporting the definition as its own
		// unsafe caller would be the gate refusing the thing it protects.
		callees := map[ast.Node]bool{}
		ast.Inspect(file, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.CallExpr:
				callees[v.Fun] = true
				// A qualified callee is a SelectorExpr whose Sel is the
				// helper's own name, and the walk reaches that Ident
				// separately. Marking only the selector reported every
				// ordinary `identity.LiveMemberSQL("u")` as a value reference.
				if sel, ok := v.Fun.(*ast.SelectorExpr); ok {
					callees[sel.Sel] = true
				}
			case *ast.FuncDecl:
				callees[v.Name] = true
			}
			return true
		})
		ast.Inspect(file, func(n ast.Node) bool {
			if namesTheHelper(n) && !callees[n] {
				findings = append(findings, fmt.Sprintf("%s:%d (used as a value, so no argument is checkable)",
					path, fset.Position(n.Pos()).Line))
				return true
			}
			call, ok := n.(*ast.CallExpr)
			if !ok || calleeName(call) != liveMemberHelper || len(call.Args) != 1 {
				return true
			}
			calls++
			if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				return true
			}
			findings = append(findings, fmt.Sprintf("%s:%d", path, fset.Position(call.Pos()).Line))
			return true
		})
	}
	// The floor catches a walk that stopped finding calls, which would make
	// this pass over a tree full of them.
	if calls < 10 {
		t.Fatalf("only %d call(s) to %s were seen, so this proved almost nothing", calls, liveMemberHelper)
	}
	if len(findings) > 0 {
		t.Errorf("these calls to %s pass an alias that is not a literal:\n  %s\n\n"+
			"the alias is formatted into the SQL as an identifier, so anything off a request injects there.",
			liveMemberHelper, strings.Join(findings, "\n  "))
	}
}

// namesTheHelper reports whether this node IS a reference to the helper — bare
// inside identity, qualified anywhere else.
func namesTheHelper(n ast.Node) bool {
	switch v := n.(type) {
	case *ast.SelectorExpr:
		return v.Sel != nil && v.Sel.Name == liveMemberHelper
	case *ast.Ident:
		return v.Name == liveMemberHelper
	}
	return false
}

// liveMemberVerdict is what this gate says about one statement: "copy", "half",
// or "" for one it passes over.
//
// The census and the probe suite BOTH call it, and that is the point. They used
// to spell the rule separately, so a probe could agree with a census that had
// changed underneath it — the two readings of one value this repo has been
// bitten by before. A probe now exercises the code the tree is judged with, or
// it exercises nothing.
func liveMemberVerdict(sql string) string {
	// A bare fragment IS the pair — onlyTheLivenessPair matches nothing else —
	// so it is a copy without needing a table to attribute its columns to.
	// Asking liveMemberHalves instead would refuse it, because that binds every
	// column to app_user's alias and a fragment names no table to take one from.
	if bareLivenessPredicate(sql) {
		return "copy"
	}
	if bareActivatablePredicate(sql) {
		return "activatable-copy"
	}
	// Judged BEFORE the halves, because the two questions overlap and the wrong
	// answer here is actively harmful. A hand-spelled activatable pair carries
	// `archived_at IS NULL` without `status = 'active'`, so the halves below
	// would report it as a half-spelling — and that message tells the next
	// author to add `status = 'active'`, which is precisely the edit that breaks
	// invitation redemption. It is not a half of anything; it is a second copy
	// of ActivatableMemberSQL, and the report has to say so.
	if activatableCopy(sql) {
		return "activatable-copy"
	}
	status, archived := liveMemberHalves(sql)
	switch {
	case status && archived:
		return "copy"
	case (status || archived) && offersAUser(sql):
		return "half"
	}
	return ""
}

// activatableCopy reports a hand-written second spelling of ActivatableMemberSQL:
// the invited-or-active status test paired with the archived half, on app_user's
// own columns.
//
// It matches the exact predicate ActivatableMemberSQL emits and not an
// arbitrary `status IN (…)` list, so a predicate over some other pair of
// statuses is still reported by the halves below rather than quietly absorbed
// here. A census that widens to tolerate the shape of a defect stops being able
// to see it.
func activatableCopy(sql string) bool {
	sql = stripAssignments(sql)
	prefix := appUserPrefix(sql)
	invitedOrActive := appUserColumn(prefix, `status\s+IN\s+\(\s*'invited'\s*,\s*'active'\s*\)`).MatchString(sql) ||
		appUserColumn(prefix, `status\s+IN\s+\(\s*'active'\s*,\s*'invited'\s*\)`).MatchString(sql)
	archived := appUserColumn(prefix, `archived_at\s+IS\s+NULL`).MatchString(sql)
	return invitedOrActive && archived && offersAUser(sql)
}

// bareLivenessPredicate matches the shape that started all of this: the pair
// spelled into a declaration of its own and consumed elsewhere by name.
//
// `const liveMemberWhere = "u.status = 'active' AND u.archived_at IS NULL"` is
// invisible to a table-keyed walk from both ends — the declaration never
// mentions app_user, and the statement that concatenates it renders the
// identifier as a blank. That is precisely how org360's copy sat unheld while
// its comment told the next reader the question was already settled.
//
// It deliberately does not catch a HALF spelled into a bare declaration: a lone
// `archived_at IS NULL` fragment could belong to any of a dozen tables, and
// reporting it would be a guess.
func bareLivenessPredicate(sql string) bool {
	return onlyTheLivenessPair.MatchString(strings.TrimSpace(sql))
}

// bareActivatablePredicate is bareLivenessPredicate for the SECOND definition,
// and it exists because the defect this whole gate was written for reproduces
// exactly once per predicate: `const activatableWhere = "status IN ('invited',
// 'active') AND archived_at IS NULL"` names no table, so a table-keyed walk
// never yields it and the copy sits unheld while its comment tells the next
// reader the question is settled. That is the org360 incident, verbatim, for a
// predicate added after it.
func bareActivatablePredicate(sql string) bool {
	return onlyTheActivatablePair.MatchString(strings.TrimSpace(sql))
}

// onlyTheActivatablePair matches a literal that is the activatable predicate and
// NOTHING else, in either status order, with or without an alias — the same
// "nothing else" discipline onlyTheLivenessPair keeps, and for the same reason:
// prose that quotes the predicate back to a reader is not a second
// implementation of it.
var onlyTheActivatablePair = regexp.MustCompile(`(?is)^\(?\s*(?:` +
	`(?:[a-z]\w*\.)?status\s+IN\s+\(\s*'(?:invited|active)'\s*,\s*'(?:invited|active)'\s*\)\s+AND\s+(?:[a-z]\w*\.)?archived_at\s+IS\s+NULL` + `|` +
	`(?:[a-z]\w*\.)?archived_at\s+IS\s+NULL\s+AND\s+(?:[a-z]\w*\.)?status\s+IN\s+\(\s*'(?:invited|active)'\s*,\s*'(?:invited|active)'\s*\)` +
	`)\s*\)?$`)

// onlyTheLivenessPair matches a literal that is the predicate and NOTHING else,
// in either order, with or without an alias.
//
// "Nothing else" is doing real work rather than being strict for its own sake.
// A looser reading — "spells both halves and names no table" — reported a test
// whose FAILURE MESSAGE quotes the predicate back to the reader. Prose that
// mentions the pair is not a second implementation of it, and a gate that says
// so teaches people to stop reading its output.
var onlyTheLivenessPair = regexp.MustCompile(`(?is)^\(?\s*(?:` +
	`(?:[a-z]\w*\.)?status\s*=\s*'active'\s+AND\s+(?:[a-z]\w*\.)?archived_at\s+IS\s+NULL` + `|` +
	`(?:[a-z]\w*\.)?archived_at\s+IS\s+NULL\s+AND\s+(?:[a-z]\w*\.)?status\s*=\s*'active'` +
	`)\s*\)?$`)

// appUserStatements returns the SQL statements in a declaration that read
// app_user, flattened so a statement assembled out of a literal and a helper
// call is judged as the one statement it builds.
//
// Per DECLARATION and not per file, so a file holding one app_user query and
// one unrelated archived_at test does not report a pairing nobody wrote.
func appUserStatements(decl ast.Decl, owner helperScope) []string {
	var out []string
	seen := map[ast.Node]bool{}
	ast.Inspect(decl, func(n ast.Node) bool {
		if seen[n] {
			return false
		}
		text, ok := flattenSQL(n, seen, owner)
		if !ok || (!readsAppUser.MatchString(text) && !bareLivenessPredicate(text) && !bareActivatablePredicate(text)) {
			return true
		}
		out = append(out, text)
		return true
	})
	return out
}

// liveMemberHalves reports which halves of liveness a statement constrains, on
// APP_USER's columns — resolved through the alias so a joined table's
// archived_at is somebody else's question.
func liveMemberHalves(sql string) (status, archived bool) {
	sql = stripAssignments(sql)
	prefix := appUserPrefix(sql)
	status = appUserColumn(prefix, `status\s*=\s*'active'`).MatchString(sql)
	archived = appUserColumn(prefix, `archived_at\s+IS\s+NULL`).MatchString(sql)
	return status, archived
}

// stripAssignments drops an UPDATE's SET clause, because `SET status =
// 'active'` MAKES someone live rather than asking whether they are. Reading it
// as a predicate reported every fixture that activates a seat, which is the
// opposite of the statement this gate is looking for.
//
// From SET to the first WHERE or RETURNING at the same depth, so a predicate in
// the same statement is still judged and a subquery inside an assignment does
// not end the clause early.
func stripAssignments(sql string) string {
	set := regexp.MustCompile(`(?is)\bSET\b`).FindStringIndex(sql)
	if set == nil {
		return sql
	}
	depth := 0
	for i := set[1]; i < len(sql); i++ {
		switch sql[i] {
		case '(':
			depth++
		case ')':
			depth--
		}
		if depth != 0 {
			continue
		}
		if end := endsAssignments.FindStringIndex(sql[i:]); end != nil && end[0] == 0 {
			return sql[:set[0]] + " " + sql[i:]
		}
	}
	return sql[:set[0]]
}

// endsAssignments is what closes a SET clause: the predicate, or the row the
// statement hands back.
var endsAssignments = regexp.MustCompile(`(?is)^\s(WHERE|RETURNING|FROM)\b`)

// offersAUser reports whether the statement is one that could hand back the
// wrong colleague — which is the only shape the half-spelling defect takes.
//
// A WRITE is judged like a read. Excluding one on the grounds that "an UPDATE
// offers nobody" was wrong in the direction that matters: `UPDATE app_user SET
// password_hash = $2 WHERE id = $1 AND archived_at IS NULL` sets a DEACTIVATED
// account's password, and the row set a mutation chooses is exactly as much a
// liveness question as the one a read returns. Only its SET clause is exempt,
// because that assigns rather than asks (stripAssignments).
func offersAUser(sql string) bool {
	return !resolvesOneUserByID(sql)
}

// resolvesOneUserByID reports whether the statement already names WHICH user it
// wants, by the primary key its caller was handed.
//
// Such a statement cannot offer the wrong colleague — it is reading a row
// somebody else chose, and `archived_at IS NULL` alone is the right filter
// there: an archived seat is gone, a deactivated one is an account that still
// has a locale, a display name and a team to render on the work it did. About
// twenty statements in identity have exactly this shape, and reporting them
// would be a gate firing on correct code, which teaches readers to skip its
// output.
//
// The liveness question only arises when the statement CHOOSES a user. An
// email is deliberately NOT treated as such a key: it arrives as input rather
// than as a row identity somebody already read, and "may this address reset a
// password" is a question about the account's state, not a lookup.
func resolvesOneUserByID(sql string) bool {
	sql = stripAssignments(sql)
	if !readsTheStatusColumn.MatchString(sql) {
		return false
	}
	// APP_USER's id, bound through its own alias. An unbound `id = $N` let a
	// joined table's key excuse a liveness defect on app_user: `… JOIN app_user
	// u … WHERE p.id = $1 AND u.archived_at IS NULL` names WHICH PROJECT, not
	// which colleague, and the statement still chooses freely among seats.
	return appUserColumn(appUserPrefix(sql),
		`id\s*(?:=\s*\$\d+|=\s*ANY\s*\(\s*\$\d+)`).MatchString(sql)
}

// readsTheStatusColumn matches a statement that hands `status` BACK rather than
// filtering on it — the difference between deferring the liveness decision to
// Go and never making it.
//
// Without this, resolving by id excused everything, and the escape hatch was
// wider than the gate: `SELECT seat_type FROM app_user WHERE id = $1 AND
// archived_at IS NULL` grants a DEACTIVATED seat live authority
// (identity/authority.go's liveUserTx), reads as a plain lookup, and would have
// passed in silence. A statement that returns `status` has at least asked the
// question; one that neither filters on it nor reads it has not.
var readsTheStatusColumn = regexp.MustCompile(`(?is)SELECT\b[^;]*?\bstatus\b[^;]*?\bFROM\b|RETURNING\b[^;]*?\bstatus\b`)

// appUserColumn matches one of APP_USER's columns and refuses somebody else's.
//
// The anchor is the whole point. `prefix` is empty whenever app_user is
// unaliased, and an unanchored pattern then matched any relation's column: a
// statement joining `project p` was read as constraining liveness because IT
// carried `p.status` and `p.archived_at`, and `id = $N` matched
// `external_id = $1`, handing a statement the id-lookup exemption on a column
// that names nobody. Both readings said "this statement already names which
// colleague" about one that does not.
//
// `[^.\w]` refuses a qualifier and refuses a longer identifier ending in the
// column's name, which is the two ways a column can be somebody else's. RE2 has
// no lookbehind, so the boundary is matched rather than asserted; it costs a
// leading group and nothing else.
func appUserColumn(prefix, column string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)(^|[^.\w])` + regexp.QuoteMeta(prefix) + column)
}

// appUserPrefix is what app_user's columns are written with in this statement:
// its alias plus a dot, or "" when the statement reads app_user unaliased.
func appUserPrefix(sql string) string {
	if m := appUserAlias.FindStringSubmatch(sql); m != nil && !isSQLKeyword(m[1]) {
		return m[1] + "."
	}
	return ""
}

// isSQLKeyword keeps a clause word from being mistaken for an alias.
// `FROM app_user WHERE …` would otherwise bind the alias "where", and every
// bare column then reads as somebody else's — the whole statement goes quiet.
func isSQLKeyword(word string) bool {
	switch strings.ToLower(word) {
	// "as" is here because the pattern's own `AS` was case-sensitive: a
	// lowercase `FROM app_user as u` captured "as" as the alias, every column
	// then read as `as.status`, and the whole statement went silent. The word
	// is refused as well as matched, so neither spelling can bind it.
	case "as", "where", "set", "on", "join", "left", "right", "inner", "cross", "order",
		"group", "having", "limit", "union", "using", "and", "or", "values", "returning":
		return true
	}
	return false
}

// firstLiveMemberLine points the report at the line that names a half rather
// than dumping the statement.
func firstLiveMemberLine(sql string) string {
	for _, line := range strings.Split(sql, "\n") {
		if strings.Contains(line, "archived_at") || strings.Contains(line, "status = 'active'") {
			return strings.TrimSpace(line)
		}
	}
	return strings.TrimSpace(strings.Split(sql, "\n")[0])
}

// liveMemberProbe is one planted source file and the verdict the detector owes
// it. `fires` is not enough on its own — a run that reports SOMETHING proves
// only that some assertion tripped — so each probe names WHICH arm must answer.
type liveMemberProbe struct {
	name string
	arm  string // "copy", "half", or "" for a statement the gate must pass over
	// mode picks the file the probe is parsed as, because the answer depends
	// on it and a probe that guesses wrong asks a different question than the
	// tree does:
	//
	//   ""         package probe, importing identity — an ordinary caller
	//   "identity" package identity — the one place a bare call is the helper's
	//   "noimport" package probe, NOT importing identity — most of the tree,
	//              and where a bare helper name is somebody else's function
	mode string
	src  string
}

// The census above is a census of ZERO once the tree is clean: it reads
// identically over a clean tree and over a detector that has
// stopped detecting. These read the detector instead, and each one is a shape
// this change actually met.
var liveMemberProbes = []liveMemberProbe{
	{"both halves spelled out, one literal", "copy", "", `
func read() string {
	return ` + "`" + `SELECT id FROM app_user WHERE status = 'active' AND archived_at IS NULL` + "`" + `
}`},
	{"the same halves in the other order", "copy", "", `
func read() string {
	return ` + "`" + `SELECT id FROM app_user WHERE archived_at IS NULL AND status = 'active'` + "`" + `
}`},
	{"the same, split across a concatenation", "copy", "", `
func read() string {
	return ` + "`" + `SELECT id FROM app_user WHERE status = 'active' AND ` + "`" + ` + ` + "`" + `archived_at IS NULL` + "`" + `
}`},
	{"the same, inside a formatter's argument", "copy", "", `
func read() string {
	return fmt.Sprintf(` + "`" + `SELECT %s FROM app_user WHERE status = 'active' AND archived_at IS NULL` + "`" + `, cols)
}`},
	{"the same, inside a transaction callback", "copy", "", `
func read() error {
	return db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, ` + "`" + `SELECT id FROM app_user WHERE status = 'active' AND archived_at IS NULL` + "`" + `).Scan(&id)
	})
}`},
	{"archived only, in a statement that chooses", "half", "", `
func read() string {
	return ` + "`" + `SELECT id FROM app_user WHERE display_name ILIKE $1 AND archived_at IS NULL` + "`" + `
}`},
	{"status only, in a statement that chooses", "half", "", `
func read() string {
	return ` + "`" + `SELECT id FROM app_user WHERE id <> $1 AND status = 'active'` + "`" + `
}`},
	{"archived only, aliased", "half", "", `
func read() string {
	return ` + "`" + `SELECT u.id FROM app_user u JOIN edge e ON e.user_id = u.id AND u.archived_at IS NULL` + "`" + `
}`},
	{"the helper called for the whole predicate", "", "", `
func read() string {
	return ` + "`" + `SELECT id FROM app_user WHERE ` + "`" + ` + identity.LiveMemberSQL("") + ` + "`" + ` AND NOT is_agent` + "`" + `
}`},
	{"the helper unqualified inside identity", "", "identity", `
func read() string {
	return ` + "`" + `SELECT id FROM app_user WHERE ` + "`" + ` + LiveMemberSQL("") + ` + "`" + ` AND NOT is_agent` + "`" + `
}`},
	{"the helper for one half and a hand-written other", "half", "", `
func read() string {
	return ` + "`" + `SELECT id FROM app_user WHERE archived_at IS NULL AND ` + "`" + ` + identity.LiveMemberSQL("")
}`},
	{"a lookalike helper from another package", "half", "", `
func read() string {
	return ` + "`" + `SELECT id FROM app_user WHERE archived_at IS NULL AND ` + "`" + ` + other.LiveMemberSQL("")
}`},
	{"a bare helper name where identity is not imported", "half", "noimport", `
func read() string {
	return ` + "`" + `SELECT id FROM app_user WHERE archived_at IS NULL AND ` + "`" + ` + LiveMemberSQL("")
}`},
	{"resolving by id and handing status back to Go", "", "", `
func read() string {
	return ` + "`" + `SELECT status, seat_type FROM app_user WHERE id = $1 AND archived_at IS NULL` + "`" + `
}`},
	{"resolving a set of ids and handing status back", "", "", `
func read() string {
	return ` + "`" + `SELECT id, status FROM app_user WHERE id = ANY($1) AND archived_at IS NULL` + "`" + `
}`},
	{"resolving by id and never asking about status at all", "half", "", `
func read() string {
	return ` + "`" + `SELECT seat_type FROM app_user WHERE id = $1 AND archived_at IS NULL` + "`" + `
}`},
	{"the same, for a column that only renders", "half", "", `
func read() string {
	return ` + "`" + `SELECT display_name FROM app_user WHERE id = $1 AND archived_at IS NULL` + "`" + `
}`},
	{"a write choosing rows by half a predicate", "half", "", `
func read() string {
	return ` + "`" + `UPDATE app_user SET password_hash = $2 WHERE display_name = $1 AND archived_at IS NULL` + "`" + `
}`},
	{"a write naming the row it was handed, status read back", "", "", `
func read() string {
	return ` + "`" + `UPDATE app_user SET locale = $2 WHERE id = $1 AND archived_at IS NULL RETURNING status` + "`" + `
}`},
	{"a write whose SET makes someone live", "", "", `
func read() string {
	return ` + "`" + `UPDATE app_user SET status = 'active' WHERE id = $1` + "`" + `
}`},
	{"the helper called, and both halves hand-written beside it", "copy", "", `
func read() string {
	return ` + "`" + `SELECT id FROM app_user WHERE ` + "`" + ` + identity.LiveMemberSQL("") +
		` + "`" + ` AND status = 'active' AND archived_at IS NULL` + "`" + `
}`},
	{"a joined table's id, not app_user's", "half", "", `
func read() string {
	return ` + "`" + `SELECT u.id FROM app_user u JOIN project p ON p.owner_id = u.id WHERE p.id = $1 AND u.archived_at IS NULL AND u.status IS NOT NULL` + "`" + `
}`},
	{"the halves assembled through a slice literal", "copy", "", `
func read() string {
	return strings.Join([]string{
		` + "`" + `SELECT id FROM app_user WHERE ` + "`" + `,
		` + "`" + `status = 'active' AND archived_at IS NULL` + "`" + `,
	}, " ")
}`},
	{"the table name folded to upper case", "copy", "", `
func read() string {
	return ` + "`" + `SELECT id FROM APP_USER WHERE STATUS = 'active' AND ARCHIVED_AT IS NULL` + "`" + `
}`},
	{"the half-spelling, table name folded", "half", "", `
func read() string {
	return ` + "`" + `SELECT id FROM APP_USER WHERE display_name ILIKE $1 AND ARCHIVED_AT IS NULL` + "`" + `
}`},
	{"a lowercase alias keyword", "copy", "", `
func read() string {
	return ` + "`" + `SELECT u.id FROM app_user as u WHERE u.status = 'active' AND u.archived_at IS NULL` + "`" + `
}`},
	{"the pair spelled into a declaration of its own", "copy", "", `
func read() string {
	return ` + "`" + `u.status = 'active' AND u.archived_at IS NULL` + "`" + `
}`},
	{"a fragment naming one half only, table unknowable", "", "", `
func read() string {
	return ` + "`" + `archived_at IS NULL` + "`" + `
}`},
	{"a joined table carrying both columns, app_user unaliased", "", "", `
func read() string {
	return ` + "`" + `SELECT id FROM app_user JOIN project p ON p.owner_id = id WHERE p.status = 'active' AND p.archived_at IS NULL` + "`" + `
}`},
	{"a column merely ending in id, not the key", "half", "", `
func read() string {
	return ` + "`" + `SELECT status FROM app_user WHERE external_id = $1 AND archived_at IS NULL` + "`" + `
}`},
	{"another table carrying the same two columns", "", "", `
func read() string {
	return ` + "`" + `SELECT id FROM voice_profile_version WHERE status = 'active' AND archived_at IS NULL` + "`" + `
}`},
	{"a joined table's archived_at, not app_user's", "", "", `
func read() string {
	return ` + "`" + `SELECT u.id FROM app_user u JOIN project p ON p.owner_id = u.id AND p.archived_at IS NULL WHERE p.id = $2` + "`" + `
}`},
	{"the foreign key alone, no app_user row read", "", "", `
func read() string {
	return ` + "`" + `SELECT 1 FROM mirror_user_map m WHERE m.app_user_id = $1 AND m.archived_at IS NULL AND m.name ILIKE $2` + "`" + `
}`},
	{"app_user read with no liveness constraint at all", "", "", `
func read() string {
	return ` + "`" + `SELECT display_name FROM app_user WHERE email ILIKE $1` + "`" + `
}`},
	// The second definition. Its arm has to be probed for the same reason the
	// first one's is: a regex that regressed would take the census quiet, and
	// quiet is the one failure this file cannot afford.
	{"a hand-spelled activatable pair", "activatable-copy", "", `
func read() string {
	return ` + "`" + `SELECT id FROM app_user WHERE status IN ('invited', 'active') AND archived_at IS NULL` + "`" + `
}`},
	// The equally natural spelling. Reported as the SAME arm rather than as a
	// half: the half arm's advice is to add status = 'active', which is exactly
	// the edit that makes redemption refuse the invitation its own link was
	// minted to spend.
	{"a hand-spelled activatable pair, the other way round", "activatable-copy", "", `
func read() string {
	return ` + "`" + `SELECT id FROM app_user WHERE status IN ('active', 'invited') AND archived_at IS NULL` + "`" + `
}`},
	{"the activatable helper called by name answers nothing", "", "", `
func read() string {
	return ` + "`" + `SELECT id FROM app_user WHERE ` + "`" + ` + identity.ActivatableMemberSQL("")
}`},
	// A LOOKALIKE from another package is not the definition, so its argument
	// must not be hidden behind a marker.
	{"a lookalike activatable helper is not the definition", "activatable-copy", "noimport", `
func read() string {
	return ` + "`" + `SELECT id FROM app_user WHERE ` + "`" + ` + other.ActivatableMemberSQL("status IN ('invited', 'active') AND archived_at IS NULL")
}`},
	// The org360 shape, for the second predicate: a bare declaration naming no
	// table, consumed elsewhere by name. Invisible to a table-keyed walk from
	// both ends, which is exactly why it needs its own recogniser.
	{"the activatable pair spelled into a bare declaration", "activatable-copy", "", `
const activatableWhere = ` + "`" + `status IN ('invited', 'active') AND archived_at IS NULL` + "`" + `
`},
	// A THIRD status pair is not this shape and must not be absorbed by it: it
	// still reads as a half, which is the honest answer for a set nobody named.
	{"a third status pair is still a half, not an activatable copy", "half", "", `
func read() string {
	return ` + "`" + `SELECT id FROM app_user WHERE status IN ('suspended', 'active') AND archived_at IS NULL` + "`" + `
}`},
}

func TestTheLiveMemberDetectorSeesWhatItClaimsTo(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	for _, tc := range liveMemberProbes {
		t.Run(tc.name, func(t *testing.T) {
			head := "package probe\n"
			// BOTH definitions, or a probe exercising the second one would flatten
			// its call to the arguments and prove the opposite of what it claims.
			names := map[string]bool{liveMemberHelper: true, activatableMemberHelper: true}
			scope := helperScope{qualifier: "identity", names: names}
			switch tc.mode {
			case "identity":
				head, scope = "package identity\n", helperScope{inside: true, names: names}
			case "noimport":
				scope = helperScope{names: names}
			default:
				head += "import (\n\t\"fmt\"\n\n\t\"" + identityPath + "\"\n)\n"
			}
			file, err := parser.ParseFile(fset, "probe.go", head+tc.src, 0)
			if err != nil {
				t.Fatalf("the probe does not parse, so it proves nothing: %v", err)
			}
			got := ""
			for _, decl := range file.Decls {
				for _, sql := range appUserStatements(decl, scope) {
					if verdict := liveMemberVerdict(sql); verdict != "" {
						got = verdict
					}
				}
			}
			if got != tc.arm {
				t.Errorf("the detector answered %q where it owes %q — a census keyed on this reads green over the wrong tree:\n%s",
					orNothing(got), orNothing(tc.arm), tc.src)
			}
		})
	}
}

// orNothing names the silent verdict, so a failure says "answered nothing"
// rather than printing an empty pair of quotes the reader has to decode.
func orNothing(arm string) string {
	if arm == "" {
		return "nothing"
	}
	return arm
}
