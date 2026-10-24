// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// The walk behind an object-half census: which functions read a table, and
// which of them reach that table's object gate.
//
// Its own file because two censuses now ask it — the relationship edge and the
// lead record — and the question is the same one both times: RBAC's object half
// says whether a caller may read this KIND of record at all, the row half says
// which ones, and a read that takes only the second is the defect neither
// composerowscope_test.go nor rbacgate_test.go covers in the compose tier.
//
// It is deliberately NOT in gatekit. The shared walk moves there when a third
// SHAPE needs it; what these two share is one shape used twice, and an
// abstraction lifted out of the tree before its second real caller is shaped for
// the cases it happened to see. restrictedreaders_test.go is the structural
// near-twin that would make three — and it is not refactored here, because
// refactoring it against a walk built for two is the same mistake one level up.

import (
	"go/ast"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// objectGate is one census's subject: the table whose reads carry the
// obligation, and the spellings that discharge it.
//
// The object and the table are one field because they are one name in every
// case this tree has — `relationship` reads are governed by the `relationship`
// object, `lead` by `lead`. A schema where they diverge would need two, and
// would need the census to say which of them the failure message is about.
type objectGate struct {
	// object is the RBAC object name AND the table name.
	object string
	// objectGateSatisfies marks a census whose question auth.Require(ctx,
	// object, …) answers: "may this caller read a record of this kind at all".
	// For the five record censuses that call IS the admission; only the edge
	// adds a seed of its own.
	//
	// A FIELD MASK asks a different question — which COLUMN of a readable row
	// this reader may have — and the object gate cannot answer it, because
	// every masked read passes the object gate too. So the default is off, and
	// a census that wants the object half says so: the direction of the mistake
	// is what decides that. A census that forgets the field reports reads it
	// should have admitted, and somebody reads the failure. A census that takes
	// the object gate for an answer it does not give reports PASS over the read
	// it exists to find, and nothing says so at all — which is what happened
	// here: the relationship graph ordered an account's open deals by
	// d.amount_minor with no mask anywhere, and the deal-amount census read it
	// as guarded because readOpenDeals opens with auth.Require(ctx, "deal").
	objectGateSatisfies bool
	// literal matches a SQL string literal reading the table.
	literal *regexp.Regexp
	// gateSeeds are the spellings that ARE the admission, anywhere in the tree.
	gateSeeds []string
	// rowHalfSeeds bound WHICH rows and answer nothing about whether the caller
	// may read this kind of record at all — so alone they are the INVERTED form
	// of the defect, and they satisfy only inside rowHalfOwners.
	rowHalfSeeds []string
	// rowHalfOwners are the packages that ask the object gate at their store
	// entry points, where a row-half spelling is the clause that entry point
	// composes rather than a substitute for it.
	rowHalfOwners []string
}

// objectGateCall matches a call that asks the object gate under any of this
// tree's spellings, paired with the object name in the same CALL.
//
// Two families, and the second is not a weaker form of the first:
//
//   - Require-shaped — auth.Require, or a package-local wrapper such as
//     contact360's requireRead. The answer is a 403 and the request ends.
//   - auth.ReadGranted. The answer is a boolean, for the reads that must
//     DEGRADE rather than refuse: a list ordered by a company name the caller
//     may not read falls back to its tie-breaker instead of failing a page the
//     caller never asked to sort. It asks the same merged permissions Require
//     asks, so a read that takes it has taken the object half.
//
// Recognising only the first is what let four hand-rolled copies of the second
// sit in this tree reading as ungated, which is the reverse of the error this
// walk exists to prevent: a census that cannot see a real gate teaches its
// readers to distrust it.
var objectGateCall = regexp.MustCompile(`^([Rr]equire[A-Za-z]*|ReadGranted)$`)

// site is one read: the function that holds it, empty for a package-level SQL
// fragment, the first line of the SQL for the report, and whether that
// function's OWN body takes the admission.
//
// holdsGate is answered from the declaration itself rather than by looking the
// function up by name, and that distinction is the whole reason this field
// exists. *Store and Handlers in one module routinely spell the same method
// names — contacts has both a Store.RemoveProjectStakeholder and a
// Handlers.RemoveProjectStakeholder — so a by-name index lets one answer for
// the other, and which one wins is Go map iteration order. rbacgate_test.go
// says so in its own header, having been bitten by exactly this. The name index
// below is still used, but only to reach a gate in a SIBLING function, where a
// collision can merely be optimistic rather than wrong.
type site struct {
	function  string
	sql       string
	holdsGate bool
	// calls is what this declaration calls, so a gate reached through a helper
	// in a sibling file resolves without consulting this declaration's name.
	calls map[string]bool
	// fragments names the package-level values a fragment site declares.
	fragments []string
}

func (g objectGate) readSites(parsed gatekit.ParsedFile, consts map[string]string) []site {
	var sites []site
	for _, decl := range parsed.File.Decls {
		reads := gatekit.DeclReads(decl, g.literal)
		if len(reads) == 0 {
			continue
		}
		refs := referencesIn(decl, consts)
		sites = append(sites, site{
			function: reads[0].Function, sql: gatekit.FirstLineOf(reads[0].SQL),
			holdsGate: g.holdsSeedGate(refs, pkgOf(parsed.Path)), calls: refs.calls,
			fragments: valueNames(decl),
		})
	}
	return sites
}

// subject names a site for its verdict: file:function, or file:value for a
// package-level fragment, so a file-keyed verdict still answers for either.
func (s site) subject(filePath string) string {
	switch {
	case s.function != "":
		return filePath + ":" + s.function
	case len(s.fragments) > 0:
		return filePath + ":" + s.fragments[0]
	}
	return filePath
}

// carriesGate is whether a function site's read is behind the admission: its
// own body asks, or it calls a helper that vouches for its caller.
func (s site) carriesGate(gated map[string]bool) bool {
	return s.holdsGate || callsAGatedHelper(s.calls, gated)
}

func valueNames(decl ast.Decl) []string {
	general, isGeneral := decl.(*ast.GenDecl)
	if !isGeneral {
		return nil
	}
	var names []string
	for _, spec := range general.Specs {
		if value, isValue := spec.(*ast.ValueSpec); isValue {
			for _, name := range value.Names {
				names = append(names, name.Name)
			}
		}
	}
	return names
}

func callsAGatedHelper(calls map[string]bool, gated map[string]bool) bool {
	for name := range calls {
		if gated[name] {
			return true
		}
	}
	return false
}

func pkgOf(filePath string) string { return path.Dir(filePath) }

// objectWalk is one census's resolution: the functions that vouch for a caller,
// per package, and the package bodies a fragment site is judged against.
type objectWalk struct {
	gated  map[string]map[string]bool
	bodies map[string]packageReferences
}

// walkPackages resolves, per package directory, every function that
// reaches the object's gate — directly, or through another function in the
// same package. Transitive because this tree routinely splits a read across the
// function holding the SQL and the helper building its predicate (company360's
// edgeScope, meetingbrief's seatJoinPredicate), and a gate asking only about
// direct calls would report the reader red while its admission sits in the file
// next door.
func (g objectGate) walkPackages(t *testing.T, files []gatekit.ParsedFile) objectWalk {
	t.Helper()
	// The WHOLE package, not only the files that read the table. The admission
	// this tree writes routinely lives in a sibling file that holds no SQL of
	// its own — company360's edgeScope sits in sections.go while the three reads it
	// gates are in graphreads.go and contacts.go — so a resolution seeded from
	// the subject files alone reports gated code as ungated, which costs the
	// gate its credibility faster than a miss does.
	walk := objectWalk{gated: map[string]map[string]bool{}, bodies: map[string]packageReferences{}}
	for _, parsed := range files {
		pkg := path.Dir(parsed.Path)
		if walk.bodies[pkg].funcs != nil {
			continue
		}
		walk.bodies[pkg] = packageFunctionReferences(t, pkg)
		walk.gated[pkg] = g.vouchingFunctions(pkg, walk.bodies[pkg])
	}
	return walk
}

// vouchingFunctions is the fixed point of "holds or reaches the gate" over the
// bodies that run no read of the table themselves.
//
// A name is gated when any declaration spelling it is: a union, never an
// overwrite. That is optimistic where two receivers share a method name. The
// index only answers "is there a gated helper called X here". A site's own
// declaration is asked directly, so a gated neighbour cannot excuse it.
func (g objectGate) vouchingFunctions(pkg string, walked packageReferences) map[string]bool {
	gated := map[string]bool{}
	for name, decls := range walked.funcs {
		for _, refs := range decls {
			if g.holdsSeedGate(refs, pkg) && !g.spendsItsGate(refs, walked.fragments) {
				gated[name] = true
				break
			}
		}
	}
	for grew := true; grew; {
		grew = false
		for name, decls := range walked.funcs {
			if gated[name] {
				continue
			}
			for _, refs := range decls {
				if !g.spendsItsGate(refs, walked.fragments) && callsAGatedHelper(refs.calls, gated) {
					gated[name], grew = true, true
					break
				}
			}
		}
	}
	return gated
}

// carries is whether a read site is behind the admission. A fragment counts
// as gated by a function of its file that vouches, or that names the fragment
// and holds or reaches the gate. A function that runs only its own read says
// nothing about a fragment it never names.
func (w objectWalk) carries(t *testing.T, g objectGate, parsed gatekit.ParsedFile, s site, consts map[string]string) bool {
	t.Helper()
	pkg := pkgOf(parsed.Path)
	if s.function != "" || s.holdsGate {
		return s.carriesGate(w.gated[pkg])
	}
	names := w.bodies[pkg].namingFragments(s.fragments)
	for name, decls := range functionBodies(parsed, consts) {
		if w.gated[pkg][name] {
			return true
		}
		for _, refs := range decls {
			namesIt := slices.ContainsFunc(names, func(n string) bool { return refs.idents[n] })
			if namesIt && (g.holdsSeedGate(refs, pkg) || callsAGatedHelper(refs.calls, w.gated[pkg])) {
				return true
			}
		}
	}
	return false
}

// spendsItsGate is whether the body runs its own read of the table. Its
// admission covers that read only.
//
// Such a gate may only empty this body's answer, or run after a caller's read
// already did, so it never vouches for the caller. A statement the walk cannot
// read, built at run time or held in a local, counts as a read of the table.
func (g objectGate) spendsItsGate(refs references, fragments map[string]packageValue) bool {
	if g.literal == nil || !refs.runs {
		return false
	}
	if refs.opaqueRun {
		return true
	}
	for name := range refs.runIdents {
		if _, known := fragments[name]; !known {
			return true
		}
	}
	for text := range refs.literals {
		if g.literal.MatchString(text) {
			return true
		}
	}
	for name := range refs.idents {
		if slices.ContainsFunc(fragments[name].literals, g.literal.MatchString) {
			return true
		}
	}
	return false
}

// holdsSeedGate reports whether a function body IS the admission: one of the
// platform spellings, or the older object-gate form — a Require-shaped call
// taking the object as an argument.
func (g objectGate) holdsSeedGate(refs references, pkg string) bool {
	for _, seed := range g.gateSeeds {
		if refs.calls[seed] {
			return true
		}
	}
	// The older form is read off the CALL rather than from the body at large,
	// because a body holding RequireHuman(ctx) and, separately, an unrelated
	// object-name string — an entity-type constant, a table name in a comment's
	// sibling literal — would otherwise vouch for itself.
	if g.objectGateSatisfies && refs.gatedObjects[g.object] {
		return true
	}
	if !slices.Contains(g.rowHalfOwners, pkg) {
		return false
	}
	for _, seed := range g.rowHalfSeeds {
		if refs.calls[seed] {
			return true
		}
	}
	return false
}

// references is what a function CALLS and what string literals it holds. Read
// off the syntax rather than the source text, so a gate spelling cannot be
// matched inside a comment that merely discusses it — and calls only, because a
// parameter or local variable that happens to share a gated function's name is
// not a call to it.
type references struct {
	calls    map[string]bool
	literals map[string]bool
	// idents are the names a body mentions, so a read through a package-level
	// SQL fragment is seen as that body's own.
	idents map[string]bool
	// runs is whether the body runs a statement. runIdents are the names it
	// hands a runner as SQL; opaqueRun is a statement the walk cannot read.
	runs, opaqueRun bool
	runIdents       map[string]bool
	// gatedObjects are the object names this body passes to a Require-shaped
	// call. Resolved here rather than left as two facts a reader has to pair
	// up, because pairing them at the body level is what let RequireHuman(ctx)
	// beside an unrelated "relationship" literal vouch for a read.
	//
	// A set rather than one boolean so ONE walk of a package serves every
	// census over it: a function gating `deal` and a function gating `lead`
	// look identical to the parser, and the difference is which name reached
	// the call.
	gatedObjects map[string]bool
}

// packageFunctionReferences parses every non-test source in one package
// directory and returns what each function mentions.
//
// The directory is read and its files parsed one at a time rather than through
// parser.ParseDir, which is deprecated for a reason that would bite here: it
// does not consider build tags when grouping files into packages, and several
// of the directories this walks hold tagged files.
func packageFunctionReferences(t *testing.T, pkg string) packageReferences {
	t.Helper()
	// The path is relative to the module root, which is this test's working
	// directory: package gates sits one below it and TestMain chdirs up.
	dir := filepath.FromSlash(pkg)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s to resolve its object gates: %v", pkg, err)
	}
	// The package's own string constants first: contacts writes
	// auth.Require(ctx, entityLead, …) against `const entityLead = "lead"`, and
	// a walk reading only literals would report the tree's own owning module as
	// ungated. Under-recognition reported as a finding against correct code
	// costs a gate its credibility faster than a miss does.
	parsedFiles := parsePackage(t, pkg, entries, dir)
	consts := gatekit.PackageStringConstants(t, dir)
	out := packageReferences{funcs: map[string][]references{}, fragments: map[string]packageValue{}}
	for _, file := range parsedFiles {
		for fn, decls := range functionBodies(gatekit.ParsedFile{File: file}, consts) {
			out.funcs[fn] = append(out.funcs[fn], decls...)
		}
		collectFragments(file, out.fragments)
	}
	return out
}

