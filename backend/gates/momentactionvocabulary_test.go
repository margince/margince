// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// Every offered verb is classified by whether pressing it writes.
//
// compose/momentaction looks each verb up in its surface's map to decide what
// a seat without `activity.create` is offered. A verb missing from a map is
// offered wrongly, so each map must answer the contract's whole enum. The enum
// is read from api/crm.yaml and the keys from the source.

import (
	"go/ast"
	"go/token"
	"sort"
	"strings"
	"testing"
	"unicode"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

type offeredVerbs struct {
	schema string
	// Where the enum sits in the schema: each anchor is found in turn, then the
	// first flow-list enum after the last one.
	anchors     []string
	source      string
	mapName     string
	constPrefix string
	// The enum's size today. A floor, so an extraction that goes short fails.
	floor int
	// How an entry says "this verb writes".
	writes func(t *testing.T, mapName, kind string, value ast.Expr) bool
}

var offeredVerbVocabularies = []offeredVerbs{
	{
		schema: "ContactMomentAction", source: "internal/compose/momentaction/withhold.go",
		mapName: "writesAnActivity", constPrefix: "ContactMomentActionKind", floor: 8,
		writes: boolLiteral,
	},
	{
		schema: "WorklistMove", anchors: []string{"\n        action:"},
		source:  "internal/compose/momentaction/writeverbs.go",
		mapName: "worklistVerbWrites", constPrefix: "WorklistMoveAction", floor: 7,
		writes: refusalPhrase,
	},
	{
		schema: "Company360Suggestion", anchors: []string{"\n        action:", "\n            kind:"},
		source:  "internal/compose/momentaction/writeverbs.go",
		mapName: "suggestionVerbWrites", constPrefix: "Company360SuggestionActionKind", floor: 3,
		writes: refusalPhrase,
	},
}

func TestEveryOfferedVerbIsClassifiedAsWritingOrNot(t *testing.T) {
	t.Parallel()
	for _, vocabulary := range offeredVerbVocabularies {
		t.Run(vocabulary.mapName, func(t *testing.T) {
			t.Parallel()
			vocabulary.check(t)
		})
	}
}

func (v offeredVerbs) check(t *testing.T) {
	declared := v.enum(t)
	if len(declared) < v.floor {
		t.Fatalf("read %d verbs from %s in api/crm.yaml (%v), and the enum has at least %d, "+
			"so the extraction has gone short and this gate would pass vacuously",
			len(declared), v.schema, declared, v.floor)
	}
	classified, writing := v.classification(t)
	// A map read as all-false would still satisfy set equality while
	// withholding nothing at all.
	if writing < 2 {
		t.Errorf("%s classifies %d verbs as writing an activity, want at least two: "+
			"a vocabulary where nothing writes withholds nothing", v.mapName, writing)
	}
	for _, kind := range declared {
		if _, ok := classified[kind]; !ok {
			t.Errorf("%s admits %q and %s does not answer for it. Add it to the map with the "+
				"answer for that verb", v.schema, kind, v.mapName)
		}
	}
	for kind := range classified {
		if !sortedHas(declared, kind) {
			t.Errorf("%s answers for %q, which %s does not admit: a classification of a verb the "+
				"contract cannot produce is a stale entry", v.mapName, kind, v.schema)
		}
	}
}

// enum reads the sorted verb enum off the contract's own schema.
func (v offeredVerbs) enum(t *testing.T) []string {
	t.Helper()
	raw, err := gatekit.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	block := schemaBlock(t, string(raw), v.schema)
	for _, anchor := range v.anchors {
		at := strings.Index(block, anchor)
		if at < 0 {
			t.Fatalf("contract: %s has no %q", v.schema, anchor)
		}
		block = block[at+len(anchor):]
	}
	start := strings.Index(block, "enum: [")
	if start < 0 {
		t.Fatalf("contract: %s declares no enum after %v", v.schema, v.anchors)
	}
	end := strings.Index(block[start:], "]")
	if end < 0 {
		t.Fatalf("contract: the %s enum is unterminated", v.schema)
	}
	return splitEnumList(block[start+len("enum: [") : start+end])
}

// classification reads the map's keys and counts how many answer "this verb
// writes". It parses the source, because a text scan cannot see the values.
func (v offeredVerbs) classification(t *testing.T) (kinds map[string]bool, writing int) {
	t.Helper()
	file, err := gatekit.ParseFile(v.source, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", v.source, err)
	}
	kinds = map[string]bool{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			value, named := mapLiteral(spec, v.mapName)
			if !named {
				continue
			}
			for _, item := range value.Elts {
				pair, ok := item.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				kind := v.kindOf(t, pair.Key)
				writes := v.writes(t, v.mapName, kind, pair.Value)
				kinds[kind] = writes
				if writes {
					writing++
				}
			}
		}
	}
	if len(kinds) == 0 {
		t.Fatalf("no %s map literal found in %s: this gate reads the classification from there, "+
			"and an empty read agrees with every contract", v.mapName, v.source)
	}
	return kinds, writing
}

