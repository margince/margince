// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A deal row changes through one door, and that door records the forecast.
//
// deal_forecast_history exists because deal_stage_history is written on creation
// and on a stage move and nowhere else, so an amount revised in place and a close
// date slipped leave no trace in it at all — and the second of those is the most
// common reason a real forecast moves. A reconstruction built from what remains
// reconciles over stage movement, omits the rest, and presents itself as the
// whole answer: a partial sum wearing the label of a total.
//
// Completeness is therefore a property of the WRITE, not a duty each of the
// deal's several writers discharges. Applying a patch to the deal row IS the
// recording, and a test suite cannot hold that: one written against the table
// drives the doors that call the recorder, and is silent about a door that does
// not. So this gate holds the two ways round the seam:
//
//   - a storekit patch applied to the deal table from a function that does not
//     record;
//   - a statement that assigns a forecast column with its own SQL, anywhere in
//     the tree.
//
// Both halves take their vocabulary from the deals module rather than restating
// it: the table name from its own constant, the forecast columns from the list
// the recorder consults. A gate that spelled either itself would go on guarding
// the old name the day the module moved to a new one.
//
// The classification is exhaustive rather than filtered. A patch apply whose
// target table this walk cannot resolve FAILS saying so — under-recognition is
// the one way a census must not break, because it reads a smaller tree, reports
// nothing wrong, and leaves no failing assertion to notice.

import (
	"fmt"
	"go/ast"
	"go/token"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// recorderFunc is the call that puts a row in deal_forecast_history. A function
// that makes it is a door; one that applies a deal patch without it is a bypass.
const recorderFunc = "recordForecastMovement"

// patchApplyMethods are storekit.Patch's apply verbs, and the argument position
// each one names its table in. ApplyLocked carries no table argument at all —
// the table is inside the lock — so it resolves through the lock instead, which
// is what lockTables below is for.
var patchApplyMethods = map[string]int{
	"ApplyGuarded":     2,
	"ApplyGuardedIn":   2,
	"ApplyWithVersion": 2,
	"ApplyLocked":      -1,
}

// lockMinters are the storekit calls that mint a RowLock, and the argument
// position each names its table in.
var lockMinters = map[string]int{"LockRow": 2, "LockPair": 2}

func TestEveryDealRowWriteRecordsTheForecastItMoved(t *testing.T) {
	t.Parallel()

	table, forecastColumns := dealWriteVocabulary(t)

	// The patch half reads the packages allowed to write the deal row: the module
	// that owns it, plus the ones tableownership_test.go ratifies as cross-store
	// writers of it. Reading only the owner would leave the ratified writers — a
	// merge relinking deals, a retention sweep archiving one — inside the fence
	// and outside the census, and the SQL half cannot see them either, because a
	// storekit patch has no SET clause in the source for it to match.
	//
	// The corpus is DERIVED from that gate's waiver map rather than listed here,
	// so ratifying a third cross-store writer of this table brings it under this
	// rule in the same edit.
	//
	// Whole packages, not the files that happen to apply a patch: a helper that
	// mints a lock and returns it applies nothing, and resolving only appliers
	// would call each of its callers unplaceable.
	fset := token.NewFileSet()
	var recorded, elsewhere int
	for _, scope := range packagesThatMayWrite(t, table) {
		parsedPackage := parsePackageDir(t, fset, scope.dir)
		// The package's own string constants, so a table named by constant
		// resolves exactly as a literal one does. Without them the walk reports
		// an unplaceable write and tells the author to take a row lock — advice
		// that is wrong, for a table that was legible all along.
		consts := map[string]string{}
		for _, file := range parsedPackage {
			collectStringConstants(file, consts)
		}
		locks := packageLockTables(parsedPackage, consts)
		for _, parsed := range packageFiles(t, fset, scope.dir) {
			r, e := judgeDealPatchApplies(t, parsed, table, locks, consts, scope)
			recorded, elsewhere = recorded+r, elsewhere+e
		}
	}

	// The SQL half sweeps the tree, because a statement of its own is exactly how
	// a write escapes both the patch seam and the module boundary — which is the
	// shape the accepted offer had.
	statements := 0
	for _, parsed := range (gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: func(_ string, file *ast.File) bool { return assignsAForecastColumn(file, forecastColumns) },
	}).Files(t) {
		statements += judgeDealStatements(t, parsed, table, forecastColumns)
	}

	// Three floors, because the walk has three ways to stop seeing things and
	// one total would hide which. Each sits below the real number, so a detector
	// that quietly stopped matching fails here instead of finding nothing.
	if recorded < 2 || elsewhere < 8 || statements < 2 {
		t.Errorf("resolved %d recording deal patch apply/applies, %d on other tables and %d statement(s) "+
			"writing the deal row — one of the ways of finding a write has stopped working",
			recorded, elsewhere, statements)
	}
}

