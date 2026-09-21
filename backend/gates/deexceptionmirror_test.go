// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The engine's test fixture for the German exception is the pack's declaration,
// or the tests prove nothing about what ships.
//
// UWG §7(3) permits advertising to an existing customer only where all four of
// its conditions hold, and extensions/de declares all four. The evaluator that
// reads them lives in internal/modules/consent, and its unit tests hand it a
// MarketingException built in the test file — because the schema will not store
// the failing shape for one condition, so the only way to ask what the code does
// with it is to pass the values directly.
//
// That fixture calls itself "what extensions/de declares" and nothing checked
// it. The pack cannot be imported to remove the copy: extensions/de depends on
// backend, so backend reading it back would be a cycle. So the copy is a
// declared MIRROR, and this is what makes "mirror" true — it compares the two
// literals in both directions, so a condition added to the pack and not the
// fixture fails, and so does one relaxed in the fixture and not the pack.
//
// Why it matters that the fixture is right: relax a condition there and the
// evaluator's tests go on passing while asserting a §7(3) the statute does not
// grant. A test that models a laxer law than the one shipped is worse than no
// test, because it reports coverage of the case it stopped checking.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"slices"
	"testing"

	"github.com/margince/margince/backend/pkg/extension/messaging"
)

const (
	dePackSource     = "../extensions/de/de.go"
	consentFixture   = "internal/modules/consent/existingcustomer_test.go"
	exceptionLiteral = "MarketingException"
	// The function on the consent side whose literal IS the mirror. Named
	// rather than taken by position: the fixture file exists to exercise
	// conditions one at a time, so a relaxed variant written there as a literal
	// is an ordinary thing for it to grow — and a gate reading whichever came
	// first would then compare a struct nobody ships and report agreement.
	// Today the file holds one literal and builds its relaxed case by copying
	// it, so this changes nothing yet; it is what keeps the subject stable.
	fixtureFunc = "germanConditions"
	// And the function on the pack side, for the same reason. Reading the whole
	// file merged every MarketingException literal in it into one map, last
	// writer wins — so a relaxed shipped exception plus an honest reference
	// literal elsewhere in the file compared as agreeing.
	packFunc = "messagingRules"
	// The pack's constructor, which is what compose calls.
	packConstructor = "New"
)

func TestTheGermanExceptionFixtureMirrorsThePack(t *testing.T) {
	t.Parallel()

	// The pack side is read from the function New() wires, and that wiring is
	// asserted here rather than assumed: this gate names a FILE and a FUNCTION,
	// so a pack that moved its shipped declaration elsewhere would leave this
	// comparing a literal nobody registers. extensions/de's own test reads
	// New().Messaging and fails on exactly that move; this checks the two are
	// still the same function, which is what makes reading the file enough.
	if !packWires(t, packFunc) {
		t.Fatalf("%s no longer builds its Messaging from %s, so this gate is comparing a literal "+
			"that may not be the one compose registers. Point packFunc at whatever New() wires now",
			dePackSource, packFunc)
	}

	declared := marketingExceptionFields(t, dePackSource, packFunc)
	if len(declared) == 0 {
		t.Fatalf("%s builds no %s in %s, so this gate has no subject and would pass however far "+
			"the engine's fixture had drifted from the pack. If the pack's declaration moved, "+
			"point packFunc at it", dePackSource, exceptionLiteral, packFunc)
	}

	fixture := marketingExceptionFields(t, consentFixture, fixtureFunc)
	if len(fixture) == 0 {
		t.Fatalf("%s builds no %s in %s — the evaluator's tests no longer model the pack's "+
			"exception there, so nothing says what they are asserting about. If the fixture "+
			"moved, point fixtureFunc at it rather than letting this gate read a different one",
			consentFixture, exceptionLiteral, fixtureFunc)
	}

	for field, want := range declared {
		got, named := fixture[field]
		if !named {
			t.Errorf("extensions/de requires %s and the engine's fixture does not set it: the "+
				"evaluator's tests are modelling a §7(3) with one fewer condition than the pack "+
				"grants, and would keep passing if the engine stopped reading it", field)
			continue
		}
		if got != want {
			t.Errorf("%s is %s in extensions/de and %s in the engine's fixture — the tests are "+
				"asserting about an exception this installation does not ship", field, want, got)
		}
	}
	// A field on the type that NEITHER side states. Both would agree by
	// omission, and the §7(3) condition it represents would be unchecked in the
	// engine and unnoticed here — the census failing short over a field nobody
	// wrote. Read from the type, so a field added tomorrow is covered the day
	// it is added.
	for _, field := range exceptionFields() {
		if _, stated := declared[field]; !stated {
			t.Errorf("messaging.MarketingException carries %s and extensions/de states it nowhere. "+
				"An unstated condition is one the engine is not asked to check: set it in the "+
				"pack, or say in the type why Germany does not require it", field)
		}
	}

	for field := range fixture {
		if _, declared := declared[field]; !declared {
			t.Errorf("the engine's fixture sets %s, which extensions/de does not declare. Either "+
				"the pack dropped a condition and the fixture kept it — so the tests assert a "+
				"stricter rule than ships — or the fixture named a field the pack never had",
				field)
		}
	}
}

