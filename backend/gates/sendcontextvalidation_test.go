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

// sendInputTypes are the two structs a send door fills in. Named by their bare
// type name because a composite literal writes them either qualified
// (activities.SendEmailInput, from compose) or bare (from inside the module).
var sendInputTypes = map[string]bool{
	"SendEmailInput":   true,
	"SendMessageInput": true,
}

// validators are the entry points that apply the shared refusals. A door
// reaching any of them has asked the one validator; sendContextFrom is the
// HTTP spelling, the two Apply… functions the typed one.
var validators = map[string]bool{
	"sendContextFrom":     true,
	"ApplyContext":        true,
	"ApplyChannelContext": true,
	// The two HTTP decoders wrap sendContextFrom and are what the handlers
	// actually call, so a handler reaching one has reached the validator.
	"replySendInput":   true,
	"accountSendInput": true,
}

// revalidationExempt is the one shape that restores a claim rather than
// accepting one. thaw reads back what freezePayload wrote, and what it wrote
// was validated at schedule time by the door that took it — so re-running the
// refusals here would judge the same claim twice and, worse, would make a
// scheduled send fail at FIRE time for a claim the rep was told was accepted
// hours earlier, with nobody at the keyboard to correct it.
//
// Keyed on the exact function, so moving the restore somewhere else re-arms
// the gate rather than inheriting the exemption.
var revalidationExempt = map[string]bool{
	"internal/modules/activities/scheduledpayload.go:scheduledPayload.thaw": true,
}

func TestEverySendDoorValidatesTheClaimedContext(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	building := map[string]string{}    // doors that set a context field themselves
	buildsInput := map[string]string{} // every function that builds a send input
	readsClaim := map[string]bool{}    // functions that read the caller's claim
	validating := map[string]bool{}    // functions that reach a validator

	for _, root := range []string{"internal/compose", "internal/modules/activities"} {
		for path, file := range parseTreeFiles(t, fset, root) {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				name := path + ":" + receiverQualified(fn)
				ast.Inspect(fn, func(n ast.Node) bool {
					switch node := n.(type) {
					case *ast.CompositeLit:
						// A door is a function that puts a claim ON a send
						// input: it either fills a context field itself, or it
						// builds an input in a function that also reads the
						// caller's claim. The three that rebuild an
						// already-validated input — the held-draft release, the
						// scheduled thaw and its replay — do neither, and a gate
						// that failed them would ask them to judge a claim
						// nobody made twice.
						if sendInputTypes[bareTypeName(literalTypeName(node))] {
							buildsInput[name] = path
						}
						if sendInputTypes[bareTypeName(literalTypeName(node))] && setsAContextField(node) {
							building[name] = path
						}
					case *ast.SelectorExpr:
						if validators[node.Sel.Name] {
							validating[name] = true
						}
						// Reading the caller's own claim is what makes a
						// function a door. A door that then hands the input
						// straight to the validator sets no field itself, so
						// the composite-literal test alone cannot see it.
						if node.Sel.Name == claimArgsField {
							readsClaim[name] = true
						}
					case *ast.Ident:
						if validators[node.Name] {
							validating[name] = true
						}
					}
					return true
				})
			}
		}
	}

	// Under-recognition is the one way this gate must not break: a scan that
	// found nothing would report PASS over an empty corpus.
	if len(building) == 0 {
		t.Fatal("found no send input construction — the scan is looking in the wrong place")
	}
	// Both shapes of door, judged the same way.
	for door, path := range buildsInput {
		if readsClaim[door] {
			building[door] = path
		}
	}
	for door := range building {
		if revalidationExempt[door] {
			continue
		}
		if !validating[door] {
			t.Errorf("%s builds a send input and never validates the claimed context — "+
				"a door that judges its own claims can name a category reserved for controller mail", door)
		}
	}
}

// claimArgsField is the embedded struct a transport reads a caller's claim out
// of. A function naming it is accepting a claim, whatever it does next.
const claimArgsField = "SendContextArgs"

// contextFields are the four a caller's claim lands in. A literal setting any
// of them is accepting a claim and owes it the validator.
var contextFields = map[string]bool{
	"Context": true, "MarketingPurpose": true, "OperatorReason": true, "Evidence": true,
}

// setsAContextField reports whether this literal fills in a claim.
func setsAContextField(lit *ast.CompositeLit) bool {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if key, ok := kv.Key.(*ast.Ident); ok && contextFields[key.Name] {
			return true
		}
	}
	return false
}

// bareTypeName drops a package qualifier, because a send input is written
// activities.SendEmailInput from compose and SendEmailInput inside the module,
// and both are the same door.
func bareTypeName(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i+1:]
	}
	return name
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
