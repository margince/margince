// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package vectorkit_test

import (
	"testing"

	"github.com/gradionhq/margince/backend/internal/platform/vectorkit"
)

// A zero vector must never reach storage or a query: cosine against it is
// 0/0 = NaN, and `ORDER BY sim DESC` sorts NaN FIRST — silently outranking
// every real match.
func TestIsZeroRecognisesTheAllZeroVector(t *testing.T) {
	if !vectorkit.IsZero([]float32{0, 0, 0}) {
		t.Fatal("all-zero vector not recognised as zero")
	}
	if !vectorkit.IsZero(nil) {
		t.Fatal("nil vector not recognised as zero")
	}
	if vectorkit.IsZero([]float32{0, 0, 0.0001}) {
		t.Fatal("a vector with one non-zero component is not zero")
	}
}

func TestLiteralRendersThePgvectorForm(t *testing.T) {
	got := vectorkit.Literal([]float32{1, -0.5, 0})
	want := "[1,-0.5,0]"
	if got != want {
		t.Fatalf("literal = %q, want %q", got, want)
	}
}

// An empty vector still renders as a well-formed literal rather than as the
// empty string: pgvector answers an empty input with a parse error naming the
// column, which reads as a schema fault rather than as the caller having handed
// over nothing.
func TestLiteralRendersAnEmptyVectorAsEmptyBrackets(t *testing.T) {
	if got := vectorkit.Literal(nil); got != "[]" {
		t.Fatalf("literal = %q, want %q", got, "[]")
	}
}

// Unchanged is the skip-compare that makes a re-ingest of identical text free.
// It must answer NO when the identity moved, even though the text did not: a
// row stamped with a model that no longer serves the workspace is
// indistinguishable from a live one, and would rank against queries it cannot
// share a space with.
func TestUnchangedRequiresBothHashAndIdentityToMatch(t *testing.T) {
	const h, id = "abc", "openai/text-embedding-3-small@1536"
	if !vectorkit.Unchanged(h, id, h, id) {
		t.Fatal("same hash and same identity must be unchanged")
	}
	if vectorkit.Unchanged(h, id, "def", id) {
		t.Fatal("a changed hash must not be unchanged")
	}
	if vectorkit.Unchanged(h, id, h, "ollama/nomic-embed-text@768") {
		t.Fatal("a changed identity must not be unchanged")
	}
	if vectorkit.Unchanged("", "", h, id) {
		t.Fatal("an unembedded row must never be unchanged")
	}
}

// A row whose stored identity is empty is unembedded, and no combination of the
// other three arguments makes it current — including the degenerate case where
// the live identity is ALSO empty, which is what an unbound embed lane reports.
// Answering "unchanged" there would let a corpus with no vectors at all read as
// fully embedded.
func TestUnchangedIsFalseWhenNoEmbedLaneIsBound(t *testing.T) {
	if vectorkit.Unchanged("", "", "", "") {
		t.Fatal("an unembedded row under an unbound lane must not read as unchanged")
	}
}