// packageReferences is one package's function bodies and its package-level
// const and var values.
type packageReferences struct {
	funcs     map[string][]references
	fragments map[string]packageValue
}

// packageValue is one package-level value: the string literals it holds and the
// names it is built from.
type packageValue struct {
	literals []string
	idents   map[string]bool
}

func collectFragments(file *ast.File, into map[string]packageValue) {
	for _, decl := range file.Decls {
		general, isGeneral := decl.(*ast.GenDecl)
		if !isGeneral {
			continue
		}
		for _, spec := range general.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue {
				continue
			}
			refs := referencesIn(value, nil)
			for _, name := range value.Names {
				into[name.Name] = packageValue{literals: slices.Collect(maps.Keys(refs.literals)), idents: refs.idents}
			}
		}
	}
}

// namingFragments is the declared names plus every fragment built from one.
func (p packageReferences) namingFragments(declared []string) []string {
	names := slices.Clone(declared)
	for grew := true; grew; {
		grew = false
		for name, value := range p.fragments {
			if !slices.Contains(names, name) && slices.ContainsFunc(names, func(n string) bool { return value.idents[n] }) {
				names, grew = append(names, name), true
			}
		}
	}
	return names
}

// statementRunners are the calls that run SQL.
var statementRunners = []string{"Query", "QueryRow", "Exec", "SendBatch"}

