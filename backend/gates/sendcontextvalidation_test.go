// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
//gate:kind census H3

package gates

// Every door that builds a send input runs the claim through the shared
// validator.
//
// The categories that serve the recipient are the ones a hard suppression does
// not stop, and they belong to the installation's own controller mail behind a
// registered template. A door able to claim one could dress marketing as a
// security warning and reach somebody who has objected. The HTTP doors are
// refused by sendContextFrom and the tool doors by ApplyContext /
// ApplyChannelContext — one validator, and this is what stops a third door from
// deciding for itself.
//
// Derived from the CHOKEPOINT rather than from a list of doors: a function that
// calls activities.Store.SendOrSchedule or SendMessage IS a send door, whatever
// it is called and wherever it lives, so one added tomorrow is judged the day it
// is written. A new door is judged by the walk itself; the floor below is a
// separate guard, against the walk losing doors it used to see.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// sendChokepoints are the two store methods every outbound message passes
// through. The corpus is derived from THEM rather than from a syntactic tell,
// because they are the owner this gate protects: a function calling one is
// sending a message, whatever shape it used to build the input and wherever it
// lives.
//
// An earlier version of this gate looked for a composite literal with a Context
// key inside two named directories. Three probes walked straight past it — a
// door in cmd/, an assignment after the literal, and a method that sets the
// field — so it defended the shape of the code that existed rather than the
// obligation.
// sendDoorFloor is the number of send doors the tree holds today: the three
// activities handlers (SendEmail, SendAccountEmail, SendMessage), the two
// commsAdapter methods the tool surface reaches them by, and one test driver.
//
// WHAT IT DOES AND DOES NOT CATCH. It fails when the walk stops recognising a
// door it used to see — a renamed field, a narrowed matcher — which is the way
// a census silently reports PASS over a corpus it no longer reads. It does NOT
// fail when a SEVENTH door appears through a shape the walk never covered: the
// count goes up, not down. Nothing here promises otherwise, and a door added
// through a covered shape is judged by the walk on its own.
//
// "Found at least one" would be too weak to notice a derivation that lost five
// of six, which is why this is a count and not a presence check.
const sendDoorFloor = 6

var sendChokepoints = map[string]bool{
	"SendOrSchedule": true,
	"SendMessage":    true,
}

// sendStoreType is the type whose SendOrSchedule and SendMessage this gate
// watches. The chokepoint names alone are not enough: "SendMessage" is also a
// method on the Telegram connector, on the fenced channel sender, on the
// dispatcher's own seam and on the generated HTTP wrapper, and none of those
// decodes a caller's claim. What this gate is about is the ACTIVITIES store,
// the one object that turns a request into an outbound message.
const sendStoreType = "activities.Store"

// sendStoreDir is the store's OWN package, where the type is written *Store
// with no qualifier. A gate matching only the qualified spelling recognises the
// handlers in this package by accident — through the h.store field — and would
// stop the day one of them used its receiver directly.
const sendStoreDir = "internal/modules/activities"

// sendStoreLocalType is that unqualified spelling.
const sendStoreLocalType = "Store"

// storeBindings holds every name an activities store reaches a call site under.
//
// DERIVED, not listed. An earlier version of this gate carried five hard-coded
// receiver spellings (store, s, h.store, c.store, t.comms), so a door holding
// the store in any other field — and the tree already names it activities,
// files and attachments elsewhere — dropped out of the corpus silently. A
// census that can fail short reports PASS over the door it never read, which is
// the one way this gate must not break. Proven before it was fixed: a compose
// door calling p.acts.SendOrSchedule and validating nothing passed the old gate.
//
// Three declaration shapes reach a store, and all three are read:
//
//   - a struct field, giving the "x.field" selections,
//   - a function parameter, receiver or var, giving a bare identifier,
//   - a short assignment from a function whose result is *activities.Store.
//
// Types are not resolved — that would need the whole program loaded — so this
// reads the DECLARED type as written, in four shapes: a named field, an
// EMBEDDED store (recorded under the type's own name, since it is reached as
// x.Store), a bare identifier from a parameter, receiver or var, and a local
// bound from a constructor's store-typed result.
//
// WHAT THIS STILL CANNOT SEE, stated rather than rationalised: a store held
// behind an interface, or returned by a method rather than a package-level
// function. sendDoorFloor does not close that gap — a floor notices a door
// LOST, not a door added through a shape the walk never covered. Closing it
// needs real type resolution, which needs the whole program loaded. If a send
// door ever appears behind an interface, this is the file that has to grow a
// types pass.
type storeBindings struct {
	bare      map[string]bool  // identifiers declared *activities.Store
	fields    map[string]bool  // struct field names of type *activities.Store
	returning map[string][]int // functions, and which of their results is a store
}

