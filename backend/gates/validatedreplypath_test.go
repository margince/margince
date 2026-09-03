// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A model reply this tree can REFUSE must be asked for through the validated
// lane, so the refusal reaches the model that can act on it.
//
// `CompleteValidated` re-asks with the validator's own message appended
// (`modules/ai` §5.2). A site that refuses a reply from the plain `Complete`
// lane instead has no second attempt and no error to surface: it falls back to
// its deterministic floor, records provenance as `Deterministic`, and logs
// nothing. Every obedient model then fails IDENTICALLY, so the lane is dead for
// every user on every request with nothing red in the tree. That is not a
// hypothetical failure mode — two sites were measured refusing 0-of-6
// certification runs before anybody noticed, and both were sites without
// re-ask.
//
// The rule this holds is narrower than "every model call is validated", and the
// narrowness is the point: most lanes here write PROSE, accept whatever comes
// back, and cannot exhibit the failure at all. Only a site whose reply passes
// through a parse that can REFUSE it is owed a retry.
//
// H2 honesty. The walk resolves a reply's reader by name, within the call
// site's own package or through the file's imports. A reader reached through an
// interface, a stored function field or a closure is invisible to it, and this
// gate does not claim those routes carry nothing — it claims that every route
// it CAN see is either validated or ratified below.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// plainLaneReaders ratifies each site that reads a refusable model reply and
// still sends through the plain lane. A waiver rather than a comment because a
// site that adopts the validated path must stop being listed here, and nothing
// else would notice that it had.
var plainLaneReaders = gatekit.Waive(map[string]string{
	// The certification harness measures what a model does UNASSISTED. A retry
	// here would report the second answer as the first, which is the one thing
	// a certification run must never do.
	"internal/compose/certcase_replydraft.go: replyDraftRecorder.Complete": "the certification recorder, which must send bare: a re-ask would record a corrected reply as the model's own first answer and every certified score would be of a lane nobody runs",
	// The agent loop re-asks through its window rather than through §5.2: the
	// parse error goes back as an observation and the next turn is the retry.
	"internal/modules/agents/runner/runner.go: Runner.loop": "the agent loop IS the re-ask — parseStep's error goes back to the model as an observation (observeThen, outputValidatorSource) and the loop asks again, giving up only after consecutiveInvalidLimit consecutive refusals",
})

// laneComplete is the lane method a site calls to obtain one model reply, and
// laneCompleteValidated is the same call with the §5.2 retry policy behind it.
const (
	laneComplete          = "Complete"
	laneCompleteValidated = "CompleteValidated"
	replyTextField        = "Text"
	askHelper             = "Ask"
	aiImportPath          = "github.com/margince/margince/backend/internal/modules/ai"
)

func TestARefusableModelReplyIsAskedThroughTheValidatedLane(t *testing.T) {
	t.Parallel()
	defer plainLaneReaders.AssertAllMatched(t)
	scope := gatekit.Scope{
		Roots: []string{"internal"},
		Subject: func(_ string, file *ast.File) bool {
			return len(modelReplySites(file)) > 0 || askCallsIn(file) > 0
		},
		Exempt: gatekit.Waive(map[string]string{}),
	}
	packages := newPackageIndex()
	seen := map[string]bool{}
	plain, asked := 0, 0
	for _, src := range scope.Files(t) {
		asked += askCallsIn(src.File)
		for _, site := range modelReplySites(src.File) {
			plain++
			reader, refuses := site.refusingReader(t, packages, src)
			if !refuses || site.validated {
				continue
			}
			// Keyed by path AND function: writeWithModel is the spelling in
			// three different packages, and a bare-name waiver would ratify
			// all of them from one entry nobody meant to be that wide.
			where := src.Path + ": " + site.name
			if seen[where] {
				continue
			}
			seen[where] = true
			if !plainLaneReaders.Waived(t, where) {
				t.Errorf("%s takes its reply from the plain %s lane and hands it to %s, which can refuse "+
					"it. A refusal there is silent — the site degrades to its floor and no model is ever told "+
					"why — so ask through %s with the site's own read as the validator, or ratify it in "+
					"plainLaneReaders with the reason its reply cannot be refused",
					where, laneComplete, reader, laneCompleteValidated)
			}
		}
	}
	// Under-recognition is the one way this gate must not break: a walk that
	// resolves nothing reports PASS over a tree it never read.
	//
	// The floor is on the TOTAL population — sites that ask bare plus sites that
	// ask through ai.Ask — because adopting a site MOVES it between those two
	// counts rather than removing it. A floor on the bare count alone would fall
	// as the tree got better and have to be lowered each time, which is a floor
	// that tracks the tree instead of guarding it.
	//
	// That the reader resolution still works is proved by this file's own
	// detector tests rather than by a finding, since a tree with nothing left to
	// find would otherwise have to keep one defect to stay honest.
	if plain+asked < 30 {
		t.Errorf("only %d model reply site(s) were found (%d bare, %d through %s) — this tree has many "+
			"more, so the walk has stopped recognising the call it judges rather than the tree having "+
			"lost them", plain+asked, plain, asked, askHelper)
	}
}

