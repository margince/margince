// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package backendarch

// A comment that says a declaration is the ONLY one of its kind is not
// decoration. It is what stops the next author looking — they grep, they find
// the claim, and they stop. So a false one is worse than silence: silence at
// least leaves the search on.
//
// This tree's own rulebook already says so, in `Reuse before you build`:
// "a comment may not claim to be the only implementation unless a test holds
// it. If no test fails when a second one appears, delete the claim or write the
// test." It has been instruction since #2220, and instruction was not enough —
// a sweep in 2026-08 audited ten such claims exhaustively and **nine were
// false**. Among them:
//
//   - "the only definition of a current employment in this product" — eight
//     compose sites spelled the predicate by hand, which is the exact
//     notice-period bug the comment said it existed to prevent.
//   - "the one spelling every table uses" for a credential handle — there were
//     three, and the two the sweep missed carried ciphertext past a wipe.
//   - "the ONE way a caught failure becomes words on a screen" — five screens
//     bypassed it, and one of them put an RBAC object and verb in front of a
//     user.
//
// None of those was found by a gate. Every one was found by a human re-reading
// a file for some other reason, months later.
//
// THIS GATE DOES NOT AUDIT THE CLAIMS. It cannot: whether a claim is true is a
// question about the whole tree, and there are 641 of them. What it does is
// make the class stop GROWING, which is the half a test can hold:
//
//   - A claim that names its gate is held. `Held by: TestName (path)` in the
//     same doc comment, and this file checks that test exists.
//   - Every other claim in the tree today is in `uniquenessclaims.txt`, which
//     is a DEBT REGISTER and not a permission. It can only shrink: a claim that
//     is not in it and does not name a gate fails, so a NEW claim must be held
//     the day it is written.
//   - An entry that has stopped matching a real claim fails too, so the
//     register tracks the tree rather than rotting beside it.
//
// The register is 638 lines with no reasons, and that is deliberate rather than
// sloppy: a reason per entry would be 638 rationalisations written by somebody
// who has not audited the claim, which is worse than an honest count of what is
// unaudited. The number is the point. It is meant to go down.

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// claimShapes are the ways this tree says "this is the only one".
//
// THE HONEST PART FIRST: this list is hand-written, and it is the one input
// this gate cannot derive. There is no grammar for "a sentence asserting
// uniqueness" the way there is a runtime answer for "which Intl members take a
// locale" — prose is prose. So instead of pretending, the list is held to two
// obligations it CAN meet, both below: every shape must be proven to match a
// real claim taken verbatim from this tree (`claimCorpus`), and every
// near-miss that must NOT match is proven too (the `prose` cases beside it). A shape that
// stops matching anything fails, and a shape widened until it matches ordinary
// prose fails.
//
// What is deliberately OUT, measured rather than guessed: "the one place",
// "the only way", "the one path" and "rather than a second" are ordinary prose
// here far more often than they are claims — 540 hits between them, most of
// them sentences like "the only way to ask the DATABASE" or "one type with one
// branch rather than two authorities". Including them would put ~540 lines of
// noise in a register whose whole value is that its number means something.
// That is a real gap and it is named here rather than left for a reader to
// discover: a claim worded that way is invisible to this gate.
var claimShapes = map[string]*regexp.Regexp{
	"one-of-a-kind": regexp.MustCompile(`(?i)\bthe (?:one|only|single) (?:spelling|writer|reader|caller|definition|implementation|copy|mapping|owner|producer|consumer|declaration|entry point|census|source|home|list|table|gate|function|module)\b`),
	"only-noun":     regexp.MustCompile(`(?i)\bonly (?:writer|reader|caller|definition|spelling|implementation|producer|consumer|entry point|copy|mapping|source|home)\b`),
	"cannot-drift":  regexp.MustCompile(`(?i)\bcannot (?:drift|diverge|come apart|get out of step|disagree|be respelled)\b`),
	"once":          regexp.MustCompile(`(?i)\b(?:spelled|named|written|declared|stated|defined|lives|expressed|answered) (?:exactly )?once\b`),
	"no-second":     regexp.MustCompile(`(?i)\bno (?:second|other) (?:spelling|copy|writer|implementation|mapping|answer|definition|list|table|reader|caller)\b`),
	"one-truth":     regexp.MustCompile(`(?i)\b(?:single|one) source of truth\b`),
	"is-every":      regexp.MustCompile(`\b(?:is|are) EVERY\b`),
	// `is-every-named` has no entry here: it is built per declaration, from the
	// declaration's own name. See namedExhaustiveness.
	"never-twice": regexp.MustCompile(`(?i)\b(?:never|not) (?:duplicated|respelled|re-?implemented|spelled twice|written twice|copied|two)\b`),
}

