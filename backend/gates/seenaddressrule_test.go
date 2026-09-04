// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
//
//gate:kind prohibition H2

package gates

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestTheSeenSenderRuleIsSpelledOnce holds the claim addressWasSeen makes about
// itself.
//
// Two questions in capture are answered by "was this sender part of what the
// verdict read": whether an ARRIVING message may inherit an opening verdict,
// and whether an already-imported SIBLING may be stamped with one. Both admit
// mail to a wider audience, so a second copy that drifted — a fold missed, a
// trim dropped, a domain compared where an address was meant — publishes
// correspondence the classifier never judged.
//
// The rule is a lowercased, trimmed, exact comparison of one address against
// the recorded set. This gate refuses a second place that performs it, by
// looking for the comparison rather than for the helper's name: a copy would
// have a different name, which is exactly why a name-based check would miss it.
func TestTheSeenSenderRuleIsSpelledOnce(t *testing.T) {
	t.Parallel()
	const pkg = "internal/modules/capture"
	entries, err := os.ReadDir(pkg)
	if err != nil {
		t.Fatalf("reading the capture package: %v", err)
	}
	// The rule's SHAPE, not its vocabulary: a function that takes a []string of
	// addresses and decides whether one is among them. Matching on the word
	// `seen` would be defeated by renaming the parameter to `observed`, which
	// is the first thing a second copy would do.
	//
	// Two signatures cover it — the slice second (addressWasSeen's own shape)
	// or first — and the body is then any of the ways Go compares strings:
	// ==, EqualFold, a map lookup, or slices.Contains.
	signature := regexp.MustCompile(
		`func\s+\w+\(\s*\w+\s+string,\s*\w+\s+\[\]string\s*\)\s*bool` +
			`|func\s+\w+\(\s*\w+\s+\[\]string,\s*\w+\s+string\s*\)\s*bool`)
	compares := regexp.MustCompile(
		`strings\.EqualFold|slices\.Contains|map\[string\]|==\s*\w+\b`)

	var found []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(pkg, name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		// Every match in the file, not merely whether the file matches: two
		// copies inside one file would otherwise register as one.
		for _, decl := range signature.FindAll(body, -1) {
			at := bytes.Index(body, decl)
			window := body[at:min(at+600, len(body))]
			if compares.Match(window) {
				found = append(found, name)
			}
		}
	}
	if len(found) == 0 {
		// Under-recognition is the one way this gate must not fail: a scan that
		// matches nothing reports PASS and there is no assertion to notice.
		t.Fatal("the seen-sender comparison was not found anywhere in capture — this gate has " +
			"stopped reading its own subject, and would pass over a second copy")
	}
	if len(found) > 1 {
		t.Fatalf("the seen-sender rule is spelled in %v; it belongs in addressWasSeen alone, "+
			"because a copy that drifts publishes mail the classifier never judged", found)
	}
	if found[0] != "verdictinherit.go" {
		t.Fatalf("the seen-sender rule moved to %s; repoint this gate and the claim in its doc "+
			"comment, rather than leaving a second one beside it", found[0])
	}
}
