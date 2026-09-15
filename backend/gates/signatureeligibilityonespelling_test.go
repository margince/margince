// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind claim H2

package gates

// One spelling of "may this message be mined for this contact's signature",
// held by a test rather than by a comment.
//
// TWO statements must agree: the candidate query that SELECTS the work
// (enrichcandidates.go) and the apply that LANDS the fields
// (enrichsignature.go). They ran on different rules once — selection widened to
// an owner's own participants mail while the apply still demanded
// audience='workspace' — and the pass marked every such message read without
// writing anything. The mail was consumed, nothing was enriched, and no later
// run reconsidered it, because the watermark had advanced.
//
// That failure is invisible from either file alone. Each read correctly; only
// the pair was wrong, which is why the obligation is a gate rather than a
// review note.
//
// The subject is DERIVED from the owner: every SQL string in the contacts
// module that tests an activity's `audience` against 'workspace' must reach it
// through SignatureSourceEligible, unless it is the fragment itself. A
// hand-listed pair of files would go stale the moment a third reader appears.
//
// WHAT THIS DOES NOT CATCH, deliberately: a rule rebuilt from the same columns
// under different names, or one composed in Go rather than SQL. The defect this
// exists over was a literal audience comparison, as is every plausible next one.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// audienceWorkspaceTest matches a SQL comparison of an audience column against
// the workspace literal, in either order and with or without a table alias.
var audienceWorkspaceTest = regexp.MustCompile(
	`(?i)(\w+\.)?audience\s*(=|<>|!=)\s*'workspace'|'workspace'\s*(=|<>|!=)\s*(\w+\.)?audience`)

// signatureEligibilityOwner is the file allowed to spell the rule out — and
// only inside the fragment's own declaration. Every other reader composes it.
const signatureEligibilityOwner = "enrichcandidates.go"

// withoutGoLineComments drops Go `//` lines before the scan. Prose DESCRIBING
// the rule — this change's own account of the defect it exists over — is not a
// second spelling of it, and a census that cannot tell a comment from a
// statement reports the documentation as the violation.
//
// Deliberately NOT gatekit's withoutComments, which is a different question:
// that one strips SQL spans inside a query literal ('…', --, /* */, $tag$) and
// leaves Go comments standing, which is exactly the text this gate must ignore.
// Neither is a copy of the other; delete one and the other stops working.
func withoutGoLineComments(text string) string {
	lines := strings.Split(text, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// withoutFragmentDeclaration returns the owner file with SignatureSourceEligible's
// body removed, so what remains is every OTHER statement in it. The function ends
// at the first line that closes a declaration at column zero.
func withoutFragmentDeclaration(text string) string {
	const decl = "func SignatureSourceEligible("
	start := strings.Index(text, decl)
	if start < 0 {
		return text
	}
	rest := text[start:]
	if end := strings.Index(rest, "\n}\n"); end >= 0 {
		return text[:start] + rest[end+len("\n}\n"):]
	}
	return text[:start]
}

func TestSignatureEligibilityHasOneSpelling(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("internal", "modules", "contacts")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the contacts module: %v", err)
	}
	var scanned int
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		// The owner file is not skipped wholesale: a second, diverging spelling
		// could then hide inside the very file that declares the fragment. Only
		// the fragment's OWN declaration is exempt, so the rest of that file is
		// held to the same rule as every other reader.
		if name == signatureEligibilityOwner {
			body, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("reading %s: %v", name, err)
			}
			scanned++
			if outside := withoutGoLineComments(withoutFragmentDeclaration(string(body))); audienceWorkspaceTest.MatchString(outside) {
				t.Errorf("%s spells the signature source rule a SECOND time, outside "+
					"SignatureSourceEligible's own declaration.\n"+
					"  The fragment exists so selection and application cannot drift; a rival\n"+
					"  spelling beside it defeats that whether or not it sits in this file.",
					name)
			}
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		scanned++
		text := withoutGoLineComments(string(body))
		if !strings.Contains(text, "signature") && !strings.Contains(text, "Signature") {
			continue
		}
		for _, hit := range audienceWorkspaceTest.FindAllString(text, -1) {
			// Deliberately NOT "the file mentions the helper somewhere". That
			// exemption is file-level, so a reader could compose the fragment
			// once and still spell the rule a second time further down — which
			// is the exact defect this gate exists to catch, passing.
			t.Errorf("%s spells the signature source rule itself (%q) instead of asking "+
				"SignatureSourceEligible.\n"+
				"  Selection and application must ask ONE question: when they disagreed, the pass\n"+
				"  marked an owner's own mail read and wrote nothing, consuming it for good.\n"+
				"  Compose the fragment, or state here why this reader is not the signature rule.",
				name, hit)
		}
	}
	// A census that reads nothing reports PASS. The module has well over a
	// hundred files; a count this low means the path moved.
	if scanned < 50 {
		t.Fatalf("scanned only %d files in %s — the census is looking in the wrong place", scanned, dir)
	}
}
