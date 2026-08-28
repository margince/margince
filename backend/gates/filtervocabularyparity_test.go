// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// Two surfaces answer a filtered question: the list/segment compiler in
// `platform/database/storekit`, and the query compiler in `modules/search`.
// A reader does not know which one served them, so `status neq lost` has to
// mean the same thing on both.
//
// TWO WAYS IT CAN STOP MEANING THE SAME THING, and this file holds both.
//
// The SQL. `<>` and `IS DISTINCT FROM` are different row sets, not different
// spellings: a record whose status is UNSET is dropped by the first and kept by
// the second. A caller reading "everything that is not lost" means to be shown
// it. Nothing about the operator's name says which reading applies, so the
// choice has to be made once and held.
//
// The VOCABULARY. One surface offering an operator the other refuses is a
// reader who can ask a question in search and not in a segment. That may be
// right — the two surfaces do not index the same things — but it is a decision,
// and an undeclared difference is indistinguishable from an oversight.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	storekitMatrix = "internal/platform/database/storekit/predicate.go"
	searchMatrix   = "internal/modules/search/queryvocab.go"
)

// correspondingTypes pairs a storekit field type with the search kind that
// answers the same question about the same data.
//
// Declared rather than derived: the two vocabularies were named independently
// and neither is a renaming of the other. A pair left out of this map is not
// compared, so the map is also the statement of what this gate does NOT cover.
// exists is why neither surface's `exists` difference is a defect. It is one
// fact about one operator, so it has one home: seven copies would be seven
// things to keep in step, and whichever drifted would misstate why its
// difference is ratified while still reading like a reason.
const exists = "search declares no `exists` operator at all — its vocabulary is eq, neq, in, lt, lte, gt, gte, within_radius and nothing else. It compiles against the record's OWN TABLE (querysql builds `FROM <table> t`, columns derived from information_schema), so an unset field is a NULL column exactly as it is for storekit and `IS NULL` would answer it. There is no capability reason; this is a vocabulary gap nobody has decided about."

// unpairedKinds are types one surface declares that the other has no counterpart
// for, so `correspondingTypes` cannot name them. Ratified rather than skipped:
// an unpaired type is uncompared, and the gate must say which ones those are or
// a new type on either surface joins them silently.
var unpairedKinds = gatekit.Waive(map[string]string{
	"KindTimestamp": "search separates a timestamp from a date; storekit's FieldDate covers both, so there is no distinct storekit type to compare this against. The operators it carries are identical to KindDate's, which FieldDate is already compared with.",
	"KindGeo":       "a geo field admits only within_radius, an operator storekit has no compiler for at all. There is nothing to compare: the difference is the whole type, not an operator within it.",
})

// gatekit:fixture the declared pairing between the two vocabularies' type names
var correspondingTypes = map[string]string{
	"FieldText":     "KindText",
	"FieldID":       "KindID",
	"FieldNumber":   "KindNumber",
	"FieldDate":     "KindDate",
	"FieldBoolean":  "KindBoolean",
	"FieldPicklist": "KindText",
	"FieldCurrency": "KindNumber",
}

// declaredDifferences ratifies each operator one surface offers and the other
// does not. A difference without an entry is a finding; an entry that stops
// matching is one for a difference that has been resolved, and leaving it
// re-exempts whatever takes its place.
var declaredDifferences = gatekit.Waive(map[string]string{
	"FieldText/exists":     exists,
	"FieldID/exists":       exists,
	"FieldNumber/exists":   exists,
	"FieldDate/exists":     exists,
	"FieldBoolean/exists":  exists,
	"FieldPicklist/exists": exists,
	"FieldCurrency/exists": exists,
	"FieldText/contains":   "search declares no `contains` operator. Its structured `where` answers exact and ordering comparisons; approximate text is the free-text half of the same request, not a structured operator. storekit has no free-text half, so `contains` is the only way to ask there. The split is real, but nothing states it as a decision — it is how the two surfaces were built.",
	"FieldBoolean/neq":     "search offers `eq` alone on a boolean, its vocabulary saying `neq true` is `eq false`. THAT REASONING NO LONGER HOLDS: with neq NULL-safe, `neq true` selects false AND unset, which `eq false` does not. storekit's boolean neq now answers a question search cannot express. Waived because closing it is a product decision about search's vocabulary, not a repair.",
	"FieldDate/in":         "search admits `in` on a date and storekit does not. A gap rather than a decision — but not a one-bit fix: flipping the matrix routes dates through the string `in` branch, which binds a text array against a date column and fails at query time. It needs a date branch.",
})

