// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind claim H1

package gates

// A comment that says a declaration is the ONLY one of its kind is not
// decoration. It is what stops the next author looking — they grep, they find
// the claim, and they stop. So a false one is worse than silence: silence at
// least leaves the search on.
//
// This tree's own rulebook already says so, in `Reuse before you build`:
// "a comment may not claim to be the only implementation unless a test holds
// it. If no test fails when a second one appears, delete the claim or write the
// test." That has been instruction for a while, and instruction was not enough:
// a sweep audited ten such claims exhaustively and NINE were false. Among
// them:
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
// The class is broader than prose, and the sharpest example is one rung more
// concrete: an e2e spec asserted the relink search box reads "Person,
// Organisation, Deal, Lead oder Projekt suchen". Projects joined the searchable
// kinds, the string grew a fifth, and nothing derived either the sentence or
// the spec's copy of it from the set they both describe. It surfaced five
// merges later, in an unrelated lane.
//
// So the rule below is about claims that nothing HOLDS, not about comments
// specifically: a literal asserting a set's contents with no deriver is the
// same defect as a uniqueness comment with no test behind it.
//
// THIS GATE DOES NOT AUDIT THE CLAIMS. It cannot: whether a claim is true is a
// question about the whole tree, and the tree holds hundreds of them. What it
// does is make the class stop GROWING, which is the half a test can hold:
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
// CLOSED TO NEW CLAIMS, NOT TO A BETTER DETECTOR. A shape that learns to see a
// wording it used to miss finds claims that were already there, and the
// register grows to hold them; "closed" would otherwise mean the detector may
// never improve, which is the opposite of the point. `shapeCensus` is the line
// where that growth is agreed to, one row per shape.
//
// The rows are per shape rather than one total because the number falls two
// ways and only one of them is work. A claim leaves because it was held or
// deleted, or because the detector stopped seeing it — and the second costs
// nothing, changes no code a reader would notice, and reads exactly like the
// first. Priced per shape, looking away has to say which shape stopped looking
// and what it cost.
//
// The register carries no per-entry reasons, and that is deliberate rather than
// sloppy: a reason per entry would be six hundred rationalisations written by
// somebody who has not audited the claim, which is worse than an honest count
// of what is unaudited. The number is the point, it is meant to go down, and it
// is not restated here: a second hand-written copy of a derived total is the
// thing shapeCensus exists to refuse.

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

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

// gateFiles are this gate's own sources, by PATH relative to the backend root.
// Both halves spell out the phrases they hunt for, so a sweep that read either
// would register its own prose as debt.
//
// It answers TWO questions, which happen to have the same answer and are not
// the same question: which files the sweep skips, and which files a `Held by:`
// may not name. The second is because these arms judge the register rather than
// any claim's subject. A gate arm added in a file of its own is covered by
// neither until it is named here — and `exemptGateFiles` pins how many there
// are, so adding one is a line somebody agrees to rather than a skip that
// arrives with a refactor.
//
// By path and not by basename: a basename match would skip every file so named
// in every swept tree, and a nested one could then carry an unbound claim.
// Named rather than derived from the runtime, because runtime.Caller inside a
// walk is a worse kind of clever than a constant a reader can check against the
// file they are in.
var gateFiles = map[string]bool{
	gateDir + "/uniquenessclaims_test.go":         true,
	gateDir + "/uniquenessclaimsdetector_test.go": true,
	gateDir + "/uniquenessclaimscorpus_test.go":   true,
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
				if unauthoredDir(entry.Name()) {
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
	t.Parallel()
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
			"\t// Held by: TestSomethingHasOneWriter (backend/gates/somethingwriters_test.go)\n\n"+
			"or delete the claim. The register is CLOSED to new entries: it records what was already "+
			"here when this gate landed, and it only shrinks.\n\n\t%s",
			len(loose), strings.Join(loose, "\n\t"))
	}
}

