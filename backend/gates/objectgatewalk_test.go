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
	"go/parser"
	"go/token"
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

// requireCall matches a call that asks the object gate under any of this
// tree's spellings — auth.Require, or a package-local wrapper such as
// contact360's requireRead. Paired with the object name in the same CALL, it
// is the older form of the admission and still counts.
var requireCall = regexp.MustCompile(`^[Rr]equire[A-Za-z]*$`)

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
		})
	}
	return sites
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

// gatedFunctionsByPackage resolves, per package directory, every function that
// reaches the object's gate — directly, or through another function in the
// same package. Transitive because this tree routinely splits a read across the
// function holding the SQL and the helper building its predicate (company360's
// edgeScope, meetingbrief's seatJoinPredicate), and a gate asking only about
// direct calls would report the reader red while its admission sits in the file
// next door.
func (g objectGate) gatedFunctionsByPackage(t *testing.T, files []gatekit.ParsedFile) map[string]map[string]bool {
	t.Helper()
	// The WHOLE package, not only the files that read the table. The admission
	// this tree writes routinely lives in a sibling file that holds no SQL of
	// its own — company360's edgeScope sits in sections.go while the three reads it
	// gates are in graphreads.go and contacts.go — so a resolution seeded from
	// the subject files alone reports gated code as ungated, which costs the
	// gate its credibility faster than a miss does.
	bodies := map[string]map[string][]references{}
	for _, parsed := range files {
		pkg := path.Dir(parsed.Path)
		if bodies[pkg] != nil {
			continue
		}
		bodies[pkg] = packageFunctionReferences(t, pkg)
	}

	// A name is gated when ANY declaration spelling it is — a union, never an
	// overwrite. Optimistic where two receivers share a method name, and
	// deliberately so: this index only ever answers "is there a gated helper
	// called X in this package", and the site's own declaration has already
	// been asked directly, so the optimism cannot excuse an ungated read whose
	// same-named neighbour happens to be gated.
	gated := map[string]map[string]bool{}
	for pkg, funcs := range bodies {
		gated[pkg] = map[string]bool{}
		for name, decls := range funcs {
			for _, refs := range decls {
				if g.holdsSeedGate(refs, pkg) {
					gated[pkg][name] = true
					break
				}
			}
		}
		for grew := true; grew; {
			grew = false
			for name, decls := range funcs {
				if gated[pkg][name] {
					continue
				}
				for _, refs := range decls {
					for callee := range gated[pkg] {
						if refs.calls[callee] {
							gated[pkg][name] = true
							grew = true
							break
						}
					}
					if gated[pkg][name] {
						break
					}
				}
			}
		}
	}
	return gated
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
	if refs.gatedObjects[g.object] {
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
func packageFunctionReferences(t *testing.T, pkg string) map[string][]references {
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
	refs := map[string][]references{}
	for _, file := range parsedFiles {
		for fn, decls := range functionBodies(gatekit.ParsedFile{File: file}, consts) {
			refs[fn] = append(refs[fn], decls...)
		}
	}
	return refs
}

func parsePackage(t *testing.T, pkg string, entries []os.DirEntry, dir string) []*ast.File {
	t.Helper()
	fset := token.NewFileSet()
	var out []*ast.File
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
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
	refs := references{calls: map[string]bool{}, literals: map[string]bool{}, gatedObjects: map[string]bool{}}
	ast.Inspect(node, func(n ast.Node) bool {
		switch typed := n.(type) {
		case *ast.CallExpr:
			// calleeName is retentionscope_test.go's, shared rather than
			// respelled: "the called function's own name, ignoring any
			// qualifier" is the same question here, and auth.EdgeReadScope and
			// a local edgeScope both need to resolve to what they call.
			name := calleeName(typed)
			if requireCall.MatchString(name) {
				for _, object := range stringArgumentsOf(typed, consts) {
					refs.gatedObjects[object] = true
				}
			}
			if name != "" {
				refs.calls[name] = true
			}
		case *ast.BasicLit:
			if text, isString := gatekit.LiteralText(typed); isString {
				refs.literals[text] = true
			}
		}
		return true
	})
	return refs
}

// stringArgumentsOf is the object names a Require-shaped call could be gating.
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
	if idx := strings.LastIndex(subject, ":"); idx >= 0 {
		return subject[:idx]
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
