// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import "testing"

// The run path refuses a javascript: source; so must the save path. A client is
// not obliged to send back what the run returned, and a read that refuses while
// a write accepts is how untrusted input lands in the record and waits for a
// renderer to turn it into a sink.
func TestOnlyAnOpenableDocumentCountsAsASource(t *testing.T) {
	for _, refused := range []string{
		"javascript:fetch('//evil')",
		"data:text/html,<script>",
		"ftp://example.com/x",
		"http:", // parses cleanly, points nowhere
		"",
	} {
		if webSourceURL(refused) {
			t.Errorf("%q was accepted as a source; a reader cannot open it", refused)
		}
	}
	for _, allowed := range []string{
		"https://example.com/team",
		"http://example.com/bio",
	} {
		if !webSourceURL(allowed) {
			t.Errorf("%q was refused; it is a document a reader can open", allowed)
		}
	}
}

// Two claims for one field are two answers to one question, and applying both
// did not pick either: the acceptance REPLACES, so the second reached the
// evidence row, while the empty-slot fill the first triggered is ADDITIVE and
// kept the first URL. Last wins, and the field keeps the place it first took so
// the audit image reads back in the order the caller sent.
func TestASecondClaimForOneFieldReplacesTheFirst(t *testing.T) {
	got := lastClaimPerField([]ResearchClaimInput{
		{Field: "linkedin", Value: "https://www.linkedin.com/in/first", Quote: "a", SourceURL: "https://a.test"},
		{Field: "title", Value: "Head of Ops", Quote: "b", SourceURL: "https://b.test"},
		{Field: "linkedin", Value: "https://www.linkedin.com/in/second", Quote: "c", SourceURL: "https://c.test"},
	})

	if len(got) != 2 {
		t.Fatalf("kept %d claims, want 2 — one per field", len(got))
	}
	if got[0].Field != "linkedin" || got[1].Field != "title" {
		t.Errorf("fields = %q, %q, want linkedin then title — a field keeps the place it first took",
			got[0].Field, got[1].Field)
	}
	if got[0].Value != "https://www.linkedin.com/in/second" {
		t.Errorf("linkedin value = %q, want the last one the caller sent", got[0].Value)
	}
	// The whole claim is replaced, not merely its value: an evidence row
	// carrying the first claim's quote beside the second's URL would attest to
	// words that do not name the value stored under them.
	if got[0].Quote != "c" || got[0].SourceURL != "https://c.test" {
		t.Errorf("linkedin evidence = %q from %q, want the last claim's own",
			got[0].Quote, got[0].SourceURL)
	}
}

// The ordinary batch is untouched, which is what makes the case above a
// narrowing rather than a rewrite of what the caller sent.
func TestClaimsForDistinctFieldsPassThroughUnchanged(t *testing.T) {
	in := []ResearchClaimInput{
		{Field: "title", Value: "Head of Ops", Quote: "a", SourceURL: "https://a.test"},
		{Field: "linkedin", Value: "https://www.linkedin.com/in/x", Quote: "b", SourceURL: "https://b.test"},
	}

	got := lastClaimPerField(in)

	if len(got) != len(in) {
		t.Fatalf("kept %d claims, want all %d", len(got), len(in))
	}
	for i := range in {
		if got[i] != in[i] {
			t.Errorf("claim %d = %+v, want %+v", i, got[i], in[i])
		}
	}
}
