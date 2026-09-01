// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig

import (
	"strings"
	"testing"

	"io/fs"
	"os"
	"path/filepath"
	"regexp"

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
	if cfg.Capture.TracePayloads == nil || !*cfg.Capture.TracePayloads {
		t.Error("capture.trace_payloads: true did not reach Capture.TracePayloads")
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
	if w := (Capture{TracePayloads: ptr(true)}).Warnings(); len(w) != 0 {
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
	if cfg.Capture.TracePayloads == nil {
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
	if cfg.Capture.TracePayloads != nil {
		t.Errorf("TracePayloads = %v for a file that says nothing, want nil", *cfg.Capture.TracePayloads)
	}
	if !cfg.Capture.TracesPayloads() {
		t.Error("TracesPayloads() = false for a file that says nothing, want true")
	}
}

func ptr(b bool) *bool { return &b }

// The claim above TracesPayloads — that the default lives in one place — is
// only true while nothing else dereferences the pointer. A second reader that
// wrote `*c.TracePayloads` would compile, work on every file that sets the key,
// and answer FALSE for the silent file the default exists for: the one case a
// reader is least likely to test by hand.
//
// So the gate is a scan, not an assertion about behaviour. It reads the tree
// rather than this package, because the wrong reader is by definition somewhere
// else.
func TestOnlyTheResolverDereferencesTracePayloads(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolving the backend root: %v", err)
	}
	// The one file allowed to read the pointer at all: the resolver's own.
	resolver := filepath.Join(root, "internal", "platform", "deployconfig", "capture.go")
	deref := regexp.MustCompile(`\*[a-zA-Z_][a-zA-Z0-9_.]*\.TracePayloads\b`)

	var offenders []string
	walked := 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		// This file spells the pattern it hunts for, and the resolver is the
		// answer rather than a violation of it.
		if path == resolver || strings.HasSuffix(path, "capture_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		walked++
		if loc := deref.FindString(string(src)); loc != "" {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			offenders = append(offenders, rel+": "+loc)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the backend tree: %v", err)
	}
	// A walk that read nothing passes exactly like a clean tree.
	if walked < 100 {
		t.Fatalf("the walk read %d Go files, which is too few to have covered the tree", walked)
	}
	for _, o := range offenders {
		t.Errorf("%s dereferences TracePayloads directly. Call Capture.TracesPayloads() "+
			"instead: a bare dereference answers false for a file that never set the key, "+
			"which is the default this pointer exists to express", o)
	}
}
