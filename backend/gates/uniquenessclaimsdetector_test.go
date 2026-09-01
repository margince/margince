// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// The detector behind uniquenessclaims_test.go: which comments are claims,
// which declaration each belongs to, and the key that tells two of them apart.
//
// Split from the rules it serves because the two answer different questions. A
// reader asking "what does the gate refuse" wants the arms; a reader asking
// "would it see MY comment" wants this, and neither should have to scroll past
// the other. The rules and their fixtures live in uniquenessclaims_test.go.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
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
	// The bare `two` alternative is deliberately NOT here. It matched any
	// counting sentence — "the grace window is not two hours but three" — and a
	// shape that reads a quantity as a uniqueness claim puts innocent prose in a
	// closed register, which teaches people to write worse comments rather than
	// fewer false claims.
	//
	// Measured cost: about twenty `not two <noun>` sites, most of them real
	// claims ("not two independent", "not two sites", "not two readings"), are
	// invisible to this gate. That is the same trade the file makes above for
	// "the one place" and "the only way", and it is stated for the same reason:
	// a blind spot a reader can see is worth more than a shape they turn off.
	"never-twice": regexp.MustCompile(`(?i)\b(?:never|not) (?:duplicated|respelled|re-?implemented|spelled twice|written twice|copied)\b`),
}

// namedShape is what a finding attributed to the derived shape is called. One
// spelling, because it is used by the record below and by both corpus arms.
const namedShape = "is-every-named"

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
	// The BARE name, with both the kind prefix ("func ", "method ") and any
	// receiver qualifier stripped: a doc comment on `func (s *Service)
	// readProfileFields` opens with `readProfileFields`, not with
	// `Service.readProfileFields`. Keying the label by receiver (which the
	// register needs, so two `Close` methods stay apart) silently broke this
	// lookup and the claim disappeared from the sweep — a shape that stops
	// matching a whole KIND of declaration while still matching others keeps
	// every arm green, because the arms ask whether a shape matches ANYTHING.
	name := decl
	if index := strings.LastIndex(name, " "); index >= 0 {
		name = name[index+1:]
	}
	if index := strings.LastIndex(name, "."); index >= 0 {
		name = name[index+1:]
	}
	if name == "" || name == "block" {
		return nil
	}
	return regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(name) + `\b\s+(?:is|are) every\s+(\w+)`)
}

// claimPhrase returns the first match that is a claim rather than prose, and ""
// when the pattern reaches nothing else.
//
// `\bonly caller` matches inside "a lead:update-only caller", where "only"
// binds leftwards into the compound and quantifies nothing: the sentence says
// which KIND of caller, not that there is one. Ordinary prose of that shape is
// common — "Go-only definition", "stdlib-only implementation", "body-only
// reader" — and a register counting it as unaudited debt is a register whose
// number means less than it says.
//
// A hyphen is the discriminator rather than a list of the compounds seen so
// far, because the next one will be spelled differently and a list would miss
// it. Applied to every shape, not just the one that has the problem today: a
// match that begins immediately after a hyphen is the second half of a word in
// any of them.
//
// Go's regexp has no lookbehind, so this reads what precedes the match rather
// than expressing the exclusion in the pattern — the same trade `intensifier`
// makes below, for the same missing feature.
//
// Every match is examined, not only the first. Returning "" because the first
// match was prose would un-see every claim that happens to FOLLOW a hyphenated
// word, and the register would fall with nothing to say why — the same defect
// one rung down from the one the shape census exists to price.
//
// reject, when non-nil, is asked about the match's first capture group and says
// whether that match is prose. It carries the derived shape's `intensifier`
// rule, which has to be applied HERE rather than beside the caller: a shape
// whose first match is an idiom still qualifies on a later real one.
func claimPhrase(pattern *regexp.Regexp, text string, reject func(string) bool) string {
	for _, span := range pattern.FindAllStringSubmatchIndex(text, -1) {
		if compoundedQuantifier(text, span[0], span[1]) {
			continue
		}
		if reject != nil && len(span) >= 4 && span[2] >= 0 && reject(text[span[2]:span[3]]) {
			continue
		}
		return text[span[0]:span[1]]
	}
	return ""
}