// noteStatement records the SQL a runner call is handed. A literal is already
// among the literals, a name may be a fragment, and anything else is unreadable.
// A one-argument Query is url.Values, not SQL.
func (r *references) noteStatement(call *ast.CallExpr, runner string) {
	if !slices.Contains(statementRunners, runner) || len(call.Args) < 2 {
		return
	}
	r.runs = true
	switch sql := call.Args[1].(type) {
	case *ast.BasicLit:
	case *ast.Ident:
		if runner == "SendBatch" {
			r.opaqueRun = true
			return
		}
		r.runIdents[sql.Name] = true
	default:
		r.opaqueRun = true
	}
}

func parsePackage(t *testing.T, pkg string, entries []os.DirEntry, dir string) []*ast.File {
	t.Helper()
	var out []*ast.File
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := gatekit.ParseFile(filepath.Join(dir, name), 0)
		if parseErr != nil {
			t.Fatalf("parsing %s/%s to resolve its object gates: %v", pkg, name, parseErr)
		}
		out = append(out, file)
	}
	return out
}

func functionBodies(parsed gatekit.ParsedFile, consts map[string]string) map[string][]references {
	bodies := map[string][]references{}
	for _, decl := range parsed.File.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc {
			continue
		}
		bodies[fn.Name.Name] = append(bodies[fn.Name.Name], referencesIn(fn, consts))
	}
	return bodies
}