// packagesThatMayWrite lists the package directories allowed to write one
// table: the module that owns it, and every package tableownership_test.go
// ratifies as a cross-store writer of it.
//
// Derived from that gate's own waiver map, whose keys are
// `module:table:file:receiver.func`. A hand-kept list here would be a second
// answer to "who may write this table", and the copy that went stale would be
// this one — the census reading a smaller tree and reporting it clean.
func packagesThatMayWrite(t *testing.T, table string) []writeScope {
	t.Helper()
	// The owner's whole package: every function in it may write the row, so
	// every function in it is judged.
	scopes := []writeScope{{dir: dealsDir}}

	// A ratified cross-store writer is judged at the FUNCTION the waiver names
	// and nowhere else. Its package writes many tables this rule has nothing to
	// say about, and judging all of them would make this gate demand that a
	// foreign module keep every one of its locks statically placeable — which is
	// legislating over code whose table is not its business.
	byDir := map[string]map[string]bool{}
	for _, waived := range crossStoreWrites.Subjects() {
		parts := strings.Split(string(waived), ":")
		if len(parts) < 4 || parts[1] != table {
			continue
		}
		name := parts[3]
		if dot := strings.LastIndex(name, "."); dot >= 0 {
			name = name[dot+1:] // receiver.func -> func
		}
		if byDir[parts[0]] == nil {
			byDir[parts[0]] = map[string]bool{}
		}
		byDir[parts[0]][name] = true
	}
	if len(byDir) == 0 {
		t.Fatalf("no cross-store writer of %q found in crossStoreWrites: either the waiver key shape "+
			"changed under this derivation, or the ratified writers went away. Both are worth knowing, "+
			"because this half of the gate silently narrows to one package either way.", table)
	}
	for _, dir := range slices.Sorted(maps.Keys(byDir)) {
		scopes = append(scopes, writeScope{dir: dir, only: byDir[dir]})
	}
	return scopes
}

// writeScope is one package this gate judges, and which of its functions. An
// empty `only` means all of them — the owner's package.
type writeScope struct {
	dir  string
	only map[string]bool
}

func (s writeScope) judges(fn string) bool { return s.only == nil || s.only[fn] }

// packageFiles pairs each of a package's parsed sources with the path it came
// from, so a finding names a file rather than a syntax tree.
//
// It walks nothing itself: parsePackageDir is the package walk this directory
// already has, and a second one would learn a new exclusion in only one place —
// the half that stopped parsing would read green over whatever it no longer saw.
func packageFiles(t *testing.T, fset *token.FileSet, dir string) []gatekit.ParsedFile {
	t.Helper()
	var out []gatekit.ParsedFile
	for _, file := range parsePackageDir(t, fset, dir) {
		out = append(out, gatekit.ParsedFile{
			Path: filepath.ToSlash(fset.Position(file.Pos()).Filename),
			File: file,
		})
	}
	return out
}

// judgeDealPatchApplies holds the patch half over one file, returning how many
// applies it saw inside a recording function and how many it resolved to another
// table. An apply it can place neither way is a finding.
func judgeDealPatchApplies(t *testing.T, parsed gatekit.ParsedFile, table string,
	locks, consts map[string]string, scope writeScope,
) (recorded, elsewhere int) {
	t.Helper()
	for _, decl := range parsed.File.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || fn.Body == nil || !scope.judges(fn.Name.Name) {
			continue
		}
		records := callsFunction(fn, recorderFunc)
		for _, apply := range patchAppliesIn(fn) {
			switch target, known := apply.table(fn, locks, consts); {
			case records:
				// The door itself. It has satisfied the rule whichever row it
				// writes, so its lock is deliberately not resolved.
				recorded++
			case !known:
				t.Errorf("%s:%s applies a patch through %s and this gate cannot tell which table it writes.\n"+
					"\tTake the lock in this function with storekit.LockRow, or return it from one that does, "+
					"so the walk can place the write. An unplaceable deal write is how the forecast history "+
					"went short in the first place.",
					parsed.Path, fn.Name.Name, apply.method)
			case target == table:
				t.Errorf("%s:%s applies a patch to %q without recording the forecast it may have moved.\n"+
					"\tApply it through the deals module's own seam (applyDealPatchGuarded / "+
					"applyDealPatchLocked), which records for every writer, rather than calling "+
					"storekit's %s directly.",
					parsed.Path, fn.Name.Name, table, apply.method)
			default:
				elsewhere++
			}
		}
	}
	return recorded, elsewhere
}