// holdsTheSendStore reports whether a type expression, as written, is
// *activities.Store or activities.Store.
func holdsTheSendStore(expr ast.Expr, inStorePackage bool) bool {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if id, ok := expr.(*ast.Ident); ok {
		return inStorePackage && id.Name == sendStoreLocalType
	}
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return pkg.Name+"."+sel.Sel.Name == sendStoreType
}

// collectStoreBindings reads the declared names before any call site is judged,
// because a store field declared in one file is called on in another and a
// local is bound from a constructor declared in a third.
func collectStoreBindings(files map[string]*ast.File) storeBindings {
	b := storeBindings{
		bare:      map[string]bool{},
		fields:    map[string]bool{},
		returning: map[string][]int{},
	}
	for path, file := range files {
		local := strings.HasPrefix(path, sendStoreDir)
		ast.Inspect(file, func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.StructType:
				b.readFields(v, local)
			case *ast.FuncDecl:
				b.readSignature(v, local)
			case *ast.ValueSpec:
				if v.Type != nil && holdsTheSendStore(v.Type, local) {
					b.bindNames(v.Names)
				}
			}
			return true
		})
	}
	// A second pass, because a short assignment reads a constructor declared
	// anywhere in the tree and the first pass is what learns those names.
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			if assign, ok := n.(*ast.AssignStmt); ok && assign.Tok == token.DEFINE {
				b.readShortAssign(assign)
			}
			return true
		})
	}
	return b
}

// readFields records the struct fields that hold a store.
//
// An EMBEDDED store has no field name and is reached as x.Store, so the type's
// own name is recorded as the selector. A named field of a type that itself
// holds a store — one level deeper, x.inner.st — is reached through that
// field's own entry, because callsTheSendStore asks about the selector base's
// LAST name rather than the whole path.
func (b storeBindings) readFields(st *ast.StructType, local bool) {
	for _, f := range st.Fields.List {
		if !holdsTheSendStore(f.Type, local) {
			continue
		}
		if len(f.Names) == 0 {
			b.fields[sendStoreLocalType] = true
			continue
		}
		for _, name := range f.Names {
			b.fields[name.Name] = true
		}
	}
}

// readSignature records the receivers and parameters that carry a store, and
// the functions that hand one back.
func (b storeBindings) readSignature(fn *ast.FuncDecl, local bool) {
	fields := []*ast.Field{}
	if fn.Recv != nil {
		fields = append(fields, fn.Recv.List...)
	}
	if fn.Type.Params != nil {
		fields = append(fields, fn.Type.Params.List...)
	}
	for _, f := range fields {
		if holdsTheSendStore(f.Type, local) {
			b.bindNames(f.Names)
		}
	}
	if fn.Type.Results == nil {
		return
	}
	// Result POSITION, counted the way a call site unpacks them: a field with
	// several names occupies several positions.
	pos := 0
	for _, r := range fn.Type.Results.List {
		width := max(len(r.Names), 1)
		if holdsTheSendStore(r.Type, local) {
			for i := range width {
				b.returning[fn.Name.Name] = append(b.returning[fn.Name.Name], pos+i)
			}
		}
		pos += width
	}
}

// readShortAssign binds "store := sendStore(...)" and its multi-value form, so
// a local reaching the chokepoint is recognised by where it came from.
//
// A single call filling several names binds only the POSITIONS whose declared
// result is a store: heldSendActors returns (*activities.Store, scheduleTimer,
// bool), and binding all three would make the timer and the ok flag look like
// stores. That is not a safe over-approximation — it is a gate reporting the
// wrong receiver for a finding, and the next author would not believe it.
func (b storeBindings) readShortAssign(assign *ast.AssignStmt) {
	for i, rhs := range assign.Rhs {
		call, ok := rhs.(*ast.CallExpr)
		if !ok {
			continue
		}
		id, ok := call.Fun.(*ast.Ident)
		if !ok {
			continue
		}
		positions, returns := b.returning[id.Name]
		if !returns {
			continue
		}
		if len(assign.Rhs) == 1 {
			for _, pos := range positions {
				if pos < len(assign.Lhs) {
					b.bindAssignedNames(assign.Lhs[pos : pos+1])
				}
			}
			continue
		}
		if i < len(assign.Lhs) && len(positions) > 0 {
			b.bindAssignedNames(assign.Lhs[i : i+1])
		}
	}
}