func referencesIn(node ast.Node, consts map[string]string) references {
	refs := references{
		calls: map[string]bool{}, literals: map[string]bool{}, idents: map[string]bool{},
		runIdents: map[string]bool{}, gatedObjects: map[string]bool{},
	}
	ast.Inspect(node, func(n ast.Node) bool {
		switch typed := n.(type) {
		case *ast.CallExpr:
			// calleeName is retentionscope_test.go's, shared rather than
			// respelled: "the called function's own name, ignoring any
			// qualifier" is the same question here, and auth.EdgeReadScope and
			// a local edgeScope both need to resolve to what they call.
			name := calleeName(typed)
			if objectGateCall.MatchString(name) {
				for _, object := range stringArgumentsOf(typed, consts) {
					refs.gatedObjects[object] = true
				}
			}
			if name != "" {
				refs.calls[name] = true
			}
			refs.noteStatement(typed, name)
		case *ast.BasicLit:
			if text, isString := gatekit.LiteralText(typed); isString {
				refs.literals[text] = true
			}
		case *ast.Ident:
			refs.idents[typed.Name] = true
		}
		return true
	})
	return refs
}

// stringArgumentsOf is the object names an object-gate call could be gating.
// Every string argument, because the position differs by spelling —
// auth.Require(ctx, "lead", action) and contact360's requireRead(ctx, "lead")
// — and a census pinned to one index would silently stop seeing the other.
func stringArgumentsOf(call *ast.CallExpr, consts map[string]string) []string {
	var out []string
	for _, arg := range call.Args {
		// FoldStrict: an argument this cannot fully resolve is not an object
		// name, rather than a half-read one. A gate vouching for a read on a
		// fragment it guessed is worse than one that misses it.
		if text, isString := gatekit.StringExpr(arg, consts, gatekit.FoldStrict); isString {
			out = append(out, text)
		}
	}
	return out
}