// judgeDealStatements holds the SQL half over one file, returning how many
// statements writing the deal row it read. A statement that sets a forecast
// column is a finding wherever in the tree it is written.
func judgeDealStatements(t *testing.T, parsed gatekit.ParsedFile, table string, forecastColumns []string) int {
	t.Helper()
	seen := 0
	for _, statement := range statementsIn(parsed.File) {
		column, assigns := assignedForecastColumn(statement, forecastColumns)
		if !assigns {
			continue
		}
		seen++
		switch target, known := statementWriteTarget(statement); {
		case !known:
			t.Errorf("%s assigns %s in a statement whose table this gate cannot read:\n\t%s\n"+
				"\tSpell the table as a literal in the SQL. A statement that assembles its target at "+
				"run time is one this census cannot place, and an unplaceable write is where a census "+
				"goes blind without saying so.",
				parsed.Path, column, statement)
		case target == table:
			t.Errorf("%s writes %s.%s with its own statement:\n\t%s\n"+
				"\tA forecast column moves through the deals module's patch seam, which records the "+
				"move in deal_forecast_history. A statement of its own writes the column and no "+
				"history, and the table's contract to whoever reconstructs a forecast is that it "+
				"holds every move a stage transition did not.",
				parsed.Path, table, column, statement)
		}
	}
	return seen
}

// patchApply is one storekit patch apply, located.
type patchApply struct {
	method string
	args   []ast.Expr
}

// table resolves the row the apply writes: the argument the verb names it in,
// or — for ApplyLocked, whose table rides the lock — the table the lock was
// taken on. Reporting false is a result, not a shrug: the caller fails on it.
func (a patchApply) table(fn *ast.FuncDecl, locks, consts map[string]string) (string, bool) {
	position := patchApplyMethods[a.method]
	if position >= 0 {
		if position >= len(a.args) {
			return "", false
		}
		return resolveString(a.args[position], consts)
	}
	if len(a.args) < 3 {
		return "", false
	}
	name, isIdent := a.args[2].(*ast.Ident)
	if !isIdent {
		return "", false
	}
	target, known := lockTableFor(fn, name.Name, locks, consts)
	return target, known
}

// patchAppliesIn finds every storekit patch apply in one function body.
func patchAppliesIn(fn *ast.FuncDecl) []patchApply {
	var out []patchApply
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		selector, isSelector := call.Fun.(*ast.SelectorExpr)
		if !isSelector {
			return true
		}
		if _, isApply := patchApplyMethods[selector.Sel.Name]; !isApply {
			return true
		}
		// storekit's own package qualifier would make this a type name rather
		// than a patch value; every apply in the tree is a method on a *Patch.
		out = append(out, patchApply{method: selector.Sel.Name, args: call.Args})
		return true
	})
	return out
}

// lockTableFor resolves which table a named RowLock held in this function was
// taken on: one this function's own body minted, or — when the name is a
// parameter — the table every caller in the package locked before handing it in.
func lockTableFor(fn *ast.FuncDecl, name string, locks, consts map[string]string) (string, bool) {
	if isParam(fn, name) {
		table, known := locks[lockKey(fn.Name.Name, name)]
		return table, known
	}
	var table string
	var known bool
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		assign, isAssign := node.(*ast.AssignStmt)
		if !isAssign || !assignsName(assign, name) {
			return true
		}
		for _, rhs := range assign.Rhs {
			call, isCall := rhs.(*ast.CallExpr)
			if !isCall {
				continue
			}
			if target, ok := lockTableOfCall(call, locks, consts); ok {
				table, known = target, true
			}
		}
		return true
	})
	return table, known
}

