// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A body the HTTP chassis accepts and the search index cannot hold.
//
// `activity.search_tsv` is GENERATED from the subject and body, and
// to_tsvector's output has a hard 1,048,575-byte ceiling. Output runs about
// 1.10x input for word-dense text, so a body around 950 KB overflows it — well
// inside the chassis's 1 MiB request cap. The write raised 54000, which the
// classification net did not know, so a legal-sized request answered an opaque
// 500 whose advice was to retry a call that fails the same way forever.
//
// Asked of the REAL database rather than a fabricated PgError, because the
// mapping is only worth having if Postgres actually raises that SQLSTATE here:
// a unit test over a hand-built error proves the net and nothing about the
// column.

import (
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestABodyTooLargeToIndexIsRefusedRatherThanAnsweredAsAServerFault(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)

	subject, body := "A very long note", wordDense(1_000_000)
	_, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
		Kind: "note", Subject: &subject, Body: &body,
	})
	if err == nil {
		t.Fatal("a body past the index ceiling was accepted; either the column changed or this " +
			"fixture no longer reaches the limit it is about")
	}

	fault, ok := httperr.Classify(err)
	if !ok {
		t.Fatalf("the refusal reached the unhandled path, which answers 500 internal: %v", err)
	}
	if fault.Status != 422 {
		t.Errorf("status = %d, want 422 — the request was legal and the value is the caller's to "+
			"shorten, so retrying is advice that can never work", fault.Status)
	}
	if fault.Code != "value_too_large" {
		t.Errorf("code = %q, want %q", fault.Code, "value_too_large")
	}
	// The database's own words carry the byte counts and the internal type
	// name; the caller gets the sentence, the operator gets the cause.
	for _, leak := range []string{"tsvector", "54000", "1048575"} {
		if strings.Contains(fault.Detail, leak) {
			t.Errorf("the refusal leaks %q: %q", leak, fault.Detail)
		}
	}
	if fault.InfraCause == nil {
		t.Error("the cause reaches no log — withholding a message is not the same as losing it")
	}
}

// wordDense builds text whose tsvector output EXCEEDS its input: distinct
// words, each indexed, with no repetition for the dictionary to fold away. A
// megabyte of one repeated word would compress to a single lexeme and prove
// nothing.
func wordDense(bytes int) string {
	var b strings.Builder
	b.Grow(bytes + 16)
	for i := 0; b.Len() < bytes; i++ {
		b.WriteString("w")
		b.WriteString(strconv.Itoa(i))
		b.WriteString(" ")
	}
	return b.String()
}
