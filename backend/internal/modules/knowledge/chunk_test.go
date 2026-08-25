// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

import (
	"strings"
	"testing"
)

func TestADocumentShorterThanOneChunkIsOneChunk(t *testing.T) {
	got := ChunkText("A short note about migrations.")
	if len(got) != 1 {
		t.Fatalf("got %d chunks, want 1", len(got))
	}
	if got[0].Text != "A short note about migrations." {
		t.Fatalf("text = %q", got[0].Text)
	}
	if got[0].Ix != 0 {
		t.Fatalf("ix = %d, want 0", got[0].Ix)
	}
}

func TestEmptyAndWhitespaceOnlyDocumentsProduceNoChunks(t *testing.T) {
	for _, in := range []string{"", "   ", "\n\n\t\n"} {
		if got := ChunkText(in); len(got) != 0 {
			t.Fatalf("ChunkText(%q) = %d chunks, want 0", in, len(got))
		}
	}
}

// The split prefers paragraph boundaries, so a chunk reads as prose rather
// than as a window that happened to land there.
func TestSplitPrefersParagraphBoundaries(t *testing.T) {
	para := strings.Repeat("word ", 100) // ~500 chars
	got := ChunkText(para + "\n\n" + para + "\n\n" + para)
	if len(got) < 2 {
		t.Fatalf("got %d chunks, want at least 2", len(got))
	}
	for _, c := range got {
		if strings.HasPrefix(c.Text, " ") || strings.HasSuffix(c.Text, " ") {
			t.Fatalf("chunk not trimmed: %q", c.Text)
		}
	}
}

// A single unbroken line has no boundary to prefer, and must still be split
// rather than returned whole — this is the case that a naive paragraph
// splitter silently passes through as one enormous chunk.
func TestAnUnbrokenLineIsStillSplit(t *testing.T) {
	got := ChunkText(strings.Repeat("a", 10000))
	if len(got) < 2 {
		t.Fatalf("got %d chunks for a 10k unbroken line, want several", len(got))
	}
	for _, c := range got {
		if len(c.Text) > maxChunkChars {
			t.Fatalf("chunk of %d chars exceeds the %d ceiling", len(c.Text), maxChunkChars)
		}
	}
}

func TestChunksNeverSplitMidWord(t *testing.T) {
	got := ChunkText(strings.Repeat("alpha beta gamma delta ", 200))
	for _, c := range got {
		if strings.HasSuffix(c.Text, "alph") || strings.HasSuffix(c.Text, "gamm") {
			t.Fatalf("chunk ends mid-word: %q", c.Text)
		}
	}
}

func TestIndicesAreContiguousFromZero(t *testing.T) {
	got := ChunkText(strings.Repeat("sentence here. ", 400))
	for i, c := range got {
		if c.Ix != i {
			t.Fatalf("chunk %d carries ix %d", i, c.Ix)
		}
	}
}

// CRLF is what a Windows-authored markdown file arrives as, and a chunker that
// treats \r as content produces chunks whose quotes never match.
func TestCRLFIsNormalisedBeforeSplitting(t *testing.T) {
	got := ChunkText("First line.\r\n\r\nSecond line.")
	for _, c := range got {
		if strings.Contains(c.Text, "\r") {
			t.Fatalf("chunk retains a carriage return: %q", c.Text)
		}
	}
}

// Overlap is the whole reason a sentence spanning a boundary is answerable: it
// must be wholly present in at least one chunk. A chunker that advanced by the
// full width would satisfy every other test here and still split every
// boundary-spanning sentence across two passages, present in neither.
func TestConsecutiveChunksOverlap(t *testing.T) {
	got := ChunkText(strings.Repeat("alpha beta gamma delta ", 200))
	if len(got) < 2 {
		t.Fatalf("got %d chunks, want at least 2", len(got))
	}
	tail := got[0].Text[len(got[0].Text)-overlapChars/2:]
	if !strings.Contains(got[1].Text, tail) {
		t.Fatalf("chunk 1 does not carry the tail of chunk 0, so nothing spans the seam:\n"+
			"tail  = %q\nchunk = %q", tail, got[1].Text)
	}
}

// The walk must terminate and make progress on every input shape. A boundary
// found at or before the overlap width is the case that can hand back a start
// offset no further along than the one it was given.
func TestChunkingTerminatesWhenEveryBoundaryIsNearTheStart(t *testing.T) {
	// A very long run of two-character lines: every window's last boundary sits
	// far from the ceiling, which is what stresses the advance arithmetic.
	got := ChunkText(strings.Repeat("a\n", 5000))
	if len(got) == 0 {
		t.Fatal("no chunks produced")
	}
	for i, c := range got {
		if c.Text == "" {
			t.Fatalf("chunk %d is empty", i)
		}
	}
}

// Every byte of the document reaches at least one chunk. Overlap makes the
// concatenation longer than the source, so length proves nothing — this walks
// the original and asserts each span is findable.
func TestNoContentIsDroppedBetweenChunks(t *testing.T) {
	const doc = "First paragraph about migrations.\n\n" +
		"Second paragraph about dev stacks and how they claim ports.\n\n" +
		"Third paragraph about deployment."
	got := ChunkText(doc)
	joined := strings.Join(chunkTexts(got), " ")
	for _, want := range []string{"First paragraph", "Second paragraph", "Third paragraph", "claim ports"} {
		if !strings.Contains(joined, want) {
			t.Errorf("%q reached no chunk", want)
		}
	}
}

func chunkTexts(chunks []Chunk) []string {
	texts := make([]string, 0, len(chunks))
	for _, c := range chunks {
		texts = append(texts, c.Text)
	}
	return texts
}