// namedExhaustiveness matches the claim form the emphatic `is EVERY` shape
// above cannot reach without swallowing ordinary English: a doc comment saying
// the thing it documents is ALL of something.
//
//	readProfileFields is every read of person_profile_field that RENDERS it
//	catalogFilterNames is every key a plan's `filters` object may carry
//	ciGateTargets is every make target invoked by a job the required fan-in
//
// Lower-case `is every` occurs 158 times in this tree, and matching it flat
// also catches "each one is every bit as guarded as the last". The
// discriminator is DERIVED rather than a list of excluded words: Go's own doc
// convention is that a doc comment opens with the identifier it documents, so
// the claim form is `<the declaration's name> is every …`. Ordinary prose does
// not name the declaration and then assert exhaustiveness about it.
//
// This shape is not optional politeness. The worked example the whole rule is
// argued from — `readProfileFields is every read of person_profile_field` — is
// lower-case, so a gate without this arm would be blind to the very claim its
// own docblock cites.
func namedExhaustiveness(decl string) *regexp.Regexp {
	name := decl
	if index := strings.LastIndex(decl, " "); index >= 0 {
		name = decl[index+1:]
	}
	if name == "" || name == "block" {
		return nil
	}
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b\s+(?:is|are) every\b`)
}

// heldBy is the binding a claim carries to say which test holds it. Free text
// around it, because it sits inside a doc comment a human is also reading:
//
//	// Held by: TestEveryFooHasOneWriter (backend/foowriters_test.go)
//
// It may sit ANYWHERE in the doc comment except the first line: revive requires
// a doc comment to open with the identifier it documents, so a binding written
// above that sentence fails the linter rather than this gate. Below the opening
// sentence is where it reads best anyway — the claim first, then what holds it.
//
// The PATH is required and checked. A bare test name would be a claim about a
// name, and this whole file exists because a claim nobody checks is worth less
// than nothing.
var heldBy = regexp.MustCompile(`Held by:\s*(Test[A-Za-z0-9_]*)\s*\(([^)]+)\)`)

// goTestFunc finds the declarations a `Held by:` may name.
var goTestFunc = regexp.MustCompile(`(?m)^func (Test[A-Za-z0-9_]*)\(`)

// claimedTrees are the hand-written Go trees this rule covers. The backend
// module is one tree, not all of them: extensions/, fixtures/, cli/ and
// desktop/ are separate modules a `./...` from here never reaches, and a claim
// made in one is a claim made in this product.
//
// mustHaveClaims is what stops a root passing by scanning nothing — an
// aggregate count cannot, because backend/ alone would carry it forever while a
// unit tree silently escaped the sweep.
//
// Only backend/ carries it, and the rest are named with the reason they cannot.
// Measured when this landed: backend 636, extensions 5, desktop 1, cli 0,
// fixtures 0. A tree with none today is not a tree with none forever — that is
// exactly why it is swept — but requiring extensions/ to hold one would make
// the vanilla lane fail for shipping no units, and requiring desktop/ to hold
// one would make DELETING its single claim a failure rather than the
// improvement it would be. A root that does not exist at all is still caught,
// by the walk error.
type claimedTree struct {
	root           string
	mustHaveClaims bool
}

var claimedTrees = []claimedTree{
	{root: ".", mustHaveClaims: true},
	{root: "../extensions"},
	{root: "../fixtures"},
	{root: "../cli"},
	{root: "../desktop"},
}

// claim is one uniqueness assertion: where it is, what it sits on, and the
// words that make it a claim.
type claim struct {
	path   string // slash-normalised, relative to the repo root
	decl   string // "func Foo", "var bar", "type Baz", "package qux"
	shape  string // which claimShapes entry matched
	phrase string // the matched words, for the failure message
	line   int
	held   string // the test named by `Held by:`, empty when none
	heldIn string // the file that test is claimed to live in
}

// key is what the register is keyed by: the file and the declaration, never the
// LINE. A line number churns on every edit above it, so a register keyed by one
// would be rewritten by changes that have nothing to do with the claim — and a
// register nobody can read a diff of is a register nobody checks. Renaming the
// declaration makes it a new claim, which is correct: a claim that has moved to
// a different subject is a claim somebody should look at again.
func (c claim) key() string { return c.path + "#" + c.decl }

// findClaims parses one tree and returns every uniqueness claim in a
// declaration's DOC comment.
//
// Doc comments only, and that is what makes the census readable rather than
// enormous. A comment inside a function body saying "the one thing to remember"
// is prose about the code below it; a doc comment on a declaration is a
// statement about that declaration, which is the only kind of claim this rule
// is about. Sweeping every comment finds 4308 hits, of which the overwhelming
// majority are ordinary English.
func findClaims(root string) ([]claim, error) {
	var found []claim
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			// node_modules holds a unit's installed dependency tree, and
			// internal/contracts is generated from the OpenAPI document.
			// Neither is hand-written, so neither can carry a claim somebody
			// chose to make.
			if name := entry.Name(); name == "node_modules" || name == "testdata" || name == "contracts" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_gen.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			// A source this module cannot parse is not a clean tree, and
			// skipping it silently is how a gate reads green over code it never
			// saw.
			return fmt.Errorf("parsing %s: %w", path, parseErr)
		}
		rel := filepath.ToSlash(path)
		record := func(doc *ast.CommentGroup, decl string) {
			if doc == nil {
				return
			}
			text := doc.Text()
			shapes := sortedShapes()
			patterns := make(map[string]*regexp.Regexp, len(claimShapes)+1)
			for name, pattern := range claimShapes {
				patterns[name] = pattern
			}
			if named := namedExhaustiveness(decl); named != nil {
				shapes = append(shapes, "is-every-named")
				patterns["is-every-named"] = named
			}
			for _, shape := range shapes {
				phrase := patterns[shape].FindString(text)
				if phrase == "" {
					continue
				}
				c := claim{
					path:   rel,
					decl:   decl,
					shape:  shape,
					phrase: phrase,
					line:   fset.Position(doc.Pos()).Line,
				}
				if binding := heldBy.FindStringSubmatch(text); binding != nil {
					c.held, c.heldIn = binding[1], strings.TrimSpace(binding[2])
				}
				found = append(found, c)
				return
			}
		}
		record(file.Doc, "package "+file.Name.Name)
		for _, decl := range file.Decls {
			switch node := decl.(type) {
			case *ast.FuncDecl:
				record(node.Doc, "func "+node.Name.Name)
			case *ast.GenDecl:
				recordGenDecl(node, record)
			}
		}
		return nil
	})
	return found, err
}

// recordGenDecl reports the claim on a const/var/type declaration, whether the
// doc sits on the declaration or on the single spec inside it.
//
// Both positions, because `// doc\nvar x = 1` and `var (\n// doc\n x = 1\n)`
// are the same statement written two ways and a census that knew one of them
// would be short by whichever the author happened to pick.
func recordGenDecl(node *ast.GenDecl, record func(*ast.CommentGroup, string)) {
	name := func(spec ast.Spec) (string, bool) {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			return "type " + s.Name.Name, true
		case *ast.ValueSpec:
			if len(s.Names) > 0 {
				return node.Tok.String() + " " + s.Names[0].Name, true
			}
		}
		return "", false
	}
	for _, spec := range node.Specs {
		if label, ok := name(spec); ok {
			switch s := spec.(type) {
			case *ast.TypeSpec:
				record(s.Doc, label)
			case *ast.ValueSpec:
				record(s.Doc, label)
			}
		}
	}
	if len(node.Specs) == 1 {
		if label, ok := name(node.Specs[0]); ok {
			record(node.Doc, label)
			return
		}
	}
	record(node.Doc, node.Tok.String()+" block")
}