func (b storeBindings) bindNames(names []*ast.Ident) {
	for _, name := range names {
		if name.Name != "_" {
			b.bare[name.Name] = true
		}
	}
}

func (b storeBindings) bindAssignedNames(exprs []ast.Expr) {
	for _, e := range exprs {
		if name, ok := e.(*ast.Ident); ok && name.Name != "_" {
			b.bare[name.Name] = true
		}
	}
}

// callsTheSendStore reports whether this selector is a chokepoint called on
// something declared to hold the activities store.
func (b storeBindings) callsTheSendStore(sel *ast.SelectorExpr) bool {
	if !sendChokepoints[sel.Sel.Name] {
		return false
	}
	// The base's LAST name is what names the holder: "z.inner.st" is reached
	// through st, which the nested struct declared. Reading the whole path
	// instead would need one entry per route to the same field.
	switch v := sel.X.(type) {
	case *ast.Ident:
		return b.bare[v.Name] || b.fields[v.Name]
	case *ast.SelectorExpr:
		return b.fields[v.Sel.Name]
	}
	return false
}

// validators are the entry points that apply the shared refusals. A caller
// reaching any of them has asked the one validator; sendContextFrom is the HTTP
// spelling, the two Apply… functions the typed one, and the two decoders wrap
// sendContextFrom for the handlers.
var validators = map[string]bool{
	"sendContextFrom":     true,
	"ApplyContext":        true,
	"ApplyChannelContext": true,
	"replySendInput":      true,
	"accountSendInput":    true,
}

// forwardsAnAlreadyValidatedInput are the callers that take a send input as a
// PARAMETER rather than building one from a caller's claim. They cannot
// validate what they did not decode, and requiring them to would ask the same
// claim to be judged twice.
//
// Keyed on the exact function, so moving the call re-arms the gate rather than
// inheriting the exemption. Each entry names why it is safe.
var forwardsAnAlreadyValidatedInput = map[string]bool{
	// A test-only driver: it takes the send input its caller already built, so
	// there is no claim here to decode.
	"internal/compose/commsscheduled_integration.go:ScheduleAsAgentForTest": true,
	// The store's own methods are the chokepoint, not callers of it.
	"internal/modules/activities/sendcore.go:Store.SendOrSchedule": true,
	"internal/modules/activities/channelsend.go:Store.SendMessage": true,
}

func TestEverySendDoorValidatesTheClaimedContext(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	// The whole backend tree, because a send door is wherever somebody calls
	// the chokepoint — cmd/ included, which already owns an outbound mail lane.
	files := map[string]*ast.File{}
	for _, root := range []string{"internal", "cmd"} {
		for path, file := range parseTreeFiles(t, fset, root) {
			files[path] = file
		}
	}

	bindings := collectStoreBindings(files)
	if len(bindings.fields)+len(bindings.bare) == 0 {
		t.Fatal("no identifier in the tree is declared to hold an activities store — " +
			"the type spelling this gate derives its corpus by has moved")
	}

	sending := map[string]bool{}    // functions that send a message
	validating := map[string]bool{} // functions whose send carries a validated input
	for path, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			name := path + ":" + receiverQualified(fn)
			calls := bindings.sendCallsIn(fn)
			if len(calls) == 0 {
				continue
			}
			sending[name] = true
			if sendsAValidatedInput(fn, calls) {
				validating[name] = true
			}
		}
	}

	// Under-recognition is the one way this gate must not break: a scan that
	// found nothing would report PASS over an empty corpus.
	if len(sending) < sendDoorFloor {
		t.Fatalf("found %d send doors, fewer than the %d the tree holds. Either a door was "+
			"deleted — lower sendDoorFloor to match — or the derivation stopped recognising "+
			"one, in which case an unwatched door is now sending without validating its "+
			"caller's claim", len(sending), sendDoorFloor)
	}
	for door := range sending {
		if forwardsAnAlreadyValidatedInput[door] {
			continue
		}
		if !validating[door] {
			t.Errorf("%s sends a message whose claimed context no validator produced — "+
				"a door that judges its own claims can name a category reserved for controller "+
				"mail. Every send in the function must carry a validated input, not just one of "+
				"them", door)
		}
	}
}

