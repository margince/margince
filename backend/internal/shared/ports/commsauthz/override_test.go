// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAllowedByOverrideFlipsToAllowAndNamesTheRow(t *testing.T) {
	id := ids.NewV7()
	d := Decision{Verdict: VerdictDeny, ReasonCode: ReasonNoEvidence}
	got := d.AllowedByOverride(id)
	if got.Verdict != VerdictAllow {
		t.Errorf("verdict = %q, want allow", got.Verdict)
	}
	if got.ReasonCode != ReasonAllowedByOverride {
		t.Errorf("reason = %q, want %q", got.ReasonCode, ReasonAllowedByOverride)
	}
	if got.OverrideID != id {
		t.Error("override id was not recorded on the decision")
	}
}

func TestAnOverrideAllowedDecisionIsNotItselfOverrulable(t *testing.T) {
	d := Decision{Verdict: VerdictAllow, ReasonCode: ReasonAllowedByOverride}
	if d.CanBeOverruled() {
		t.Error("an allow must never be overrulable")
	}
}

// TestUnknownPurposeIsOverrulableButNotByCategory pins the one refusal the two
// predicates must disagree on. An unknown_purpose deny is machine-level and
// non-absolute, so CanBeOverruled is true — but it resolved to NO category, so a
// per-category override has nothing to answer and CanBeOverruledByCategory is
// false. Left equal, a rep's marketing vouch would flip a send whose purpose the
// engine could not resolve.
func TestUnknownPurposeIsOverrulableButNotByCategory(t *testing.T) {
	unknown := Decision{Verdict: VerdictDeny, ReasonCode: ReasonUnknownPurpose}
	if !unknown.CanBeOverruled() {
		t.Error("unknown_purpose is machine-level and non-absolute; CanBeOverruled must stay true, " +
			"or this test's premise moved")
	}
	if unknown.CanBeOverruledByCategory() {
		t.Error("unknown_purpose resolved to no category; a per-category override must not answer it")
	}

	// A genuinely category-resolved refusal is answerable by both: it named the
	// one category a vouch can address.
	for _, reason := range []string{ReasonNoMarketingConsent, ReasonNoEvidence} {
		d := Decision{Verdict: VerdictDeny, ReasonCode: reason}
		if !d.CanBeOverruled() || !d.CanBeOverruledByCategory() {
			t.Errorf("%q resolves to a category; both predicates must be true", reason)
		}
	}

	// An allow has nothing to overrule, by either predicate.
	allowed := Decision{Verdict: VerdictAllow, ReasonCode: ReasonAllowed}
	if allowed.CanBeOverruled() || allowed.CanBeOverruledByCategory() {
		t.Error("an allow reports as overrulable")
	}
}

// TestTheCategoryExclusionIsExactlyUnknownPurposeOverEveryReason pins the
// exclusion set: over every reason this package declares, read off its own
// files rather than retyped, the two predicates part on unknown_purpose and on
// nothing else. Adding a second exclusion, or moving this one, fails here. It
// cannot tell whether a NEW reason ought to be excluded; the behavioural proof
// that an unresolved category is never answered by a vouch is
// TestAnOverrideCannotFlipAnUnknownPurpose (modules/consent).
func TestTheCategoryExclusionIsExactlyUnknownPurposeOverEveryReason(t *testing.T) {
	t.Parallel()

	for _, reason := range reasonConstants(t) {
		d := Decision{Verdict: VerdictDeny, ReasonCode: reason}
		agree := d.CanBeOverruled() == d.CanBeOverruledByCategory()
		if reason == ReasonUnknownPurpose {
			if agree {
				t.Error("unknown_purpose is the one reason the predicates must part on")
			}
			continue
		}
		if !agree {
			t.Errorf("%q: the predicates disagree, but only unknown_purpose resolves to no category — "+
				"if this reason now resolves to none, add it to CanBeOverruledByCategory's exclusion "+
				"and to this test's one exception together", reason)
		}
	}
}

// reasonConstants reads every Reason* constant off the package's own non-test
// files, so a reason exists in this census the moment it compiles, whichever
// file declares it.
func reasonConstants(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("listing the package's files: %v", err)
	}
	fset := token.NewFileSet()
	var reasons []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		reasons = append(reasons, fileReasonConstants(t, file)...)
	}
	// Under-recognition is the one way a census fails silently: a parse that
	// found a handful of reasons is reading the wrong declarations.
	if len(reasons) < 12 {
		t.Fatalf("read %d Reason* constants off the package, fewer than it declares", len(reasons))
	}
	return reasons
}

// fileReasonConstants returns the string literal of each Reason* constant one
// file declares.
func fileReasonConstants(t *testing.T, file *ast.File) []string {
	t.Helper()
	var reasons []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			values, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range values.Names {
				if !strings.HasPrefix(name.Name, "Reason") {
					continue
				}
				if i >= len(values.Values) {
					t.Fatalf("%s has no explicit value; this census reads reason codes as literals", name.Name)
				}
				lit, ok := values.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					t.Fatalf("%s is not a string literal; this census reads reason codes as literals", name.Name)
				}
				code, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("unquoting %s: %v", name.Name, err)
				}
				reasons = append(reasons, code)
			}
		}
	}
	return reasons
}