// sortedShapes gives the shapes a stable order, so a claim matching two of them
// is always attributed to the same one and the register does not churn on a map
// iteration.
func sortedShapes() []string {
	names := make([]string, 0, len(claimShapes))
	for name := range claimShapes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// allClaims sweeps every claimed tree, failing loudly on a root that finds
// nothing it was supposed to.
func allClaims(t *testing.T) []claim {
	t.Helper()
	var all []claim
	for _, tree := range claimedTrees {
		found, err := findClaims(tree.root)
		if err != nil {
			t.Fatalf("walking %s: %v", tree.root, err)
		}
		if tree.mustHaveClaims && len(found) == 0 {
			t.Fatalf("%s holds no uniqueness claim — a root that finds nothing passes exactly "+
				"like a clean one, so either the root is stale or the tree has moved", tree.root)
		}
		all = append(all, found...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].key() < all[j].key() })
	return all
}

const registerPath = "uniquenessclaims.txt"

// readRegister returns the debt register's keys, in file order, ignoring blank
// lines and the header.
func readRegister(t *testing.T) []string {
	t.Helper()
	file, err := os.Open(registerPath)
	if err != nil {
		t.Fatalf("opening %s: %v", registerPath, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			t.Errorf("closing %s: %v", registerPath, closeErr)
		}
	}()
	var keys []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		keys = append(keys, line)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("reading %s: %v", registerPath, err)
	}
	return keys
}