// lockKey names one thing in a package that holds a RowLock: a function's
// returned lock, or one of its parameters. Two namespaces in one map, because
// resolving either can depend on the other and a fixed point over both is the
// only order that terminates without a topological sort.
func lockKey(fn, held string) string { return fn + "#" + held }

const lockReturn = ""

// packageLockTables resolves, for one package, which table every RowLock that
// crosses a function boundary was taken on — locks returned by a minting helper
// and locks handed in as parameters.
//
// It iterates to a fixed point rather than resolving in one pass: a parameter's
// table comes from its callers, and a caller may itself have been handed the
// lock. Three passes settle this tree; the loop stops when a pass learns
// nothing, so it terminates on any shape.
//
// A lock it cannot place stays absent, which is what makes the apply that uses
// it fail. Guessing would be worse than not knowing: the guess that a lock is
// NOT on the deal row is exactly the answer that lets an unrecorded deal write
// through.
func packageLockTables(files []*ast.File, consts map[string]string) map[string]string {
	locks := map[string]string{}
	for changed := true; changed; {
		changed = false
		for _, file := range files {
			for _, decl := range file.Decls {
				fn, isFunc := decl.(*ast.FuncDecl)
				if !isFunc || fn.Body == nil {
					continue
				}
				changed = learnReturnedLock(fn, locks, consts) || changed
				changed = learnPassedLocks(fn, files, locks, consts) || changed
			}
		}
	}
	return locks
}

// learnReturnedLock records the table a lock-returning helper mints its lock on.
func learnReturnedLock(fn *ast.FuncDecl, locks, consts map[string]string) bool {
	key := lockKey(fn.Name.Name, lockReturn)
	if _, known := locks[key]; known || !returnsRowLock(fn) {
		return false
	}
	learned := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		if table, ok := lockTableOfCall(call, locks, consts); ok {
			locks[key] = table
			learned = true
		}
		return true
	})
	return learned
}

// learnPassedLocks resolves each of fn's RowLock parameters from the arguments
// its callers pass. Callers that disagree leave the parameter unresolved: a
// helper reached with two different tables cannot be placed, and placing it
// wrongly is the failure this gate exists to prevent.
func learnPassedLocks(fn *ast.FuncDecl, files []*ast.File, locks, consts map[string]string) bool {
	learned := false
	for position, name := range rowLockParams(fn) {
		key := lockKey(fn.Name.Name, name)
		if _, known := locks[key]; known {
			continue
		}
		table, agreed := calledWithLockOn(fn.Name.Name, position, files, locks, consts)
		if !agreed {
			continue
		}
		locks[key] = table
		learned = true
	}
	return learned
}

// calledWithLockOn reads the table every call to callee locks before passing its
// argument at position, reporting false unless at least one call resolves and
// all that resolve agree.
func calledWithLockOn(callee string, position int, files []*ast.File,
	locks, consts map[string]string,
) (string, bool) {
	table, agreed := "", false
	for _, file := range files {
		for _, decl := range file.Decls {
			caller, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || caller.Body == nil {
				continue
			}
			for _, args := range callsTo(caller, callee) {
				if position >= len(args) {
					continue
				}
				ident, isIdent := args[position].(*ast.Ident)
				if !isIdent {
					continue
				}
				found, known := lockTableFor(caller, ident.Name, locks, consts)
				switch {
				case !known:
				case !agreed:
					table, agreed = found, true
				case found != table:
					return "", false
				}
			}
		}
	}
	return table, agreed
}

// rowLockParams maps each storekit.RowLock parameter's position to its name.
func rowLockParams(fn *ast.FuncDecl) map[int]string {
	out := map[int]string{}
	position := 0
	for _, field := range fn.Type.Params.List {
		selector, isSelector := field.Type.(*ast.SelectorExpr)
		names := max(len(field.Names), 1)
		for i := 0; i < names; i++ {
			if isSelector && selector.Sel.Name == "RowLock" && i < len(field.Names) {
				out[position] = field.Names[i].Name
			}
			position++
		}
	}
	return out
}

