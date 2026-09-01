// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// The yaml tag is the contract, not the field name: the loader runs with
// KnownFields(true), so a misspelled key is a refusal to boot rather than a
// setting that quietly does nothing.
func TestTracePayloadsIsReadFromTheFile(t *testing.T) {
	cfg, err := Load(writeTemp(t, "version: 1\ncapture:\n  trace_payloads: true\n"), runtimeenv.Production)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Capture.TracePayloadsSetting == nil || !*cfg.Capture.TracePayloadsSetting {
		t.Error("capture.trace_payloads: true did not reach Capture.TracePayloadsSetting")
	}
	if !cfg.Capture.TracesPayloads() {
		t.Error("TracesPayloads() = false for an explicit true")
	}
}

func TestCaptureWarnsOnlyAboutSettingsItNoLongerActsOn(t *testing.T) {
	// Silence for a file that says nothing stale — an operator who never set
	// these must not be told to remove them.
	if w := (Capture{}).Warnings(); len(w) != 0 {
		t.Errorf("Warnings() = %v on an empty block, want none", w)
	}
	if w := (Capture{TransactionalExtra: []string{"esp.example"}}).Warnings(); len(w) != 0 {
		t.Errorf("Warnings() = %v for a setting that still works, want none", w)
	}

	// The moved keys still PARSE — deleting them would turn an upgrade into a
	// refusal to boot, for a list the operator could then no longer reach —
	// so the warning is the only thing that says they no longer act.
	for _, c := range []Capture{
		{FreemailExtra: []string{"provider.example"}},
		{FreemailNever: []string{"customer.example"}},
		{FreemailExtra: []string{"a.example"}, FreemailNever: []string{"b.example"}},
	} {
		w := c.Warnings()
		if len(w) != 1 {
			t.Fatalf("Warnings() = %v for %+v, want exactly one", w, c)
		}
		// It has to name where the list moved, or it tells an operator their
		// config is ignored without telling them what to do instead.
		if !strings.Contains(w[0], "consumer-mail-domains") || !strings.Contains(w[0], "margince.yaml") {
			t.Errorf("warning %q names neither the new surface nor the file to edit", w[0])
		}
	}
}

// The trace names senders unless an operator wrote otherwise. A default of off
// makes the page a list of decisions naming nobody, which cannot answer the one
// question it exists for: why a message did not arrive.
func TestTracePayloadsIsOnUnlessTheFileTurnsItOff(t *testing.T) {
	if !(Capture{}).TracesPayloads() {
		t.Error("TracesPayloads() on the zero block = false, want true")
	}
	if w := (Capture{TracePayloadsSetting: ptr(true)}).Warnings(); len(w) != 0 {
		t.Errorf("Warnings() = %v for a setting that acts, want none", w)
	}
}

// The reason the field is a pointer, stated as a test: for a plain bool an
// absent key and an explicit `false` are the same zero value, so a default of
// on could not be turned off at all — the operator would write the key, boot,
// and find the posture unchanged with nothing to tell them why.
func TestAnExplicitFalseTurnsThePayloadsOff(t *testing.T) {
	cfg, err := Load(writeTemp(t, "version: 1\ncapture:\n  trace_payloads: false\n"), runtimeenv.Production)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Capture.TracePayloadsSetting == nil {
		t.Fatal("capture.trace_payloads: false did not reach the field at all")
	}
	if cfg.Capture.TracesPayloads() {
		t.Error("TracesPayloads() = true after an explicit false")
	}
}

// A file that never mentions the key is the operator saying nothing, which is
// not the same as saying no.
func TestASilentFileGetsTheDefault(t *testing.T) {
	cfg, err := Load(writeTemp(t, "version: 1\n"), runtimeenv.Production)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Capture.TracePayloadsSetting != nil {
		t.Errorf("TracePayloadsSetting = %v for a file that says nothing, want nil", *cfg.Capture.TracePayloadsSetting)
	}
	if !cfg.Capture.TracesPayloads() {
		t.Error("TracesPayloads() = false for a file that says nothing, want true")
	}
}

func ptr(b bool) *bool { return &b }

// The claim above TracesPayloads — that the default lives in one place — is
// only true while nothing else READS the pointer field.
//
// A second reader answers FALSE for the silent file the default exists for,
// which is the one case a reader is least likely to check by hand: it compiles,
// and it works on every installation that wrote the key.
//
// The gate looks for a REFERENCE to the field, not for a dereference of it.
// That distinction is the whole strength of this arm: `*c.TracePayloads` is
// only the most obvious spelling, and `p := c.TracePayloads; return *p` reaches
// the same wrong answer through a local a pattern match never sees. Nothing
// outside the resolver has any business naming the field at all, so naming it
// is what the gate refuses, which leaves TracesPayloads as the way a reader
// reaches the value.
//
// AST rather than text for the same reason: a match on source characters is
// defeated by a line break, a rename, or a parenthesis, and this is a privacy
// posture rather than a style rule.
//
// The field carries a name of its own — TracePayloadsSetting — so this walk
// needs no guess about a receiver's type. compose.CaptureConfig.TracePayloads
// is the resolved plain bool every consumer reads and is a DIFFERENT name, so
// a syntax-only scan can tell the two apart exactly, which is the reason the
// rename came with this gate.
func TestOnlyTheResolverReadsTheTracePayloadsField(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolving the backend root: %v", err)
	}
	// The resolver's own file, and this one. Both NAME the field because one
	// applies the default and the other tests that it does; every other file
	// in the tree has the resolver to call instead.
	//
	// By path, not by basename: `capture.go` and `capture_test.go` exist in
	// several packages here, and excluding them by name would let a real
	// offender in any of them pass unread.
	allowed := map[string]bool{
		filepath.Join(root, "internal", "platform", "deployconfig", "capture.go"):      true,
		filepath.Join(root, "internal", "platform", "deployconfig", "capture_test.go"): true,
	}

	var offenders []string
	walked := 0
	fset := token.NewFileSet()
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || allowed[path] {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			// A file this walk cannot parse is one it cannot clear either.
			return fmt.Errorf("parsing %s: %w", path, parseErr)
		}
		walked++
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || sel.Sel == nil || sel.Sel.Name != "TracePayloadsSetting" {
				return true
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			offenders = append(offenders, fmt.Sprintf("%s:%d", rel, fset.Position(sel.Pos()).Line))
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking the backend tree: %v", err)
	}
	// A walk that read nothing passes exactly like a clean tree, which is the
	// one way a census must never fail.
	if walked < 100 {
		t.Fatalf("the walk parsed %d Go files, which is too few to have covered the tree", walked)
	}
	for _, o := range offenders {
		t.Errorf("%s names deployconfig.Capture.TracePayloadsSetting. Call TracesPayloads() instead: "+
			"the field is a pointer whose nil means the operator said nothing, and every reader "+
			"that resolves it for itself is a second place the default lives — one of which will "+
			"answer false for a file that never set the key", o)
	}
}