// operatorSets reads a matrix declaration and returns, per type, its operators.
func operatorSets(t *testing.T, path, declName string) map[string][]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	sets := map[string][]string{}
	ast.Inspect(file, func(node ast.Node) bool {
		spec, ok := node.(*ast.ValueSpec)
		if !ok || len(spec.Names) == 0 || spec.Names[0].Name != declName || len(spec.Values) == 0 {
			return true
		}
		matrix, ok := spec.Values[0].(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, element := range matrix.Elts {
			pair, ok := element.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := pair.Key.(*ast.Ident)
			if !ok {
				continue
			}
			sets[key.Name] = operatorNames(pair.Value)
		}
		return true
	})
	if len(sets) == 0 {
		t.Fatalf("%s declares no %s this census can read", path, declName)
	}
	return sets
}

// operatorNames reads a type's operators from either shape the two matrices
// use: a set (`map[string]bool`) or a list (`[]string`).
//
// A set entry's VALUE decides admission — storekit refuses an operator whose
// entry is `false` — so reading the key alone would count a refused operator as
// offered, and the census would report parity across a real divergence.
func operatorNames(value ast.Expr) []string {
	lit, ok := value.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	var names []string
	for _, element := range lit.Elts {
		switch entry := element.(type) {
		case *ast.KeyValueExpr:
			key, isIdent := entry.Key.(*ast.Ident)
			if isIdent && admits(entry.Value) {
				names = append(names, operatorConstant(key.Name))
			}
		case *ast.Ident:
			names = append(names, operatorConstant(entry.Name))
		}
	}
	sort.Strings(names)
	return names
}

// admits reads a set entry's value. Anything that is not the literal `true` is
// treated as a refusal: a value this cannot read is not one it has cleared.
func admits(value ast.Expr) bool {
	ident, ok := value.(*ast.Ident)
	return ok && ident.Name == "true"
}

// operatorConstant reduces `OpNeq` to `neq`, so the two packages' constants
// compare by what they mean rather than by how each spells them.
func operatorConstant(name string) string {
	return strings.ToLower(strings.TrimPrefix(name, "Op"))
}

func TestBothFilterSurfacesOfferTheSameOperators(t *testing.T) {
	t.Parallel()
	// A ratification that stops matching is one for a difference already closed.
	defer declaredDifferences.AssertAllMatched(t)

	storekitOps := operatorSets(t, storekitMatrix, "operatorsByType")
	searchOps := operatorSets(t, searchMatrix, "operatorsByKind")

	var findings []string
	compared := 0
	for fieldType, kind := range correspondingTypes {
		mine, known := storekitOps[fieldType]
		theirs, paired := searchOps[kind]
		if !known || !paired {
			findings = append(findings, fieldType+"/"+kind+": one side no longer declares this type")
			continue
		}
		compared++
		for _, op := range symmetricDifference(mine, theirs) {
			if declaredDifferences.Waived(t, fieldType+"/"+op) {
				continue
			}
			findings = append(findings, fieldType+"/"+op+" — offered by one surface, refused by the other")
		}
	}
	// Nothing either surface declares may go uncompared without being named. A
	// type added to one matrix and not to correspondingTypes would otherwise
	// escape the parity check entirely — the census would keep passing while
	// covering less of the tree than it did yesterday.
	defer unpairedKinds.AssertAllMatched(t)
	paired := map[string]bool{}
	for fieldType, kind := range correspondingTypes {
		paired[fieldType], paired[kind] = true, true
	}
	for declared := range storekitOps {
		if !paired[declared] && !unpairedKinds.Waived(t, declared) {
			findings = append(findings, declared+" is declared by storekit and compared with nothing")
		}
	}
	for declared := range searchOps {
		if !paired[declared] && !unpairedKinds.Waived(t, declared) {
			findings = append(findings, declared+" is declared by search and compared with nothing")
		}
	}
	if compared < 5 {
		t.Fatalf("the census compared only %d type pairs, so it is judging almost nothing", compared)
	}
	sort.Strings(findings)
	if len(findings) > 0 {
		t.Errorf("%d operator difference(s) between the two filter surfaces are undeclared.\n\n"+
			"A reader does not know which surface answered them, so a question they can ask in "+
			"search and not in a segment is a decision somebody has to have made. Offer it on both, "+
			"or ratify it in declaredDifferences with the reason.\n\n\t%s",
			len(findings), strings.Join(findings, "\n\t"))
	}
}