// callsTo returns the argument list of every call to the named package function
// inside one function body.
func callsTo(fn *ast.FuncDecl, callee string) [][]ast.Expr {
	var out [][]ast.Expr
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		switch named := call.Fun.(type) {
		case *ast.Ident:
			if named.Name == callee {
				out = append(out, call.Args)
			}
		case *ast.SelectorExpr:
			if named.Sel.Name == callee {
				out = append(out, call.Args)
			}
		}
		return true
	})
	return out
}

func isParam(fn *ast.FuncDecl, name string) bool {
	for _, field := range fn.Type.Params.List {
		for _, param := range field.Names {
			if param.Name == name {
				return true
			}
		}
	}
	return false
}

// lockTableOfCall reads the table out of a lock-minting storekit call, or out of
// a package function this walk already resolved to one.
func lockTableOfCall(call *ast.CallExpr, locks, consts map[string]string) (string, bool) {
	switch callee := call.Fun.(type) {
	case *ast.SelectorExpr:
		position, mints := lockMinters[callee.Sel.Name]
		if !mints || position >= len(call.Args) {
			return "", false
		}
		return resolveString(call.Args[position], consts)
	case *ast.Ident:
		table, resolved := locks[lockKey(callee.Name, lockReturn)]
		return table, resolved
	}
	return "", false
}

func returnsRowLock(fn *ast.FuncDecl) bool {
	if fn.Type.Results == nil {
		return false
	}
	for _, result := range fn.Type.Results.List {
		selector, isSelector := result.Type.(*ast.SelectorExpr)
		if isSelector && selector.Sel.Name == "RowLock" {
			return true
		}
	}
	return false
}

func assignsName(assign *ast.AssignStmt, name string) bool {
	for _, lhs := range assign.Lhs {
		if ident, isIdent := lhs.(*ast.Ident); isIdent && ident.Name == name {
			return true
		}
	}
	return false
}

func callsFunction(fn *ast.FuncDecl, name string) bool {
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		if ident, isIdent := call.Fun.(*ast.Ident); isIdent && ident.Name == name {
			found = true
		}
		return !found
	})
	return found
}

// dealWriteVocabulary asks the deals module what this gate is about: the table
// its writes target, and the columns a forecast reconstruction reads.
//
// Derived rather than restated. A gate carrying its own copy of either would
// keep guarding the old spelling on the day the module changed it — passing over
// the rename, which is when a rule is least likely to still hold.
func dealWriteVocabulary(t *testing.T) (table string, forecastColumns []string) {
	t.Helper()
	files := parsePackageDir(t, token.NewFileSet(), dealsDir)

	consts := map[string]string{}
	for _, file := range files {
		collectStringConstants(file, consts)
	}
	table, known := consts[dealTableConst]
	if !known {
		t.Fatalf("%s declares no %s constant: this gate no longer knows which table it protects",
			dealsDir, dealTableConst)
	}

	for _, element := range packageVarCompositeLit(t, files, forecastColumnsVar).Elts {
		column, resolved := resolveString(element, consts)
		if !resolved {
			t.Fatalf("%s holds an element this gate cannot resolve to a column name, so its census "+
				"would silently cover fewer columns than the recorder does", forecastColumnsVar)
		}
		forecastColumns = append(forecastColumns, column)
	}
	if len(forecastColumns) == 0 {
		t.Fatalf("%s resolved to no columns: the SQL half would then judge nothing and report a clean tree",
			forecastColumnsVar)
	}
	return table, forecastColumns
}

// The two declarations the vocabulary comes out of.
const (
	dealTableConst     = "dealTable"
	forecastColumnsVar = "forecastColumns"
)