// compoundedQuantifier reports whether a match opens on a QUANTIFIER that is
// the tail of a hyphenated word, where it binds leftwards and quantifies
// nothing.
//
// By RUNE and not by byte: `text[index-1]` reads the last continuation byte of
// whatever multi-byte character precedes the match, and U+2011 NON-BREAKING
// HYPHEN joins a compound exactly as U+002D does while ending in a byte that is
// not `-`. Reading the rune costs nothing and removes a whole spelling of the
// same false positive.
//
// The dashes are deliberately NOT here. An em- or en-dash in this tree is
// sentence punctuation — "the store — the only writer" — and a clause opening
// after one is ordinary prose making an ordinary claim. Only characters that
// join two halves of a single word suppress a match.
//
// And only when the compounded word is the QUANTIFIER. Most shapes open on a
// verb instead, where a hyphen changes nothing about the claim: "the route
// table is hand-written once" says exactly what "written once" says, and
// suppressing it would silence a whole shape — `hand-written` is this tree's
// own idiom, so ordinary prose would trip it. Over-collecting costs a register
// line somebody resolves; over-suppressing costs a claim nobody ever sees
// again, so the rule is the narrow one.
func compoundedQuantifier(text string, start, end int) bool {
	if start <= 0 {
		return false
	}
	previous, _ := utf8.DecodeLastRuneInString(text[:start])
	switch previous {
	case '-', '‐', '‑':
	default:
		return false
	}
	first, _, _ := strings.Cut(text[start:end], " ")
	switch strings.ToLower(first) {
	case "only", "one", "single", "no", "never", "not":
		return true
	}
	return false
}

// intensifier reports whether "every <word>" is an English idiom rather than a
// set assertion.
//
// "Flush is every bit as ordered as Write" documents a comparison and claims
// nothing about a set, and the register is closed — so without this, an author
// writing that innocent sentence has to delete it or invent a gate for it,
// which is how a census teaches people to write worse comments. Two words,
// named as the idioms they are; Go's regexp has no negative lookahead, so the
// exclusion is a check on the captured word rather than a hole in the pattern.
func intensifier(word string) bool {
	switch strings.ToLower(word) {
	case "bit", "inch":
		return true
	}
	return false
}

// heldBy is the binding a claim carries to say which test holds it. Free text
// around it, because it sits inside a doc comment a human is also reading:
//
//	// Held by: TestEveryFooHasOneWriter (backend/gates/foowriters_test.go)
//
// It may sit ANYWHERE in the doc comment except the first line: revive requires
// a doc comment to open with the identifier it documents, so a binding written
// above that sentence fails the linter rather than this gate. Below the opening
// sentence is where it reads best anyway — the claim first, then what holds it.
//
// The PATH is required and checked. A bare test name would be a claim about a
// name, and this whole file exists because a claim nobody checks is worth less
// than nothing.
//
// The name must continue with an UPPERCASE letter or an underscore, which is
// what `go test` itself requires of a test function. `Testhelperghost` is a
// legal Go identifier and a legal thing to write, and `go test` will never run
// it — so a claim could name a "gate" that compiles, exists, and can never
// execute. The signature is checked too, below.
// claimWrap is the whitespace a comment's own line WRAPPING produced: a run of
// space that crosses at most one newline. Collapsed to a single space so a
// phrase reads the same however it was broken.
//
// A blank line is deliberately not matched. It separates two paragraphs, and
// joining them would run the last words of one into the first words of the
// next — "… the one" then "writer of …" becomes a claim nobody wrote, which is
// over-recognition in a register whose whole value is that its number means
// something.
var claimWrap = regexp.MustCompile(`[^\S\n]*\n[^\S\n]*|[^\S\n]+`)

// flattenWraps rejoins each PARAGRAPH onto one line, leaving the blank lines
// between them where they are.
func flattenWraps(text string) string {
	paragraphs := claimParagraph.Split(text, -1)
	for i, paragraph := range paragraphs {
		paragraphs[i] = strings.TrimSpace(claimWrap.ReplaceAllString(paragraph, " "))
	}
	return strings.Join(paragraphs, "\n\n")
}

// claimParagraph is a blank line — the boundary flattenWraps will not cross.
var claimParagraph = regexp.MustCompile(`\n[^\S\n]*\n`)

var heldBy = regexp.MustCompile(`Held by:\s*(Test[A-Z_][A-Za-z0-9_]*)\s*\(([^)]+)\)`)