// testFunctions returns every `func TestX` in the repo, keyed by name, with the
// files it is declared in. A `Held by:` is checked against this rather than
// against a path the author typed, so a rename that leaves the binding behind
// is a failure rather than a comment nobody reads.
func testFunctions(t *testing.T) map[string][]string {
	t.Helper()
	found := map[string][]string{}
	for _, tree := range claimedTrees {
		err := filepath.WalkDir(tree.root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if name := entry.Name(); name == "node_modules" || name == "testdata" {
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, "_test.go") {
				return nil
			}
			source, readErr := os.ReadFile(path) // #nosec G304 -- a *_test.go path from walking the trusted source tree
			if readErr != nil {
				return readErr
			}
			for _, match := range goTestFunc.FindAllStringSubmatch(string(source), -1) {
				found[match[1]] = append(found[match[1]], filepath.ToSlash(path))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s for test functions: %v", tree.root, err)
		}
	}
	return found
}

func TestEveryUniquenessClaimNamesItsGateOrIsRegisteredDebt(t *testing.T) {
	claims := allClaims(t)
	registered := map[string]bool{}
	for _, key := range readRegister(t) {
		registered[key] = true
	}
	var loose []string
	for _, c := range claims {
		if c.held != "" || registered[c.key()] {
			continue
		}
		loose = append(loose, fmt.Sprintf("%s:%d %s — %q (%s)", c.path, c.line, c.decl, c.phrase, c.shape))
	}
	if len(loose) > 0 {
		t.Errorf("%d uniqueness claim(s) neither name a gate nor sit in the debt register.\n\n"+
			"A comment saying a declaration is the only one of its kind is what stops the next author "+
			"looking, so a false one is worse than silence — nine of the ten audited in this tree were "+
			"false. Write the test that fails when a second appears and name it in the doc comment:\n\n"+
			"\t// Held by: TestSomethingHasOneWriter (backend/somethingwriters_test.go)\n\n"+
			"or delete the claim. The register is CLOSED to new entries: it records what was already "+
			"here when this gate landed, and it only shrinks.\n\n\t%s",
			len(loose), strings.Join(loose, "\n\t"))
	}
}

func TestANamedGateExistsAndLivesWhereTheClaimSaysItDoes(t *testing.T) {
	claims := allClaims(t)
	tests := testFunctions(t)
	held := 0
	for _, c := range claims {
		if c.held == "" {
			continue
		}
		held++
		files, ok := tests[c.held]
		if !ok {
			t.Errorf("%s:%d %s names %s as the test that holds it, and no such test function exists",
				c.path, c.line, c.decl, c.held)
			continue
		}
		if !slicesContainsSuffix(files, c.heldIn) {
			t.Errorf("%s:%d %s names %s in %s; that test is declared in %s",
				c.path, c.line, c.decl, c.held, c.heldIn, strings.Join(files, ", "))
		}
	}
	// A gate that judges nothing passes exactly like one that judges a clean
	// tree. The seed claims this change holds are what make this arm mean
	// something on the day it lands.
	if held == 0 {
		t.Error("no claim in the tree names a gate — this arm judged nothing, which is " +
			"indistinguishable from every binding being correct")
	}
}

func slicesContainsSuffix(paths []string, want string) bool {
	want = strings.TrimPrefix(filepath.ToSlash(want), "./")
	for _, path := range paths {
		if strings.HasSuffix(path, want) || strings.HasSuffix("backend/"+path, want) {
			return true
		}
	}
	return false
}

func TestTheRegisterHoldsNoEntryThatIsNoLongerAClaim(t *testing.T) {
	claims := allClaims(t)
	live := map[string]bool{}
	for _, c := range claims {
		// A claim that now names its gate is no longer debt: its register entry
		// has to go, or the count stops meaning what it says.
		if c.held == "" {
			live[c.key()] = true
		}
	}
	var idle []string
	for _, key := range readRegister(t) {
		if !live[key] {
			idle = append(idle, key)
		}
	}
	if len(idle) > 0 {
		t.Errorf("%d register entry/entries no longer describe an unheld claim — delete them.\n\n"+
			"An entry outliving its claim is a standing permission nobody needs, and the next claim "+
			"written on that declaration inherits it. This is also how the number goes DOWN: hold a "+
			"claim with a test, or delete the claim, then remove its line here.\n\n\t%s",
			len(idle), strings.Join(idle, "\n\t"))
	}
}

func TestTheRegisterIsSortedAndFreeOfDuplicates(t *testing.T) {
	keys := readRegister(t)
	if len(keys) == 0 {
		t.Fatal("the register is empty — either every claim in the tree is held (in which case " +
			"delete this file and the arms that read it) or the reader is broken")
	}
	seen := map[string]bool{}
	for i, key := range keys {
		if seen[key] {
			t.Errorf("%s is registered twice — one debt, one line", key)
		}
		seen[key] = true
		// Sorted, so a diff of this file is readable and two people adding the
		// last removals never conflict on the same line for no reason.
		if i > 0 && keys[i-1] > key {
			t.Errorf("the register is out of order at line %d: %q sorts before %q", i+1, key, keys[i-1])
		}
	}
}

// TestTheDetectorMatchesRealClaimsAndLeavesProseAlone is the arm that makes the
// hand-written shape list honest.
//
// Every entry in `claimShapes` is proven against a sentence taken VERBATIM from
// this tree, so a shape cannot be added speculatively and cannot quietly stop
// matching. And every near-miss is proven not to match, so a shape cannot be
// widened until it swallows ordinary English — which is the failure that turns
// a census into noise and gets it deleted.
func TestTheDetectorMatchesRealClaimsAndLeavesProseAlone(t *testing.T) {
	claimCorpus := map[string]string{
		"one-of-a-kind": "values.NewMoney is the ONE spelling of a valid ISO-4217 code in this product.",
		"only-noun":     "companyform.go is the only writer a human drives, and it re-states all four obligations.",
		"cannot-drift":  "report.go and orgrollup.go compute the same weighted value and cannot drift apart.",
		"once":          "The rule is spelled once here rather than twice, for exactly two callers.",
		"no-second":     "signalTone is exported so there is no second mapping that could drift.",
		"one-truth":     "backend/api/crm.yaml is the single source of truth for the wire shape.",
		"is-every":      "catalogFilterNames is EVERY key a plan's `filters` object may carry.",
		"never-twice":   "The precondition is never duplicated: ifMatch is the one call that sets it.",
	}
	for shape, sentence := range claimCorpus {
		pattern, ok := claimShapes[shape]
		if !ok {
			t.Errorf("the corpus names shape %q, which claimShapes does not carry — one of the two is stale", shape)
			continue
		}
		if !pattern.MatchString(sentence) {
			t.Errorf("shape %q no longer matches the real claim it was written for:\n\t%s", shape, sentence)
		}
	}
	for shape := range claimShapes {
		if _, ok := claimCorpus[shape]; !ok {
			t.Errorf("shape %q has no corpus case — a shape nobody proved is a shape that may match "+
				"nothing, and a detector matching nothing reports a clean tree", shape)
		}
	}
	// Ordinary English that must NOT read as a claim. Each of these is a real
	// sentence shape from this tree; a detector that reported them would be
	// switched off within a week, which is the way a census actually dies.
	prose := []string{
		"That is the only way to ask the DATABASE whether it refuses a duplicate scope.",
		"The ledger is the only place a version and its name are recorded together.",
		"One type with one branch rather than two authorities: a mailbox is one human's.",
		"An identical step is a no-op rather than a second audit row.",
		"The one path a reader takes through this screen starts at the header.",
		"There is only one thing to remember about the cursor.",
		"Every read is checked, and each one is every bit as guarded as the last.",
	}
	for _, sentence := range prose {
		for shape, pattern := range claimShapes {
			if phrase := pattern.FindString(sentence); phrase != "" {
				t.Errorf("shape %q reads ordinary prose as a claim (%q in):\n\t%s", shape, phrase, sentence)
			}
		}
	}
}

// TestTheBindingIsReadOffTheCommentRatherThanGuessedAt proves the `Held by:`
// parser on the shapes an author will actually write, including the ones that
// must NOT bind.
func TestTheBindingIsReadOffTheCommentRatherThanGuessedAt(t *testing.T) {
	binds := []struct {
		comment, test, file string
	}{
		{"Held by: TestFoo (backend/foo_test.go)", "TestFoo", "backend/foo_test.go"},
		{"prose above\n// Held by:  TestBarBaz  ( backend/bar_test.go )\nmore prose", "TestBarBaz", "backend/bar_test.go"},
		{"Held by: TestX (cli/craft/x_test.go) and see also the sibling", "TestX", "cli/craft/x_test.go"},
	}
	for _, c := range binds {
		match := heldBy.FindStringSubmatch(c.comment)
		if match == nil {
			t.Errorf("no binding read from %q", c.comment)
			continue
		}
		if match[1] != c.test || strings.TrimSpace(match[2]) != c.file {
			t.Errorf("binding read as (%q, %q), want (%q, %q)", match[1], strings.TrimSpace(match[2]), c.test, c.file)
		}
	}
	// A binding with no file is not a binding. A test NAME alone is a claim
	// about a name, and this whole file exists because a claim nobody checks is
	// worth less than nothing.
	for _, bad := range []string{
		"Held by: TestFoo",
		"Held by: notATest (backend/foo_test.go)",
		"held by the reviewer's judgement",
	} {
		if heldBy.MatchString(bad) {
			t.Errorf("%q was read as a binding and must not be", bad)
		}
	}
}