// The SQL half reads a statement in two independent steps, and that order is
// the whole design.
//
// It does not start from the table, because a table has many legal spellings —
// aliased, quoted, schema-qualified, ONLY-prefixed, reached through DO UPDATE or
// MERGE, assembled by a format string — and a pattern that knows some of them
// reads a clean tree over the rest. It starts from the COLUMN, of which there
// are three, each spelled one way. Only then does it ask which table the
// statement writes, and a statement that assigns a forecast column and whose
// table cannot be read FAILS: an unplaceable write is a finding, never a pass.
var (
	// setClause isolates what a statement ASSIGNS from what it merely names. A
	// column read in a WHERE predicate or listed in a SELECT is not a write, and
	// `SET` is the one token every assigning form shares — a plain UPDATE, an
	// upsert's DO UPDATE, a MERGE's WHEN MATCHED THEN UPDATE. `\b` keeps it out
	// of `offset`.
	setClause = regexp.MustCompile(`(?is)\bset\b(.*?)(?:\bwhere\b|\breturning\b|;|$)`)
	// writeTarget matches the table an UPDATE, an INSERT (whose ON CONFLICT can
	// carry an UPDATE) or a MERGE writes, through every identifier spelling
	// Postgres accepts: quoted, schema-qualified, ONLY-prefixed.
	writeTarget = regexp.MustCompile(
		`(?is)\b(?:update|insert\s+into|merge\s+into)\s+(?:only\s+)?(?:"?[a-z_][a-z0-9_]*"?\.)?"?([a-z_][a-z0-9_]*)"?`)
	// assignmentFmt matches `col = …` and the multi-column form
	// `(a, col) = (…)`, with or without the identifier quoted. Anchored on the
	// whole identifier so `currency` is not found inside `base_currency` — the
	// substring reading is the half that survives a careless escape — and the
	// parenthesised branch may not cross a `(` of its own, so a column list
	// cannot reach forward to an unrelated `=`.
	assignmentFmt = `(?i)(?:(?:^|[,\s"])%[1]s"?\s*=)|(?:\([^()=]*[,\s"]?%[1]s"?[^()=]*\)\s*=)`
)

// assignsAForecastColumn is the SQL half's sweep subject: a file holding a
// statement that assigns one of the deal's forecast columns. It is the same
// question the detector asks, so the negative-space sweep proves the roots
// against every file the judgment would have something to say about.
func assignsAForecastColumn(file *ast.File, forecastColumns []string) bool {
	for _, statement := range statementsIn(file) {
		if _, assigns := assignedForecastColumn(statement, forecastColumns); assigns {
			return true
		}
	}
	return false
}

// assignedForecastColumn names the forecast column a statement assigns, if any.
// It reads only the statement's SET clauses, so a forecast column a SELECT
// returns or a WHERE narrows on is correctly not a write.
func assignedForecastColumn(statement string, forecastColumns []string) (string, bool) {
	for _, assigned := range setClause.FindAllStringSubmatch(statement, -1) {
		for _, column := range forecastColumns {
			if regexp.MustCompile(fmt.Sprintf(assignmentFmt, regexp.QuoteMeta(column))).MatchString(assigned[1]) {
				return column, true
			}
		}
	}
	return "", false
}

// statementWriteTarget names the table a statement writes. Reporting false is a
// result the caller fails on: a target assembled at run time — a %s in a format
// string, a name concatenated from a variable — is one this census cannot place.
func statementWriteTarget(statement string) (string, bool) {
	match := writeTarget.FindStringSubmatch(statement)
	if match == nil {
		return "", false
	}
	return strings.ToLower(match[1]), true
}

// statementsIn returns every SQL-shaped string in one file, comments stripped
// and whitespace collapsed, with adjacent literals joined the way the compiler
// joins them.
//
// It matches STATEMENTS and not lines: a statement broken across lines is one
// write however it is laid out, a column named in a `--` comment is prose, and
// a statement a Go author split with `+` is still one statement.
func statementsIn(file *ast.File) []string {
	var out []string
	for _, literal := range gatekit.SQLStatementsOf(file) {
		// Split on `;` AFTER stripping comments, so a semicolon inside a `--`
		// comment does not cut a statement in half.
		//
		// One Go string can hold two statements, and placing the pair by its
		// first table is a FALSE PASS — `UPDATE offer SET status = 'x';
		// UPDATE deal SET amount_minor = $1` would resolve to `offer` and report
		// nothing. That is the one outcome this census must not produce, so each
		// statement is placed on its own.
		for _, statement := range strings.Split(stripSQLComments(literal), ";") {
			if collapsed := collapsedSQL(statement); collapsed != "" {
				out = append(out, collapsed)
			}
		}
	}
	return out
}

// stripSQLComments removes `--` line comments so a column named in one is not
// read as an assignment.
func stripSQLComments(sql string) string {
	var kept []string
	for _, line := range strings.Split(sql, "\n") {
		if at := strings.Index(line, "--"); at >= 0 {
			line = line[:at]
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}