// goTestFunc finds the declarations a `Held by:` may name.
// A test `go test` will RUN: the uppercase continuation it requires of the
// name, and the `*testing.T` parameter it requires of the signature. A no-arg
// `func Testhelperghost() {}` satisfies neither, and a gate that can never run
// is not a gate.
var goTestFunc = regexp.MustCompile(`(?m)^func (Test[A-Z_][A-Za-z0-9_]*)\([a-zA-Z_][a-zA-Z0-9_]* \*testing\.T\)`)

// authoredGoFile reports whether a file is one somebody could have written a
// claim in: Go, and not generated.
//
// Shared rather than respelled at each walk. Four sweeps in this gate ask "is
// this a source an author wrote" — the claim sweep, the test-function sweep and
// the two corpus sweeps — and they answered it three different ways, one of
// them missing `.git`. A gate whose walks disagree about their own subject
// reads green over whatever the narrowest of them skipped.
func authoredGoFile(name string) bool {
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_gen.go")
}

// unauthoredDir reports a directory no author writes into: an installed
// dependency tree, a fixture directory, or git's own store. Nothing under one
// can carry a claim somebody chose to make.
func unauthoredDir(name string) bool {
	switch name {
	case "node_modules", "testdata", ".git":
		return true
	}
	return false
}

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
	path   string // slash-normalised, relative to the module the root names
	decl   string // "func Foo", "var bar", "type Baz", "package qux"
	shape  string // which claimShapes entry matched
	phrase string // the matched words, for the failure message
	line   int
	held   string // the test named by `Held by:`, empty when none
	heldIn string // the file that test is claimed to live in
	// bindingOpens is true when `Held by:` is the FIRST line of the doc
	// comment, which Go's own convention forbids: a doc comment opens with the
	// identifier it documents. Recorded rather than merely discouraged, because
	// revive enforces it only on EXPORTED declarations while this rule applies
	// to every one.
	bindingOpens bool
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
			if unauthoredDir(entry.Name()) {
				return fs.SkipDir
			}
			// The GENERATED contract package, by PATH. Matching the directory
			// NAME skipped `internal/modules/contracts/` too — a hand-written
			// product module — so a claim written anywhere in the contracts CRM
			// module needed no gate at all. A skip list keyed on a bare name is
			// a skip list that does not know what it is skipping.
			if filepath.ToSlash(path) == "internal/contracts" {
				return fs.SkipDir
			}
			return nil
		}
		if !authoredGoFile(filepath.Base(path)) {
			return nil
		}
		if gateFiles[filepath.ToSlash(path)] {
			// This file spells out the very phrases it hunts for, so a sweep
			// that read it would register its own prose as debt: a sentence
			// naming a shape is not a claim made by the declaration it sits
			// on. A gate describing its own subject cannot also be judged by
			// it. `format/zone-by-purpose.test.ts` skips itself likewise.
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
			// Flattened before matching. doc.Text() keeps the newlines the
			// comment was WRAPPED at, and every pattern below spells its
			// phrase with literal spaces — so "the one spelling" is a claim
			// this sees and the same claim broken across two lines is not.
			//
			// That is the one direction a census must not break: reflowing a
			// paragraph changes no meaning, and it would have taken a live
			// claim out of the register with nothing failing to say so.
			text := flattenWraps(doc.Text())
			shapes := sortedShapes()
			patterns := make(map[string]*regexp.Regexp, len(claimShapes)+1)
			for name, pattern := range claimShapes {
				patterns[name] = pattern
			}
			// The derived shape's idiom rule travels WITH its pattern rather
			// than being applied once here, so the two readings of the same
			// text cannot disagree about which match is the claim.
			rejects := map[string]func(string) bool{}
			if named := namedExhaustiveness(decl); named != nil {
				if claimPhrase(named, text, intensifier) != "" {
					shapes = append(shapes, namedShape)
					patterns[namedShape] = named
					rejects[namedShape] = intensifier
				}
			}
			for _, shape := range shapes {
				phrase := claimPhrase(patterns[shape], text, rejects[shape])
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
					c.bindingOpens = strings.HasPrefix(strings.TrimSpace(text), "Held by:")
				}
				found = append(found, c)
				return
			}
		}
		record(file.Doc, "package "+file.Name.Name)
		for _, decl := range file.Decls {
			switch node := decl.(type) {
			case *ast.FuncDecl:
				record(node.Doc, funcLabel(node))
			case *ast.GenDecl:
				recordGenDecl(node, record)
			}
		}
		return nil
	})
	return found, err
}