// packWires reports whether the pack's New() builds its Messaging field by
// calling the named function.
//
// Scoped to New itself, not to the file: a helper or dead literal elsewhere
// carrying a Messaging key would otherwise satisfy this while New shipped
// something else.
//
// What it CANNOT do is follow the value. New assigning a local slice, appending
// to one, or calling through a wrapper all read here as unwired, and this fails
// closed on each — loudly, naming the function to repoint, rather than passing
// over a shape it did not understand. That is the right direction for a check
// whose whole job is to say "the literal below is the one that ships": a false
// alarm costs a line of this gate, and a false pass costs the §7(3) conditions
// nobody is checking. extensions/de's own test reads New().Messaging and holds
// the value side, which is the half source-reading cannot reach.
func packWires(t *testing.T, fn string) bool {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, dePackSource, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", dePackSource, err)
	}

	var constructor *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == packConstructor {
			constructor = fn
			break
		}
	}
	if constructor == nil {
		t.Fatalf("%s declares no %s, so this gate cannot tell which rules the pack ships",
			dePackSource, packConstructor)
	}

	wired := false
	ast.Inspect(constructor, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		if key, ok := kv.Key.(*ast.Ident); !ok || key.Name != "Messaging" {
			return true
		}
		ast.Inspect(kv.Value, func(inner ast.Node) bool {
			call, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}
			if name, ok := call.Fun.(*ast.Ident); ok && name.Name == fn {
				wired = true
			}
			return true
		})
		return true
	})
	return wired
}