// The tool surface declares the four context arguments in exactly one place.
//
// A second copy is how one send tool starts advertising a category, a bound or
// a description the others do not — the two transports then disagree about one
// behaviour, which is exactly what TestEveryToolEnumMatchesTheContractItMirrors
// catches at the contract level and cannot catch between two tool schemas.
//
// Held by this test for the claim on sendContextProperties.
func TestTheToolSurfaceSpellsTheSendContextOnce(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	var declarations []string
	for path, file := range parseTreeFiles(t, fset, "internal/modules/agents") {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, name := range value.Names {
					if strings.Contains(literalOf(value), `"communication_context"`) {
						declarations = append(declarations, path+":"+name.Name)
					}
				}
			}
		}
	}
	if len(declarations) == 0 {
		t.Fatal("found no send-context schema in the tool surface — the scan is looking in the wrong place")
	}
	if len(declarations) > 1 {
		t.Errorf("the send context is declared %d times (%s) — a second copy is how two send tools "+
			"start advertising different categories", len(declarations), strings.Join(declarations, ", "))
	}
}

// sendCallsIn returns the chokepoint calls one function makes.
func (b storeBindings) sendCallsIn(fn *ast.FuncDecl) []*ast.CallExpr {
	var out []*ast.CallExpr
	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && b.callsTheSendStore(sel) {
			out = append(out, call)
		}
		return true
	})
	return out
}

// sendsAValidatedInput reports whether the value handed to the chokepoint is
// one a validator produced.
//
// Asked of the ARGUMENT, not of the function. An earlier version set the flag
// whenever a validator's name appeared anywhere in the body, in any position —
// so a door that named a validator inside an `if false` and then sent an
// unvalidated input passed. Naming a validator is not calling one, and calling
// one is not sending what it returned.
//
// Three shapes carry a validated input, and they are the three the tree uses:
// the result passed straight through (`in, err := replySendInput(req)`, then
// `in`), the validator inlined into the argument, and the result read field by
// field into the input literal (`Context: claimed.category`), which is what the
// channel handler does.
func sendsAValidatedInput(fn *ast.FuncDecl, calls []*ast.CallExpr) bool {
	validated := namesFromValidators(fn)
	for _, call := range calls {
		if !oneCallCarriesAValidatedInput(call, validated) {
			return false
		}
	}
	return true
}

// oneCallCarriesAValidatedInput asks the question of a SINGLE send.
//
// Every send in a function must carry one. Asked existentially — "does this
// function validate anywhere" — a door could validate its first message and
// send its second raw, and the gate would report the function as covered.
func oneCallCarriesAValidatedInput(call *ast.CallExpr, validated map[string]bool) bool {
	for _, arg := range call.Args {
		if readsAValidatedValue(arg, validated) {
			return true
		}
	}
	return false
}

// readsAValidatedValue reports whether an argument is, or is built from, a
// value a validator returned.
func readsAValidatedValue(arg ast.Expr, validated map[string]bool) bool {
	found := false
	ast.Inspect(arg, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.Ident:
			if validated[v.Name] {
				found = true
			}
		case *ast.CallExpr:
			if validators[calleeName(v)] {
				found = true
			}
		}
		return !found
	})
	return found
}

// namesFromValidators collects the names a validator's result was assigned to.
func namesFromValidators(fn *ast.FuncDecl) map[string]bool {
	out := map[string]bool{}
	ast.Inspect(fn, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Rhs) != 1 {
			return true
		}
		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok || !validators[calleeName(call)] {
			return true
		}
		// The validator returns (input, error); the input is first.
		if id, ok := assign.Lhs[0].(*ast.Ident); ok && id.Name != "_" {
			out[id.Name] = true
		}
		return true
	})
	return out
}

// literalOf concatenates a const spec's string parts, so a schema built by
// joining several literals is read as the one string it becomes.
func literalOf(spec *ast.ValueSpec) string {
	var out strings.Builder
	for _, v := range spec.Values {
		ast.Inspect(v, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				out.WriteString(lit.Value)
			}
			return true
		})
	}
	return out.String()
}

// parseTreeFiles parses every hand-written Go file under a directory, keyed by
// path so a finding can say where the door lives.
func parseTreeFiles(t *testing.T, fset *token.FileSet, root string) map[string]*ast.File {
	t.Helper()
	out := map[string]*ast.File{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		out[path] = file
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return out
}
