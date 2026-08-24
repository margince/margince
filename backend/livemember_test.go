// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package backendarch

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

	"github.com/gradionhq/margince/backend/internal/shared/gatekit"
)

const (
	liveMemberHelper = "LiveMemberSQL"
	liveMemberOwner  = "internal/modules/identity/livemember.go"
	identityPath     = "github.com/gradionhq/margince/backend/internal/modules/identity"
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
// Each entry is a FILE, so a NEW hand-spelled copy in one of these packages is
// still a finding: the ratification covers the statements that exist, not the
// package.
var cannotReachIdentity = gatekit.Waive(map[string]string{
	"internal/modules/activities/lifecycle.go":    "activities cannot import identity (ADR-0054 §3); the predicate must move tier first",
	"internal/modules/dealrooms/store_public.go":  "dealrooms cannot import identity (ADR-0054 §3); the predicate must move tier first",
	"internal/modules/people/counterpartyname.go": "people cannot import identity (ADR-0054 §3); the predicate must move tier first",
	"internal/modules/people/leadrouting.go":      "people cannot import identity (ADR-0054 §3); the predicate must move tier first",
	"internal/modules/people/linkedinmatch.go":    "people cannot import identity (ADR-0054 §3); the predicate must move tier first",
	"internal/modules/projects/surface.go":        "projects cannot import identity (ADR-0054 §3); the predicate must move tier first",
	"internal/modules/projects/transfer.go":       "projects cannot import identity (ADR-0054 §3); the predicate must move tier first",
	"internal/modules/search/graphedge.go":        "search cannot import identity (ADR-0054 §3); the predicate must move tier first",
})

// deliberatelyNotLiveness ratifies the half-spellings that are not answering
// the workforce question at all.
//
// overlay's user-map eligibility is `NOT is_agent AND archived_at IS NULL`, and
// it is a different set on purpose: it decides whom an admin may GRANT a mirror
// mapping to, not who still works here. It is already held as its own
// three-site invariant, named in selectUserMapTargetSQL's comment.
//
// What the difference costs, stated rather than waved away: a DEACTIVATED seat
// is not archived, so all three overlay sites still offer it a mapping — and
// listUserMapSQL's own comment justifies excluding archived users as "a seat
// that no longer logs in", which is just as true of a deactivated one. Whether
// overlay wants the tighter set is overlay's call and moves three statements at
// once, so it is issue margince/margince#2592 rather than a change made here on
// a guess.
var deliberatelyNotLiveness = gatekit.Waive(map[string]string{
	"internal/modules/overlay/usermapadmin.go": "overlay mapping eligibility is NOT is_agent AND archived_at IS NULL, a different set held as its own three-site invariant; see issue 2592",
	"internal/modules/overlay/usermapseed.go":  "overlay mapping eligibility is NOT is_agent AND archived_at IS NULL, a different set held as its own three-site invariant; see issue 2592",
	"internal/modules/identity/roster.go":      "listUsersAllQuery is the ADMIN roster and says so: every non-archived member REGARDLESS of status, because a deactivated member has to be visible to reactivate",
	"internal/modules/identity/reset.go":       "OperatorResetPassword is the operator CLI's lockout path, never exposed over HTTP; it must reach an account whose status is not active, which is what administrator lockout means. Login itself calls the helper (lockout.go)",
})

// appUserAlias finds what app_user is called in a statement, so a sibling
// table's archived_at is not read as app_user's.
//
// `FROM project p JOIN app_user u` puts two archived_at columns in scope and
// only one of them is a workforce question. An unaliased `FROM app_user` binds
// the bare column names instead.
var appUserAlias = regexp.MustCompile(`\bapp_user\b(?:\s+AS)?\s+([a-z]\w*)`)

// readsAppUser matches the table itself and not a column named after it: the
// trailing `\b` already refuses `app_user_id`, since an underscore is a word
// character. A statement that only carries the foreign key is not reading the
// row, and asking it about liveness would report a pairing nobody wrote.
var readsAppUser = regexp.MustCompile(`\bapp_user\b`)

func TestOnlyOneSpellingOfALiveMember(t *testing.T) {
	// A ratification that stops matching covers a site that has moved or been
	// fixed, and leaving it in place quietly re-exempts whatever takes its name.
	defer cannotReachIdentity.AssertAllMatched(t)
	defer deliberatelyNotLiveness.AssertAllMatched(t)

	fset := token.NewFileSet()
	var copies, halves []string
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
			names:     map[string]bool{liveMemberHelper: true},
		}
		for _, decl := range file.Decls {
			for _, sql := range appUserStatements(decl, scope) {
				judged++
				status, archived := liveMemberHalves(sql)
				if !status && !archived {
					continue
				}
				constrained++
				switch {
				case status && archived:
					if strings.Contains(sql, liveMemberHelper) || cannotReachIdentity.Waived(t, slash) {
						continue
					}
					copies = append(copies, fmt.Sprintf("%s: %s", path, firstLiveMemberLine(sql)))
				default:
					if !offersAUser(sql) || deliberatelyNotLiveness.Waived(t, slash) {
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
		ast.Inspect(file, func(n ast.Node) bool {
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
		if !ok || !readsAppUser.MatchString(text) {
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
	prefix := ""
	if m := appUserAlias.FindStringSubmatch(sql); m != nil && !isSQLKeyword(m[1]) {
		prefix = m[1] + "."
	}
	status = regexp.MustCompile(regexp.QuoteMeta(prefix) + `status\s*=\s*'active'`).MatchString(sql)
	archived = regexp.MustCompile(regexp.QuoteMeta(prefix) + `archived_at\s+IS\s+NULL`).MatchString(sql)
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
// A WRITE is excluded: an UPDATE choosing rows to deactivate offers nobody, and
// the row set it touches is a question about that statement's own intent rather
// than about who still works here.
func offersAUser(sql string) bool {
	return !writesAppUser.MatchString(sql) && !resolvesOneUserByID(sql)
}

// writesAppUser matches a statement that mutates rather than reads.
var writesAppUser = regexp.MustCompile(`(?is)^\s*(UPDATE|DELETE|INSERT)\b`)

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
	return resolvesByID.MatchString(stripAssignments(sql))
}

// resolvesByID matches primary-key equality, alias or not, single or set — the
// three spellings this tree uses to say "this user, the one I was given".
var resolvesByID = regexp.MustCompile(`(?i)\b(?:[a-z]\w*\.)?id\s*(?:=\s*\$\d+|=\s*ANY\s*\(\s*\$\d+)`)

// isSQLKeyword keeps a clause word from being mistaken for an alias.
// `FROM app_user WHERE …` would otherwise bind the alias "where", and every
// bare column then reads as somebody else's — the whole statement goes quiet.
func isSQLKeyword(word string) bool {
	switch strings.ToLower(word) {
	case "where", "set", "on", "join", "left", "right", "inner", "cross", "order",
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
	{"resolving the user the caller named", "", "", `
func read() string {
	return ` + "`" + `SELECT display_name FROM app_user WHERE id = $1 AND archived_at IS NULL` + "`" + `
}`},
	{"resolving a set of ids the caller named", "", "", `
func read() string {
	return ` + "`" + `SELECT id, display_name FROM app_user WHERE id = ANY($1) AND archived_at IS NULL` + "`" + `
}`},
	{"a write choosing rows to deactivate", "", "", `
func read() string {
	return ` + "`" + `UPDATE app_user SET status = 'deactivated' WHERE status = 'active'` + "`" + `
}`},
	{"a write whose SET makes someone live", "", "", `
func read() string {
	return ` + "`" + `UPDATE app_user SET status = 'active' WHERE id = $1` + "`" + `
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
}

func TestTheLiveMemberDetectorSeesWhatItClaimsTo(t *testing.T) {
	fset := token.NewFileSet()
	for _, tc := range liveMemberProbes {
		t.Run(tc.name, func(t *testing.T) {
			head := "package probe\n"
			names := map[string]bool{liveMemberHelper: true}
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
					status, archived := liveMemberHalves(sql)
					switch {
					case status && archived && !strings.Contains(sql, liveMemberHelper):
						got = "copy"
					case (status != archived) && offersAUser(sql):
						got = "half"
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
