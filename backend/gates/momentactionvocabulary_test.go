// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// Every moment-card verb is classified by whether pressing it writes.
//
// compose/momentaction decides which verbs a caller without `activity.create`
// is offered, by looking the action's kind up in one map. A kind MISSING from
// that map reads as "writes nothing" — Go cannot tell an absent key from a
// false one — so an unclassified verb is handed to a reader as live and its
// save is then refused with nothing said. That is the defect the card exists
// to prevent, and it arrives by addition: the vocabulary grew a second writing
// kind once already, and the second one was added with the identical shape.
//
// So the contract's own enum is the corpus and the map must answer all of it.
// Both sides are read here — the enum from api/crm.yaml, the keys from the
// source — so neither can be the one this gate believes.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"
	"unicode"
)

const (
	momentActionSchema = "PersonMomentAction"
	momentActionSource = "internal/compose/momentaction/withhold.go"
	momentActionMap    = "writesAnActivity"
	// The constant prefix oapi-codegen mints for this enum, stripped to get
	// back to the contract's own spelling.
	momentActionConstPrefix = "PersonMomentActionKind"
)

func TestEveryMomentActionKindIsClassifiedAsWritingOrNot(t *testing.T) {
	t.Parallel()
	declared := momentActionKindEnum(t)
	// The enum is eight members today. A floor rather than an equality: the
	// vocabulary is meant to grow, and what must not happen is this gate
	// reading a SHORTER one than the contract carries — an extraction that
	// silently matched nothing would otherwise agree with an empty map.
	if len(declared) < 8 {
		t.Fatalf("read %d kinds from %s.kind in api/crm.yaml (%v) — the enum has at least eight, "+
			"so the extraction has gone short and this gate would pass vacuously",
			len(declared), momentActionSchema, declared)
	}

	classified, writing := momentActionClassification(t)
	// Both dispositions must be populated. A map read as all-false would still
	// satisfy set equality while withholding nothing at all.
	if writing < 2 {
		t.Errorf("%s classifies %d kinds as writing an activity, want at least the log form and the "+
			"task form — a vocabulary where nothing writes withholds nothing", momentActionMap, writing)
	}

	for _, kind := range declared {
		if _, ok := classified[kind]; !ok {
			t.Errorf("%s.kind admits %q and %s does not answer for it, so it reads as writing nothing: "+
				"a reader without activity.create would be handed it live and refused on save. Add it to "+
				"the map with the answer for that verb", momentActionSchema, kind, momentActionMap)
		}
	}
	for kind := range classified {
		if !sortedHas(declared, kind) {
			t.Errorf("%s answers for %q, which %s.kind does not admit — a classification of a verb the "+
				"contract cannot produce is a stale entry", momentActionMap, kind, momentActionSchema)
		}
	}
}

// momentActionKindEnum reads the sorted `kind` enum off the contract's own
// PersonMomentAction schema.
func momentActionKindEnum(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	block := schemaBlock(t, string(raw), momentActionSchema)
	// The first flow-list enum inside the block is `kind`'s: it is the schema's
	// first enum property, and `state`'s follows it.
	start := strings.Index(block, "enum: [")
	if start < 0 {
		t.Fatalf("contract: %s declares no enum", momentActionSchema)
	}
	end := strings.Index(block[start:], "]")
	if end < 0 {
		t.Fatalf("contract: %s.kind enum is unterminated", momentActionSchema)
	}
	return splitEnumList(block[start+len("enum: [") : start+end])
}

// momentActionClassification reads the classification map's keys and counts
// how many of them answer "this verb writes".
//
// It parses rather than greps because the map's VALUES are the half a text
// scan cannot see, and a gate that checked only the keys would pass a map that
// had quietly answered false for the log form.
func momentActionClassification(t *testing.T) (kinds map[string]bool, writing int) {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), momentActionSource, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", momentActionSource, err)
	}
	kinds = map[string]bool{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			value, named := momentActionMapLiteral(spec)
			if !named {
				continue
			}
			for _, item := range value.Elts {
				pair, ok := item.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				kind := momentActionKindOf(t, pair.Key)
				writes := momentActionWritesOf(t, kind, pair.Value)
				kinds[kind] = writes
				if writes {
					writing++
				}
			}
		}
	}
	if len(kinds) == 0 {
		t.Fatalf("no %s map literal found in %s — this gate reads the classification from there, "+
			"and an empty read agrees with every contract", momentActionMap, momentActionSource)
	}
	return kinds, writing
}

// momentActionMapLiteral picks the classification map out of a var spec.
func momentActionMapLiteral(spec ast.Spec) (*ast.CompositeLit, bool) {
	value, ok := spec.(*ast.ValueSpec)
	if !ok || len(value.Names) != 1 || value.Names[0].Name != momentActionMap || len(value.Values) != 1 {
		return nil, false
	}
	literal, ok := value.Values[0].(*ast.CompositeLit)
	return literal, ok
}

// momentActionKindOf turns a `crmcontracts.PersonMomentActionKindLogActivity`
// key back into the contract's own `log_activity`.
func momentActionKindOf(t *testing.T, key ast.Expr) string {
	t.Helper()
	selector, ok := key.(*ast.SelectorExpr)
	if !ok || !strings.HasPrefix(selector.Sel.Name, momentActionConstPrefix) {
		t.Fatalf("%s is keyed by %T rather than a %s* constant; this gate reads the contract's "+
			"spelling out of the constant name", momentActionMap, key, momentActionConstPrefix)
	}
	return snakeOf(strings.TrimPrefix(selector.Sel.Name, momentActionConstPrefix))
}

// momentActionWritesOf reads the map entry's own answer.
func momentActionWritesOf(t *testing.T, kind string, value ast.Expr) bool {
	t.Helper()
	answer, ok := value.(*ast.Ident)
	if !ok || (answer.Name != "true" && answer.Name != "false") {
		t.Fatalf("%s[%s] is %T rather than a bool literal; the classification has to be readable "+
			"without running it", momentActionMap, kind, value)
	}
	return answer.Name == "true"
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