// mapLiteral picks the named classification map out of a var spec.
func mapLiteral(spec ast.Spec, name string) (*ast.CompositeLit, bool) {
	value, ok := spec.(*ast.ValueSpec)
	if !ok || len(value.Names) != 1 || value.Names[0].Name != name || len(value.Values) != 1 {
		return nil, false
	}
	literal, ok := value.Values[0].(*ast.CompositeLit)
	return literal, ok
}

// kindOf turns a `crmcontracts.ContactMomentActionKindLogActivity` key back
// into the contract's own `log_activity`.
func (v offeredVerbs) kindOf(t *testing.T, key ast.Expr) string {
	t.Helper()
	selector, ok := key.(*ast.SelectorExpr)
	if !ok || !strings.HasPrefix(selector.Sel.Name, v.constPrefix) {
		t.Fatalf("%s is keyed by %T rather than a %s* constant; this gate reads the contract's "+
			"spelling out of the constant name", v.mapName, key, v.constPrefix)
	}
	return snakeOf(strings.TrimPrefix(selector.Sel.Name, v.constPrefix))
}

// boolLiteral reads an entry whose answer is `true` or `false`.
func boolLiteral(t *testing.T, mapName, kind string, value ast.Expr) bool {
	t.Helper()
	answer, ok := value.(*ast.Ident)
	if !ok || (answer.Name != "true" && answer.Name != "false") {
		t.Fatalf("%s[%s] is %T rather than a bool literal; the classification has to be readable "+
			"without running it", mapName, kind, value)
	}
	return answer.Name == "true"
}

// refusalPhrase reads an entry whose answer is `nil` for a read, or the
// address of the refusal a writing verb gets.
func refusalPhrase(t *testing.T, mapName, kind string, value ast.Expr) bool {
	t.Helper()
	if ident, ok := value.(*ast.Ident); ok && ident.Name == "nil" {
		return false
	}
	if unary, ok := value.(*ast.UnaryExpr); ok && unary.Op == token.AND {
		return true
	}
	t.Fatalf("%s[%s] is %T rather than nil or &phrase; the classification has to be readable "+
		"without running it", mapName, kind, value)
	return false
}

// snakeOf turns `OpenMeetingBrief` into `open_meeting_brief`.
func snakeOf(pascal string) string {
	var out strings.Builder
	for i, r := range pascal {
		if i > 0 && unicode.IsUpper(r) {
			out.WriteByte('_')
		}
		out.WriteRune(unicode.ToLower(r))
	}
	return out.String()
}

// sortedHas reports membership in a sorted slice.
func sortedHas(sorted []string, want string) bool {
	i := sort.SearchStrings(sorted, want)
	return i < len(sorted) && sorted[i] == want
}