// nullDroppingInequality are the spellings Postgres treats as the same
// NULL-dropping comparison. `!=` is documented as a synonym for `<>`, and it is
// the one a hand typing it reaches for first, so a census that read only `<>`
// would refuse the spelling nobody uses and admit the one they do.
//
// `NOT (col = $1)` drops unset rows too and is NOT matched here: it is also the
// shape of the linked-field path's legitimate NOT EXISTS, and a pattern wide
// enough to catch one catches the other.
var nullDroppingInequality = []string{"<>", "!="}

// filterCompilers are the files that turn a caller's operator into SQL. Both
// answer the same vocabulary, so both have to answer it the same way.
var filterCompilers = []string{
	"internal/platform/database/storekit/predicate.go",
	"internal/platform/database/storekit/predicateoperand.go",
	"internal/modules/search/querysql.go",
}

func TestNeitherFilterCompilerDropsAnUnsetRowFromNeq(t *testing.T) {
	t.Parallel()
	nullSafe := 0
	var findings []string
	for _, path := range filterCompilers {
		source, err := os.ReadFile(path) // #nosec G304 -- a fixed path in the tracked source tree
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		for _, statement := range gatekit.SQLStatementsIn(t, path, string(source)) {
			for _, spelling := range nullDroppingInequality {
				if strings.Contains(statement, spelling) {
					findings = append(findings, path+": "+statement)
					break
				}
			}
			if strings.Contains(strings.ToUpper(statement), "IS DISTINCT FROM") {
				nullSafe++
			}
		}
	}
	// Zero `<>` is what a clean tree and a blind census both look like. The
	// null-safe rendering has to be visible for the absence to mean anything.
	if nullSafe == 0 {
		t.Fatal("no filter compiler renders IS DISTINCT FROM, so this census is vouching for nothing")
	}
	if len(findings) > 0 {
		sort.Strings(findings)
		t.Errorf("%d comparison(s) in a filter compiler drop unset rows.\n\n"+
			"`<>` and IS DISTINCT FROM are different ROW SETS, not different spellings: a record "+
			"whose column is unset is dropped by the first and kept by the second, and a caller "+
			"reading \"everything that is not X\" means to be shown it. The other compiler, and this "+
			"package's own linked-field path, both keep it.\n\n\t%s",
			len(findings), strings.Join(findings, "\n\t"))
	}
}

func symmetricDifference(left, right []string) []string {
	in := func(list []string, want string) bool {
		for _, item := range list {
			if item == want {
				return true
			}
		}
		return false
	}
	var only []string
	for _, op := range left {
		if !in(right, op) {
			only = append(only, op)
		}
	}
	for _, op := range right {
		if !in(left, op) {
			only = append(only, op)
		}
	}
	sort.Strings(only)
	return only
}
