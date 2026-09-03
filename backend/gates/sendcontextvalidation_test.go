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
// Derived from the input types rather than from a list of doors: a function
// that constructs an activities.SendEmailInput or SendMessageInput IS a send
// door, whatever it is called and wherever it lives, so one added tomorrow is
// judged the day it is written.

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
var sendChokepoints = map[string]bool{
	"SendOrSchedule": true,
	"SendMessage":    true,
}

// sendStoreReceivers are the expressions the chokepoint is called ON. Bound
// because "SendMessage" is a common method name — the Telegram connector, the
// fenced channel sender, the dispatcher's own seam and the generated HTTP
// wrapper all have one, and none of them decodes a caller's claim. What this
// gate is about is the ACTIVITIES store, the one object that turns a request
// into an outbound message.
//
// Matched on the selector's base expression rather than by resolving types,
// because a gate that needed type information would need the whole program
// loaded. The names below are what the store is called at its call sites.
var sendStoreReceivers = map[string]bool{
	"store": true, "s": true, "h.store": true, "c.store": true, "t.comms": true,
}

// callsTheSendStore reports whether this selector is the chokepoint called on
// something that looks like the activities store.
func callsTheSendStore(sel *ast.SelectorExpr) bool {
	if !sendChokepoints[sel.Sel.Name] {
		return false
	}
	return sendStoreReceivers[receiverText(sel.X)]
}

// receiverText renders the expression a method was called on, for the two shapes a
// store reaches a call site in: a bare identifier and one field selection.
func receiverText(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		if base, ok := v.X.(*ast.Ident); ok {
			return base.Name + "." + v.Sel.Name
		}
	}
	return ""
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
	// Test-only drivers: they take the send input their caller already built,
	// so there is no claim here to decode.
	"internal/compose/commsscheduled_integration.go:DriveScheduledSendForTest": true,
	"internal/compose/commsscheduled_integration.go:ScheduleAsAgentForTest":    true,
	// The tool hands its raw arguments to commsAdapter.SendMessage, which is
	// what runs ApplyChannelContext. Validating here too would decode the same
	// claim twice, and the adapter is the seam every channel caller shares.
	"internal/modules/agents/tools_comms.go:sendMessageTool.Handle": true,
	// The store's own methods are the chokepoint, not callers of it.
	"internal/modules/activities/sendcore.go:Store.SendOrSchedule": true,
	"internal/modules/activities/channelsend.go:Store.SendMessage": true,
}

func TestEverySendDoorValidatesTheClaimedContext(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	sending := map[string]bool{}    // functions that send a message
	validating := map[string]bool{} // functions that reach a validator

	// The whole backend tree, because a send door is wherever somebody calls
	// the chokepoint — cmd/ included, which already owns an outbound mail lane.
	for _, root := range []string{"internal", "cmd"} {
		for path, file := range parseTreeFiles(t, fset, root) {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				name := path + ":" + receiverQualified(fn)
				ast.Inspect(fn, func(n ast.Node) bool {
					sel, ok := n.(*ast.SelectorExpr)
					if !ok {
						if id, ok := n.(*ast.Ident); ok && validators[id.Name] {
							validating[name] = true
						}
						return true
					}
					if callsTheSendStore(sel) {
						sending[name] = true
					}
					if validators[sel.Sel.Name] {
						validating[name] = true
					}
					return true
				})
			}
		}
	}

	// Under-recognition is the one way this gate must not break: a scan that
	// found nothing would report PASS over an empty corpus.
	if len(sending) == 0 {
		t.Fatal("found no send call — the scan is looking in the wrong place")
	}
	for door := range sending {
		if forwardsAnAlreadyValidatedInput[door] {
			continue
		}
		if !validating[door] {
			t.Errorf("%s sends a message and never validates the claimed context — "+
				"a door that judges its own claims can name a category reserved for controller mail", door)
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