// askCallsIn counts the sites that take their reply through the shared dispatch.
func askCallsIn(file *ast.File) int {
	qualifier, dotImported := gatekit.ImportedAs(file, aiImportPath)
	inPackage := file.Name != nil && file.Name.Name == path.Base(aiImportPath)
	if qualifier == "" && !dotImported && !inPackage {
		return 0
	}
	calls := 0
	ast.Inspect(file, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			if (dotImported || inPackage) && fun.Name == askHelper {
				calls++
			}
		case *ast.SelectorExpr:
			base, isIdent := fun.X.(*ast.Ident)
			if isIdent && base.Name == qualifier && fun.Sel.Name == askHelper {
				calls++
			}
		}
		return true
	})
	return calls
}

// replySite is one function that obtains a model reply: the identifier the
// reply lands in, whether the function also reaches the validated lane, and the
// calls it hands the reply's text to.
type replySite struct {
	name      string
	reply     string
	validated bool
	readers   []*ast.CallExpr
	returns   bool
	file      *ast.File
}

// modelReplySites finds every function in one file that takes a model reply.
//
// A call is a model reply site when its result's `.Text` is read in the same
// function. That is what makes the value a model.Response rather than one of
// the several other things in this tree with a Complete method, and it is the
// same fact the site needs anyway, so a site cannot hide from the walk without
// ceasing to read its reply.
func modelReplySites(file *ast.File) []replySite {
	var sites []replySite
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || fn.Body == nil {
			continue
		}
		for _, reply := range replyIdentsIn(fn.Body) {
			readers, returns := readersOfReply(fn.Body, reply)
			if len(readers) == 0 {
				continue
			}
			sites = append(sites, replySite{
				name:      qualifiedFuncName(fn),
				reply:     reply,
				validated: callsSelector(fn.Body, laneCompleteValidated),
				readers:   readers,
				returns:   returns && resultsCarryError(fn.Type),
				file:      file,
			})
		}
	}
	return sites
}

// replyIdentsIn names the variables a plain Complete call assigns its reply to.
func replyIdentsIn(body *ast.BlockStmt) []string {
	var replies []string
	ast.Inspect(body, func(n ast.Node) bool {
		var lhs []ast.Expr
		var rhs []ast.Expr
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			lhs, rhs = stmt.Lhs, stmt.Rhs
		default:
			return true
		}
		if len(rhs) != 1 || len(lhs) == 0 {
			return true
		}
		if !isSelectorCall(rhs[0], laneComplete) {
			return true
		}
		ident, isIdent := lhs[0].(*ast.Ident)
		if isIdent && ident.Name != "_" && !slices.Contains(replies, ident.Name) {
			// A function that re-asks assigns twice to the same name. The
			// obligation is the function's, not the attempt's, so naming it
			// once keeps the report a list of sites rather than of statements.
			replies = append(replies, ident.Name)
		}
		return true
	})
	return replies
}

// readersOfReply collects the calls that are handed the reply's text, and says
// whether one of them is returned directly. Both shapes appear in this tree:
// `kept, err := Parse(resp.Text, …)` and `return Parse(reply.Text, …)`.
func readersOfReply(body *ast.BlockStmt, reply string) ([]*ast.CallExpr, bool) {
	var readers []*ast.CallExpr
	returned := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall || !readsReplyText(call, reply) {
			return true
		}
		readers = append(readers, call)
		return true
	})
	ast.Inspect(body, func(n ast.Node) bool {
		ret, isReturn := n.(*ast.ReturnStmt)
		if !isReturn {
			return true
		}
		for _, result := range ret.Results {
			if call, isCall := result.(*ast.CallExpr); isCall && readsReplyText(call, reply) {
				returned = true
			}
		}
		return true
	})
	return readers, returned
}