// recordFields reports a claim written on a struct field or an interface
// method, keyed by the type it belongs to so two fields called `ID` on
// different types stay apart.
func recordFields(spec *ast.TypeSpec, owner string, record func(*ast.CommentGroup, string)) {
	var list *ast.FieldList
	switch node := spec.Type.(type) {
	case *ast.StructType:
		list = node.Fields
	case *ast.InterfaceType:
		list = node.Methods
	default:
		return
	}
	if list == nil {
		return
	}
	for _, field := range list.List {
		for _, fieldName := range field.Names {
			record(field.Doc, owner+"."+fieldName.Name)
		}
		if len(field.Names) == 0 {
			// An embedded field carries no name of its own; key it by the type
			// it embeds so it is still distinguishable.
			record(field.Doc, owner+".<embedded "+genericReceiverName(field.Type)+">")
		}
	}
}

// funcLabel names a function or a method uniquely within its file.
//
// The RECEIVER is part of the name, because dropping it collided: `A.Close` and
// `B.Close` in one file both keyed as `func Close`, so registering one claim
// silently ratified the other — and, worse, a NEW claim on the second method
// inherited the first's register entry and passed a gate whose whole promise is
// that new claims cannot. A register whose key is not unique has holes exactly
// where a file is busiest.
func funcLabel(node *ast.FuncDecl) string {
	if node.Recv == nil || len(node.Recv.List) == 0 {
		return "func " + node.Name.Name
	}
	// rbacgate_test.go's `receiverName`, shared rather than respelled — this
	// package already answers "which type is this method on" once, and a second
	// answer to that is the thing this file exists to refuse.
	//
	// It returns "" for a GENERIC receiver (`*Entry[T]`), which it never had to
	// unwrap. Twenty-eight methods in this tree have one, and keying them all
	// as one name would rebuild the collision this label was written to remove,
	// so they are named from the source expression here. Widening the shared
	// helper instead would change which buckets the RBAC census reads, which is
	// that gate's verdict to re-verify rather than this change's.
	owner := receiverName(node)
	if owner == "" {
		owner = genericReceiverName(node.Recv.List[0].Type)
	}
	return "method " + owner + "." + node.Name.Name
}

// genericReceiverName unwraps a receiver the shared helper does not: pointer
// and type arguments stripped, so `*Entry[T]` reads `Entry`.
func genericReceiverName(expr ast.Expr) string {
	for {
		switch node := expr.(type) {
		case *ast.StarExpr:
			expr = node.X
		case *ast.IndexExpr:
			expr = node.X
		case *ast.IndexListExpr:
			expr = node.X
		case *ast.Ident:
			return node.Name
		case *ast.SelectorExpr:
			// A package-qualified type, which is how an embedded field usually
			// arrives. Unqualified, `io.Reader` and `fmt.Stringer` embedded in
			// one struct would both key as `?` — the same collision the
			// receiver rule above exists to remove, one declaration kind over.
			return genericReceiverName(node.X) + "." + node.Sel.Name
		default:
			return "?"
		}
	}
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
		label, ok := name(spec)
		if !ok {
			continue
		}
		switch s := spec.(type) {
		case *ast.TypeSpec:
			record(s.Doc, label)
			// A struct field's or an interface method's doc comment IS a doc
			// comment on a declaration, and a claim written on one escaped
			// entirely — `Addresses is EVERY address this record names` sat in
			// a field doc and no arm ever saw it. The enclosing type's Doc does
			// not contain a field's, so the walk has to go in.
			recordFields(s, label, record)
		case *ast.ValueSpec:
			record(s.Doc, label)
		}
	}
	if len(node.Specs) == 1 {
		if label, ok := name(node.Specs[0]); ok {
			record(node.Doc, label)
			return
		}
	}
	// A multi-spec block is named by its FIRST declared name, not by the bare
	// word "block". Two `const ( … )` blocks in one file both keyed as
	// `const block`, so the second one's claim would ride the first's register
	// entry — a new, unheld claim passing the gate that refuses exactly that.
	label := node.Tok.String() + " block"
	if len(node.Specs) > 0 {
		if first, ok := name(node.Specs[0]); ok {
			label = node.Tok.String() + " block at " + first
		}
	}
	record(node.Doc, label)
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