func TestANamedGateExistsAndLivesWhereTheClaimSaysItDoes(t *testing.T) {
	t.Parallel()
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
		if !namesTheFile(files, c.heldIn) {
			t.Errorf("%s:%d %s names %s in %s; that test is declared in %s",
				c.path, c.line, c.decl, c.held, c.heldIn, strings.Join(files, ", "))
			continue
		}
		if file, isArm := declaredInAGateArm(files); isArm {
			t.Errorf("%s:%d %s names %s, which is one of this gate's own arms in %s.\n\n"+
				"Those arms judge the register, not the claim's subject, so the binding "+
				"would hold nothing. Name the test that fails when a second implementation "+
				"appears, or leave the claim in the register until one exists.",
				c.path, c.line, c.decl, c.held, file)
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

// declaredInAGateArm reports whether a binding names a test declared in one of
// this gate's own sources, and which.
//
// Those arms judge the REGISTER — that it is sorted, that it totals what it
// pins — and know nothing about any claim's subject, so binding to one
// satisfies every other check here while holding nothing at all. It is the
// cheapest way to retire a claim without auditing it, and unlike a narrowed
// detector it moves the tree, so it reads as the good kind of progress.
func declaredInAGateArm(paths []string) (string, bool) {
	for _, path := range paths {
		if gateFiles[path] {
			return path, true
		}
	}
	return "", false
}

// namesTheFile reports whether one of `paths` is the file the binding named.
//
// A PATH-SEGMENT suffix, not a string suffix. A bare `strings.HasSuffix`
// matched inside a FILENAME: `Held by: TestX (claims_test.go)` bound against
// `uniquenessclaims_test.go`, a file that does not exist, and `currency_test.go`
// bound against `employmentcurrency_test.go`. A binding that resolves to a file
// nobody named is worse than no binding, because it reads as checked.
func namesTheFile(paths []string, want string) bool {
	want = strings.TrimPrefix(filepath.ToSlash(want), "./")
	for _, path := range paths {
		for _, candidate := range []string{path, "backend/" + path} {
			if candidate == want || strings.HasSuffix(candidate, "/"+want) {
				return true
			}
		}
	}
	return false
}

// TestADeclarationsKeyIsUniqueWithinItsFile holds the labelling directly,
// because the tree does not.
//
// The register key is what makes an entry ratify ONE claim, and three
// declaration kinds can collide on a careless label: two methods named the same
// on different receivers, two multi-spec blocks, two package-qualified embedded
// fields. Nothing in the tree today carries an embedded-field claim, so the
// sweep exercises none of that branch — delete it and every other arm stays
// green while the collision returns. A guard the tree happens not to reach is a
// guard with no test.
func TestADeclarationsKeyIsUniqueWithinItsFile(t *testing.T) {
	t.Parallel()
	const source = `package probe

type Reader interface{ Read() }

// A is the only reader in this probe.
type A struct {
	// A.ID is the one spelling of a probe handle.
	ID string
	// io.Reader here is the one reader this probe embeds.
	io.Reader
	// fmt.Stringer here is the one stringer this probe embeds.
	fmt.Stringer
}

type B struct{}

// Close is the only writer on A.
func (a *A) Close() {}

// Close is the only writer on B.
func (b *B) Close() {}

// Get is the only reader on a generic store.
func (s *Store[T]) Get() {}

// left is the one spelling of the left probe.
const (
	left  = 1
	right = 2
)

// up is the one spelling of the up probe.
const (
	up   = 3
	down = 4
)
`
	file, err := parser.ParseFile(token.NewFileSet(), "probe.go", source, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing the probe: %v", err)
	}
	var labels []string
	record := func(doc *ast.CommentGroup, decl string) {
		if doc != nil {
			labels = append(labels, decl)
		}
	}
	for _, decl := range file.Decls {
		switch node := decl.(type) {
		case *ast.FuncDecl:
			record(node.Doc, funcLabel(node))
		case *ast.GenDecl:
			recordGenDecl(node, record)
		}
	}
	sort.Strings(labels)
	// Every label distinct, and each naming what it is about. Two methods on
	// different receivers, two blocks in one file, and two package-qualified
	// embedded fields are the three ways a careless key collides.
	want := []string{
		"const block at const left",
		"const block at const up",
		"method A.Close",
		"method B.Close",
		"method Store.Get",
		"type A",
		"type A.<embedded fmt.Stringer>",
		"type A.<embedded io.Reader>",
		"type A.ID",
	}
	if !slices.Equal(labels, want) {
		t.Errorf("labels = %q, want %q", labels, want)
	}
	// The embedded labels above come through `recordFields`' embedded branch on
	// the real walk, which is what makes them evidence: asserting
	// `genericReceiverName` alone would leave that branch unreached, so losing
	// the owner scope or the `<embedded …>` shape would keep this green.
}

func TestABindingDoesNotOpenTheDocCommentItSitsIn(t *testing.T) {
	t.Parallel()
	// Go's convention — and revive's `exported` rule — is that a doc comment
	// opens with the identifier it documents. A binding written above that
	// sentence turns the claim's own documentation into a footnote and fails
	// the linter on any EXPORTED declaration.
	//
	// Held here rather than left as advice, because advice already failed: the
	// linter caught one instance, this file wrote the rule into its own
	// docblock, and the next binding written broke it again on an UNEXPORTED
	// method, which revive does not check. A convention a gate can hold and
	// does not is the shape this whole change is about.
	for _, c := range allClaims(t) {
		if c.bindingOpens {
			t.Errorf("%s:%d %s opens its doc comment with `Held by:` — put the claim first "+
				"and the binding below it, so the comment still opens with the identifier "+
				"it documents", c.path, c.line, c.decl)
		}
	}
}

func TestTheRegisterHoldsNoEntryThatIsNoLongerAClaim(t *testing.T) {
	t.Parallel()
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

// shapeCensus pins how many unheld claims each detector shape attributes, and
// it is where the debt total comes from.
//
// A single total cannot tell the two ways it falls. A claim leaves the register
// either because somebody HELD it or because the claim was deleted — both of
// which change the tree — or because the DETECTOR stopped seeing it, which
// changes nothing but the number. Deleting a shape and the register lines it
// orphans satisfies every other arm in this file, and the claims it stopped
// reaching are still in the tree, still unheld, and no longer counted.
//
// Per shape, that move has to state itself: the row goes to zero or disappears,
// in a diff where the number sits beside the name of the thing that stopped
// looking. It also catches the narrowing a whole-shape deletion does not —
// trimming ONE alternative out of a regex leaves the shape matching plenty, so
// every arm asking "does this shape match anything" stays green while the
// claims that alternative reached go quiet. Here it is a failure naming the
// shape and both counts.
//
// The rows churn as claims are held, and that is the point: real work shows as
// a row falling, so the two kinds of progress are told apart by which number
// moved and whether the tree moved with it.
var shapeCensus = map[string]int{
	"cannot-drift":   156,
	"once":           155,
	"one-of-a-kind":  154,
	"is-every-named": 91,
	"only-noun":      16,
	"no-second":      10,
	"never-twice":    6,
	"is-every":       4,
	"one-truth":      3,
}

// registeredDebt is the number of unheld claims this tree carries, tracked
// EXACTLY: the register may not hold more than the census totals, and may not
// hold fewer.
//
// Pinned because "the register is closed to new entries" is a claim, and a
// claim about membership that nothing counts is what this gate refuses
// everywhere else: the other arms check that each line describes a live unheld
// claim, which a line added for a claim written this morning satisfies
// perfectly. Only a count catches that.
//
// DERIVED from the census above rather than written beside it, because two
// hand-written totals of one population are two answers to one question, and
// the one that stops matching looks exactly like the one that still does.
// Lowering it is the point of the file it counts; raising it is legal and is
// what a widened detector shape requires.
func registeredDebt() int {
	total := 0
	for _, count := range shapeCensus {
		total += count
	}
	return total
}

func TestTheRegisterHoldsExactlyTheDebtItPins(t *testing.T) {
	t.Parallel()
	keys := readRegister(t)
	debt := registeredDebt()
	if len(keys) > debt {
		t.Errorf("the register holds %d entries against a ceiling of %d.\n\n"+
			"A claim written today must name the test that holds it, not take a line here. "+
			"If a DETECTOR shape was widened, it now sees claims that were already in the tree: "+
			"raise that shape's row in shapeCensus in the same change, so the growth is one "+
			"line somebody agreed to.",
			len(keys), debt)
	}
	if len(keys) < debt {
		t.Errorf("the register holds %d entries and the census still totals %d — lower the row "+
			"of the shape whose claims were held, because the number is the thing this file is for",
			len(keys), debt)
	}
}

// TestEveryShapeAttributesTheClaimsItsCensusPins is the mirror of the arm above:
// that one holds the register against the census, this one holds the census
// against the DETECTOR.
//
// Without it the census is a second hand-written number rather than a
// measurement, and the move it exists to price — narrowing a shape until it
// stops reaching claims — is paid for by editing the very row that was supposed
// to notice.
func TestEveryShapeAttributesTheClaimsItsCensusPins(t *testing.T) {
	t.Parallel()
	attributed := map[string]int{}
	for _, c := range allClaims(t) {
		if c.held == "" {
			attributed[c.shape]++
		}
	}
	// A shape the detector no longer carries, still pinned here. This is the
	// deletion the whole census is for, and it reports the cost rather than
	// only the fact.
	live := map[string]bool{namedShape: true}
	for name := range claimShapes {
		live[name] = true
	}
	for _, shape := range slices.Sorted(maps.Keys(shapeCensus)) {
		if !live[shape] {
			t.Errorf("shapeCensus pins %d claim(s) to shape %q and the detector no longer "+
				"declares it — a shape that stops looking lowers this file's number without "+
				"auditing a single claim. Hold or delete those claims, or say in the detector "+
				"why the shape was never a claim form",
				shapeCensus[shape], shape)
		}
	}
	for _, shape := range append(sortedShapes(), namedShape) {
		pinned, ok := shapeCensus[shape]
		if !ok {
			t.Errorf("shape %q attributes %d unheld claim(s) and has no row in shapeCensus — "+
				"an unpinned shape can be narrowed to nothing and every arm here stays green",
				shape, attributed[shape])
			continue
		}
		if got := attributed[shape]; got != pinned {
			t.Errorf("shape %q attributes %d unheld claim(s), and shapeCensus pins %d.\n\n"+
				"Fewer means the shape stopped reaching claims it used to reach: either they "+
				"were held or deleted — lower the row — or the regex was narrowed, which is the "+
				"move this row exists to make somebody agree to. More means it was widened, and "+
				"the claims it now sees were always in the tree.",
				shape, got, pinned)
		}
	}
	// A row pinned at zero is a shape that has stopped attributing anything,
	// which reads here as a satisfied obligation and is a dead detector. It is
	// refused rather than tolerated, because the arm above compares two numbers
	// and 0 == 0 is the one comparison that holds while nothing is being
	// measured.
	for _, shape := range slices.Sorted(maps.Keys(shapeCensus)) {
		if shapeCensus[shape] == 0 {
			t.Errorf("shapeCensus pins shape %q at zero — delete the shape deliberately, or "+
				"restore what it was written to see; a row of zero passes exactly like a shape "+
				"whose claims were all held", shape)
		}
	}
}

func TestTheRegisterIsSortedAndFreeOfDuplicates(t *testing.T) {
	t.Parallel()
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

// TestEveryShapeEarnsItsPlaceInTheRealTree is the positive half, and it is
// DERIVED — the corpus is the tree.
//
// A corpus of sentences typed into this file and called "verbatim" is a copy
// with nothing keeping it in step with the original, so it drifts into fiction
// — and a fabricated corpus is the exact defect this file refuses, standing in
// its own docblock.
//
// So every shape must be attributed at least one claim by the SWEEP. A shape
// matching nothing in the real tree fails, and there is nothing to fabricate:
// the evidence is whatever the walk found.
func TestEveryShapeEarnsItsPlaceInTheRealTree(t *testing.T) {
	t.Parallel()
	claims := allClaims(t)
	perShape := map[string]string{}
	for _, c := range claims {
		if _, seen := perShape[c.shape]; !seen {
			perShape[c.shape] = fmt.Sprintf("%s:%d %q", c.path, c.line, c.phrase)
		}
	}
	want := append(sortedShapes(), namedShape)
	for _, shape := range want {
		example, found := perShape[shape]
		if !found {
			t.Errorf("shape %q matches nothing in the tree — a detector matching nothing "+
				"reports a clean tree, so either the shape is dead and should be deleted or "+
				"the claims it was written for have been reworded past it", shape)
			continue
		}
		t.Logf("%-16s %s", shape, example)
	}
	// The derived shape reads the DECLARATION's name, so it has to be shown
	// working on both spellings of a declaration that has one. Keying the label
	// by receiver — which the register needs, so two `Close` methods stay apart
	// — can stop it matching every METHOD in the tree while the arms above stay
	// green: they ask whether a shape matches ANYTHING, and its plain-function
	// claims still would.
	kinds := map[string]bool{}
	for _, c := range claims {
		if c.shape == namedShape {
			kinds[strings.SplitN(c.decl, " ", 2)[0]] = true
		}
	}
	for _, kind := range []string{"func", "method"} {
		if !kinds[kind] {
			t.Errorf("the derived shape matches no claim on a %s declaration — it reads the "+
				"declaration's own name, so a label change can stop it seeing a whole KIND "+
				"while the arms above stay green", kind)
		}
	}

	// And the walk that supplies that evidence has to have read something.
	if len(claims) < 100 {
		t.Errorf("the sweep found only %d claims, so every arm above is vouching for a "+
			"tree it barely read", len(claims))
	}
}

// TestNoShapeReadsOrdinaryProseAsAClaim is the negative half, and it covers
// EVERY shape including the derived one.
//
// A near-miss for every shape, because a shape with no case can be widened
// arbitrarily and this arm stays green — a negative corpus with holes permits
// exactly the widening it exists to refuse.
func TestNoShapeReadsOrdinaryProseAsAClaim(t *testing.T) {
	t.Parallel()
	// One sentence per shape, each written to graze the shape it is paired
	// with — the pairing is asserted below, so a case cannot quietly stop
	// being a near-miss of anything.
	prose := map[string]string{
		"one-of-a-kind": "The ledger is the only place a version and its name are recorded together.",
		"only-noun":     "This is the only way to ask the database whether it refuses a duplicate scope.",
		"cannot-drift":  "Two readers cannot both be right here, so the second one waits.",
		"once":          "The reader is shown the figure once they have opened the disclosure.",
		"no-second":     "An identical step is a no-op rather than a second audit row.",
		"one-truth":     "The source of the truncation is the provider, not this field.",
		"is-every":      "Every read is checked, and each one is every bit as guarded as the last.",
		// Counting, not duplication. A near-miss has to assert nothing about
		// uniqueness at all — "a retry is not two effects" would be a bad one,
		// because idempotence IS a claim of this family and the shape is right
		// to match it.
		"never-twice": "The grace window is not two hours but three, measured from the last attempt.",
		namedShape:    "Flush is every bit as ordered as Write, and neither is buffered.",
	}
	for _, sentence := range prose {
		for shape, pattern := range claimShapes {
			if phrase := pattern.FindString(sentence); phrase != "" {
				t.Errorf("shape %q reads ordinary prose as a claim (%q in):\n\t%s", shape, phrase, sentence)
			}
		}
		if named := namedExhaustiveness("func Flush"); named != nil {
			if match := named.FindStringSubmatch(sentence); match != nil && !intensifier(match[1]) {
				t.Errorf("the derived shape reads ordinary prose as a claim (%q in):\n\t%s", match[0], sentence)
			}
		}
	}
	// Every shape has a case, so widening one cannot go unnoticed.
	for _, shape := range append(sortedShapes(), namedShape) {
		if _, ok := prose[shape]; !ok {
			t.Errorf("shape %q has no near-miss case — it could be widened into ordinary "+
				"English and this arm would stay green", shape)
		}
	}
}

// TestAHyphenatedModifierIsNotAClaimAndDoesNotHideTheClaimBesideIt covers both
// directions of the compound-word exclusion.
//
// One direction is the false reading it refuses: a match opening mid-word
// quantifies nothing. The other is the blind spot the refusal could become — a
// doc comment holding a compound AND a real claim must still register the real
// one, or every claim that happens to follow a hyphenated word goes quiet and
// the register falls with nothing to say why.
func TestAHyphenatedModifierIsNotAClaimAndDoesNotHideTheClaimBesideIt(t *testing.T) {
	t.Parallel()
	onlyNoun := claimShapes["only-noun"]
	if onlyNoun == nil {
		t.Fatal("the only-noun shape is gone, so the cases below prove nothing about it")
	}
	compounds := []string{
		"letting a bare status edit set them lets a lead:update-only caller skip the person mint",
		"a Go-only definition of live would drift from the config layer the product reads",
		"the empty single-row read, through a stdlib-only implementation",
		"that is the defect the old body-only reader had in mirroring",
	}
	for _, sentence := range compounds {
		if phrase := claimPhrase(onlyNoun, sentence, nil); phrase != "" {
			t.Errorf("%q read as a claim (%q) — `only` there is the tail of a hyphenated "+
				"modifier and quantifies nothing", sentence, phrase)
		}
		// The pattern itself still matches: the exclusion is the reader's, so
		// a case that stopped grazing the shape would stop proving anything.
		if !onlyNoun.MatchString(sentence) {
			t.Errorf("%q no longer matches the only-noun shape at all, so it is not a "+
				"near-miss of anything and this case has gone decorative", sentence)
		}
	}
	// The claim survives a compound earlier in the same comment.
	const both = "It stops the CREATE-only caller. The store is the only writer of that column."
	if phrase := claimPhrase(onlyNoun, both, nil); phrase != "only writer" {
		t.Errorf("claimPhrase(%q) = %q, want %q — a compound before a claim must not "+
			"swallow it", both, phrase, "only writer")
	}
	// A NON-BREAKING HYPHEN joins a compound exactly as the ASCII one does, and
	// reading the preceding BYTE sees only the last third of it. The case is
	// here rather than left to the tree because the tree happens not to contain
	// one today, which is what makes it the spelling a regression would take.
	const nonBreaking = "a Go‑only definition of live would drift from the config layer"
	if phrase := claimPhrase(onlyNoun, nonBreaking, nil); phrase != "" {
		t.Errorf("claimPhrase read %q from %q — U+2011 joins a compound and the match is "+
			"still the tail of a word", phrase, nonBreaking)
	}
	// An em-dash is sentence punctuation here, not a joiner, so a claim opening
	// after one is an ordinary claim and must still register. Suppressing it
	// would turn the compound fix into a new blind spot across the whole tree,
	// which uses em-dashes constantly.
	const emDash = "the store—only writer of that column—answers"
	if phrase := claimPhrase(onlyNoun, emDash, nil); phrase != "only writer" {
		t.Errorf("claimPhrase(%q) = %q, want %q — an em-dash separates clauses and does "+
			"not make the next word a compound", emDash, phrase, "only writer")
	}

	// A shape that opens on a VERB rather than on the quantifier keeps its
	// claim when the verb is compounded. `hand-written` is this tree's own
	// idiom, so a rule keyed on the match's start rather than on the
	// quantifier would silence the second-largest shape with ordinary prose,
	// and nothing here would fail.
	once := claimShapes["once"]
	if once == nil {
		t.Fatal("the once shape is gone, so the cases below prove nothing about it")
	}
	for _, sentence := range []string{
		"The route table is hand-written once, in composeRoutes.",
		"The timeout is well-defined once, in the config loader.",
		"Every column is auto-named once, by the generator.",
	} {
		if phrase := claimPhrase(once, sentence, nil); phrase == "" {
			t.Errorf("claimPhrase read no claim from %q — the compounded word is the verb, "+
				"and the sentence still says the thing is written once", sentence)
		}
	}
}

// TestAnIdiomDoesNotHideTheDerivedShapesRealClaim covers the derived shape's
// own version of the defect above.
//
// `namedExhaustiveness` is prefiltered before it joins the shape list, and the
// idiom rule travels with the pattern rather than being applied to that
// prefilter alone. Applied there, a comment whose FIRST "<name> is every …" is
// the "every bit" idiom would never add the shape, and the real claim in the
// next sentence would be invisible to every arm — the two readings of one text
// have to agree about which match is the claim.
func TestAnIdiomDoesNotHideTheDerivedShapesRealClaim(t *testing.T) {
	t.Parallel()
	named := namedExhaustiveness("func Read")
	if named == nil {
		t.Fatal("the derived shape built nothing for a plain function, so this case proves nothing")
	}
	// The idiom has to MATCH the pattern to be the first match — the shape
	// wants the declaration's own name immediately before "is every", so a
	// sentence merely containing the idiom reaches the reject arm never, and
	// the case would pass without exercising it.
	const idiomFirst = "Read is every bit as ordered as Write.\n" +
		"Read is every reader of the authoritative row.\n"
	if phrase := claimPhrase(named, idiomFirst, intensifier); phrase == "" {
		t.Errorf("the derived shape found no claim in %q — an idiom in the first sentence "+
			"must not hide the exhaustiveness claim in the second", idiomFirst)
	}
	// And the idiom alone still claims nothing, or the rule above has been
	// widened into ordinary English.
	const idiomOnly = "Read is every bit as ordered as Write, and neither is buffered."
	if phrase := claimPhrase(named, idiomOnly, intensifier); phrase != "" {
		t.Errorf("the derived shape read %q from an idiom-only comment: %q", phrase, idiomOnly)
	}
}

// TestTheBindingIsReadOffTheCommentRatherThanGuessedAt proves the `Held by:`
// parser on the shapes an author will actually write, including the ones that
// must NOT bind.
func TestTheBindingIsReadOffTheCommentRatherThanGuessedAt(t *testing.T) {
	t.Parallel()
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