// readsReplyText reports whether a call is handed `<reply>.Text`, at any depth
// inside one of its arguments — `Parse(ai.Unfence(resp.Text))` reads it too.
func readsReplyText(call *ast.CallExpr, reply string) bool {
	found := false
	for _, arg := range call.Args {
		ast.Inspect(arg, func(n ast.Node) bool {
			sel, isSel := n.(*ast.SelectorExpr)
			if !isSel || sel.Sel.Name != replyTextField {
				return true
			}
			if base, isIdent := sel.X.(*ast.Ident); isIdent && base.Name == reply {
				found = true
			}
			return true
		})
	}
	return found
}

// refusingReader names the first reader of this reply that can refuse it, by
// resolving the callee's declaration and asking whether it returns an error.
//
// A reader returned directly from a function that itself returns an error
// counts without a lookup: the refusal is the caller's own result either way.
func (s replySite) refusingReader(t *testing.T, packages *packageIndex, src gatekit.ParsedFile) (string, bool) {
	t.Helper()
	for _, call := range s.readers {
		name, importPath, resolvable := calleeOf(call, s.file)
		if !resolvable {
			continue
		}
		if packages.returnsError(t, src.Path, importPath, name) {
			return name, true
		}
	}
	if s.returns && len(s.readers) > 0 {
		return "its own result", true
	}
	return "", false
}

// calleeOf names a call's target and the package it lives in. An empty import
// path means the call site's own package. A callee that is not a plain name or
// a package-qualified one — a method on a value, a closure — is unresolvable,
// which is the H2 limit this gate declares rather than hides.
func calleeOf(call *ast.CallExpr, file *ast.File) (name, importPath string, resolvable bool) {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name, "", true
	case *ast.SelectorExpr:
		base, isIdent := fun.X.(*ast.Ident)
		if !isIdent {
			return "", "", false
		}
		for _, spec := range file.Imports {
			quoted, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				continue
			}
			alias := path.Base(quoted)
			if spec.Name != nil {
				alias = spec.Name.Name
			}
			if alias == base.Name {
				return fun.Sel.Name, quoted, true
			}
		}
		return "", "", false
	default:
		return "", "", false
	}
}

// packageIndex answers "does this package's function return an error", parsing
// each package directory once. It is a cache rather than a whole-module index
// because a reply's reader is nearly always in the site's own package, and
// parsing the module for every gate run to answer a dozen questions would be
// paid by every check.
type packageIndex struct {
	byDir map[string]map[string]bool
}

func newPackageIndex() *packageIndex {
	return &packageIndex{byDir: map[string]map[string]bool{}}
}

func (p *packageIndex) returnsError(t *testing.T, sitePath, importPath, name string) bool {
	t.Helper()
	dir := path.Dir(sitePath)
	if importPath != "" {
		inModule, isLocal := strings.CutPrefix(importPath, modulePath+"/")
		if !isLocal {
			// A dependency's parse is not this tree's to judge, and a reply
			// handed to one is reported by the direct-return arm instead.
			return false
		}
		dir = inModule
	}
	decls, known := p.byDir[dir]
	if !known {
		decls = errorReturnersIn(t, dir)
		p.byDir[dir] = decls
	}
	return decls[name]
}

// errorReturnersIn indexes one package directory: every top-level function and
// method whose results include an error.
func errorReturnersIn(t *testing.T, dir string) map[string]bool {
	t.Helper()
	sources, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("listing %s: %v", dir, err)
	}
	decls := map[string]bool{}
	fset := token.NewFileSet()
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, source, nil, 0)
		if parseErr != nil {
			t.Fatalf("parsing %s: %v", source, parseErr)
		}
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if isFunc && resultsCarryError(fn.Type) {
				decls[fn.Name.Name] = true
			}
		}
	}
	return decls
}

// resultsCarryError reports whether a signature returns an error.
func resultsCarryError(sig *ast.FuncType) bool {
	if sig.Results == nil {
		return false
	}
	for _, result := range sig.Results.List {
		if ident, isIdent := result.Type.(*ast.Ident); isIdent && ident.Name == "error" {
			return true
		}
	}
	return false
}

// isSelectorCall reports whether an expression is a call to a named method.
func isSelectorCall(expr ast.Expr, method string) bool {
	call, isCall := expr.(*ast.CallExpr)
	if !isCall {
		return false
	}
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	return isSel && sel.Sel.Name == method
}