// marketingExceptionFields reads the MarketingException composite literals in a
// file and returns the boolean fields they set, by name.
//
// within scopes the read to one function when given, which is how the consent
// side names its mirror: that file builds a second, deliberately relaxed
// exception for a test of one condition, and comparing THAT against the pack
// would report a disagreement the product does not have. The pack side passes
// "" because its file declares exactly one, which the caller asserts.
//
// Read from the SOURCE of both sides rather than by building either: the pack
// is a separate module this one cannot import, and the point is to compare two
// declarations rather than to trust one of them.
//
// Both the boolean conditions and Kind are read. Kind is the exception's
// identity rather than one of its conditions, but it is compared here for the
// same reason the conditions are: the pack side is asserted by extensions/de's
// own test and the FIXTURE side is asserted by nothing else, so leaving it out
// would let the evaluator's tests model an exception of a different kind
// entirely while every test in the tree stayed green.
func marketingExceptionFields(t *testing.T, path, within string) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}

	var root ast.Node = file
	if within != "" {
		root = nil
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == within {
				root = fn
				break
			}
		}
		if root == nil {
			t.Fatalf("%s declares no %s, so this gate cannot find the mirror it compares — "+
				"point fixtureFunc at wherever the fixture moved", path, within)
		}
	}

	// A field ASSIGNED after the literal is built would leave this reading the
	// pre-mutation value — the literal says one thing and the function returns
	// another. Refused rather than followed: this gate reads declarations, and
	// a function that mutates its exception is no longer a declaration.
	ast.Inspect(root, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, target := range assign.Lhs {
			sel, ok := target.(*ast.SelectorExpr)
			if !ok || !exceptionFieldName(sel.Sel.Name) {
				continue
			}
			t.Errorf("%s assigns %s after building the exception, so the literal this gate reads "+
				"is not what the function returns. Set the field in the literal, or this "+
				"comparison is about a struct nobody uses", path, sel.Sel.Name)
		}
		return true
	})

	fields := map[string]string{}
	ast.Inspect(root, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok || !namesMarketingException(lit) {
			return true
		}
		// Inside a slice of them the element type is ELIDED — the pack writes
		// `[]messaging.MarketingException{{Kind: …}}` — so the elements carry
		// no type of their own and are read here from the slice's.
		elements := lit.Elts
		if isExceptionSlice(lit) {
			elements = nil
			for _, element := range lit.Elts {
				inner, ok := element.(*ast.CompositeLit)
				if !ok {
					continue
				}
				elements = append(elements, inner.Elts...)
			}
		}
		for _, element := range elements {
			kv, ok := element.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			value, readable := literalValue(kv.Value)
			if !readable {
				t.Errorf("%s sets %s to something this gate cannot read as a written-out value. "+
					"Both sides are compared as source, so a constant or an expression could "+
					"name the same thing and mean two different ones — write the value out",
					path, key.Name)
				continue
			}
			fields[key.Name] = value
		}
		return true
	})
	return fields
}

// literalValue renders the value a field is set to, and refuses anything that is
// not written out in full.
//
// This gate diffs two SOURCES, so it compares spellings: `true` against `true`,
// `messaging.ExistingCustomer` against the same. A field set to a named constant
// would compare by that name, and two files can define one differently — so a
// name this gate does not recognise is refused rather than compared, and the
// caller says so. Reading the meaning instead would mean type-checking two
// modules, one of which this one cannot import.
//
// Refusing is the safe direction: an unreadable field fails the gate and costs
// somebody a spelling, where comparing it could agree about a value neither side
// holds.
func literalValue(expr ast.Expr) (string, bool) {
	switch value := expr.(type) {
	case *ast.Ident:
		if value.Name == "true" || value.Name == "false" {
			return value.Name, true
		}
	case *ast.SelectorExpr:
		// A qualified constant, kept whole: `messaging.ExistingCustomer` and
		// some other package's ExistingCustomer are different values, and the
		// bare selector name would read them as the same.
		if pkg, ok := value.X.(*ast.Ident); ok {
			return pkg.Name + "." + value.Sel.Name, true
		}
	}
	return "", false
}

// exceptionFields names every field on the exception type. Derived rather than
// listed, so a field added to messaging.MarketingException is covered the day it
// is added — which is the whole point of asking the type instead of a copy.
func exceptionFields() []string {
	typ := reflect.TypeOf(messaging.MarketingException{})
	names := make([]string, 0, typ.NumField())
	for i := range typ.NumField() {
		names = append(names, typ.Field(i).Name)
	}
	return names
}

// exceptionFieldName reports whether a name is one of the exception's own
// fields.
func exceptionFieldName(name string) bool {
	return slices.Contains(exceptionFields(), name)
}

// namesMarketingException reports whether a composite literal is one, or a
// slice of them. The fixture writes it directly; the pack writes a slice whose
// elements elide their type.
func namesMarketingException(lit *ast.CompositeLit) bool {
	return exceptionTypeName(lit.Type) == exceptionLiteral || isExceptionSlice(lit)
}

func isExceptionSlice(lit *ast.CompositeLit) bool {
	slice, ok := lit.Type.(*ast.ArrayType)
	return ok && exceptionTypeName(slice.Elt) == exceptionLiteral
}

// exceptionTypeName names the type an expression denotes, qualified or bare.
func exceptionTypeName(expr ast.Expr) string {
	switch typ := expr.(type) {
	case *ast.SelectorExpr:
		return typ.Sel.Name
	case *ast.Ident:
		return typ.Name
	}
	return ""
}