// fileHoldsAGatedFunction answers for a package-level SQL fragment, which has
// no declaration of its own to ask: the file that declares it is judged as a
// whole, as restrictedreaders_test.go judges one.
func (g objectGate) fileHoldsAGatedFunction(t *testing.T, parsed gatekit.ParsedFile, gated map[string]bool, consts map[string]string) bool {
	t.Helper()
	for name, decls := range functionBodies(parsed, consts) {
		if gated[name] {
			return true
		}
		for _, refs := range decls {
			if g.holdsSeedGate(refs, pkgOf(parsed.Path)) {
				return true
			}
		}
	}
	return false
}

// verdictIn names the declaration a subject sits in, and refuses one sitting in
// two: the verdict IS the declaration, so a subject with two of them has no
// verdict at all.
func verdictIn(t *testing.T, subject string, sets []namedVerdict) string {
	t.Helper()
	var found []string
	for _, set := range sets {
		// A file-keyed verdict answers for every site in the file; a
		// function-keyed one answers for its own site only. Both are asked,
		// because a lifecycle FILE and a deferred FUNCTION are both real shapes.
		if set.waivers.Waived(t, subject) || set.waivers.Waived(t, fileOf(subject)) {
			found = append(found, set.name)
		}
	}
	if len(found) > 1 {
		t.Errorf("%s carries %s verdicts at once: which declaration a read sits in IS its verdict, "+
			"so two of them is none", subject, strings.Join(found, " and "))
		return ""
	}
	if len(found) == 1 {
		return found[0]
	}
	return ""
}

type namedVerdict struct {
	name    string
	waivers *gatekit.Waivers[string]
}

func fileOf(subject string) string {
	if before, _, ok := strings.CutLast(subject, ":"); ok {
		return before
	}
	return subject
}

// constantTable memoises one census's per-package string constants. A census
// asks for a package's table once per file in it, and parsing a package the
// size of contacts a dozen times is the difference between a gate somebody runs
// and one they skip.
//
// Held by the census rather than at package scope, because two censuses over
// this walk run t.Parallel() and a shared map between them is a data race.
type constantTable map[string]map[string]string

func (c constantTable) of(t *testing.T, pkg string) map[string]string {
	t.Helper()
	if known, cached := c[pkg]; cached {
		return known
	}
	c[pkg] = gatekit.PackageStringConstants(t, filepath.FromSlash(pkg))
	return c[pkg]
}