// callsSelector reports whether a body calls a named method anywhere.
func callsSelector(body *ast.BlockStmt, method string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		expr, isExpr := n.(ast.Expr)
		if isExpr && isSelectorCall(expr, method) {
			found = true
		}
		return !found
	})
	return found
}

// The walk above, tested against source it is handed rather than against the
// tree.
//
// A census proves itself by finding something, and this one now finds nothing:
// every site it judges has been adopted. So the evidence that it still WORKS
// cannot come from its own output — keeping one defect back to keep the gate
// honest is a worse trade than these cases. Each is a shape the tree held
// before adoption, plus the shapes that must stay unflagged.

func TestTheWalkSeesAReplyRefusedByAParse(t *testing.T) {
	t.Parallel()
	reader, refuses := judgeSource(t, `package site

func writeWithModel(ctx context.Context, lane Completer) ([]Section, error) {
	resp, err := lane.Complete(ctx, BriefRequest())
	if err != nil {
		return nil, err
	}
	return ParseBriefSections(resp.Text)
}

func ParseBriefSections(text string) ([]Section, error) { return nil, nil }
`)
	if !refuses {
		t.Fatal("a reply handed to a parse that returns an error was not seen as refusable, so every site " +
			"this gate exists for would pass unreported")
	}
	if reader != "ParseBriefSections" {
		t.Errorf("the refusing reader was named %q, want ParseBriefSections", reader)
	}
}

// A prose site accepts whatever comes back. It must NOT be flagged: a gate that
// reported these would carry a register longer than its rule, and a register
// that long is read by nobody.
func TestTheWalkLeavesAProseSiteAlone(t *testing.T) {
	t.Parallel()
	_, refuses := judgeSource(t, `package site

func summarize(ctx context.Context, lane Completer) string {
	resp, err := lane.Complete(ctx, SummaryRequest())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(resp.Text)
}
`)
	if refuses {
		t.Error("a site that only trims its reply was reported as refusing it, so the gate would demand a " +
			"retry for a lane that accepts everything")
	}
}

// The reply returned straight out of a function that itself returns an error is
// the same refusal one step out, and meetingbrief spelled it that way.
func TestTheWalkSeesARefusalReturnedDirectly(t *testing.T) {
	t.Parallel()
	_, refuses := judgeSource(t, `package site

func writePlanWithModel(ctx context.Context, lane Completer) (Plan, error) {
	reply, err := lane.Complete(ctx, PlanRequest())
	if err != nil {
		return Plan{}, err
	}
	return parseElsewhere(reply.Text)
}
`)
	if !refuses {
		t.Error("a reply returned through a reader this package cannot resolve, from a function that " +
			"returns an error, was not seen as refusable — the caller's own result IS the refusal")
	}
}

// A site that already asks through the validated lane is not a finding, however
// hard its parse refuses.
func TestTheWalkLeavesAValidatedSiteAlone(t *testing.T) {
	t.Parallel()
	site := singleSite(t, `package site

func extract(ctx context.Context, brain Brain) ([]Row, error) {
	resp, err := brain.CompleteValidated(ctx, req, shapeValid)
	if err != nil {
		return nil, err
	}
	return ParseRows(resp.Text)
}

func ParseRows(text string) ([]Row, error) { return nil, nil }
`)
	if site != nil {
		t.Errorf("a CompleteValidated call was read as a bare reply site (%s), so adopting the validated "+
			"path would not clear a finding", site.name)
	}
}

// judgeSource runs the whole walk over one synthetic package: parse, find the
// reply site, resolve its reader. It writes the source to a directory because
// the reader lookup reads the package off disk, which is the path production
// takes — a lookup stubbed here would test a copy of it.
func judgeSource(t *testing.T, source string) (string, bool) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "site.go")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parsing the fixture: %v", err)
	}
	sites := modelReplySites(file)
	if len(sites) != 1 {
		t.Fatalf("the fixture holds %d reply site(s), want exactly one", len(sites))
	}
	return sites[0].refusingReader(t, newPackageIndex(), gatekit.ParsedFile{Path: path, File: file})
}

// singleSite returns the one reply site in a fixture, or nil when the walk found
// none — which is itself the answer some cases assert.
func singleSite(t *testing.T, source string) *replySite {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "site.go", source, 0)
	if err != nil {
		t.Fatalf("parsing the fixture: %v", err)
	}
	sites := modelReplySites(file)
	if len(sites) == 0 {
		return nil
	}
	return &sites[0]
}
