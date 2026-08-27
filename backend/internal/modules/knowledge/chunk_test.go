// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

import (
	"math/rand/v2"
	"strings"
	"testing"
	"unicode/utf8"
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

// EVERY byte offset this chunker turns into a span boundary must land on a
// rune start, and there are TWO of them: the no-boundary fallback cut, and the
// overlap step back. The second is the one that bites ordinary European prose.
//
// The fixture is deliberately IRREGULAR — mixed 1- and 2-byte runes at no fixed
// period — because a uniform one passes by arithmetic luck. An earlier version
// of this test used a run of 3-byte CJK runes, and 120 (the overlap width) is
// divisible by 3, so the step back happened to land rune-aligned every time
// while the defect it was written for sat ten lines below.
func TestNoSpanBoundaryEverSplitsARune(t *testing.T) {
	// German words with umlauts, in a DETERMINISTIC but irregular order: a
	// fixed seed keeps the case reproducible while breaking the periodicity a
	// cyclic fixture has. Cycling seven words repeats every seven, which lines
	// the spans up on the same residues every time and hides the defect — that
	// is how the first version of this test passed against the broken code.
	words := []string{"Nachrichten", "Grundsätze", "Verhältnis", "übermitteln", "Prüfung", "Größe", "Maßnahme", "und", "die", "Änderung"}
	rng := rand.New(rand.NewPCG(1, 2))

	// Many documents rather than one: the offset that splits a rune depends on
	// how the bytes fall, so one document that happens to fall well proves
	// nothing about the next.
	for doc := 0; doc < 50; doc++ {
		var b strings.Builder
		for b.Len() < 6000 {
			b.WriteString(words[rng.IntN(len(words))])
			b.WriteByte(' ')
		}
		text := b.String()

		chunks := ChunkText(text)
		if len(chunks) < 2 {
			t.Fatalf("document %d produced %d chunks; it must be cut", doc, len(chunks))
		}
		for i, c := range chunks {
			if !utf8.ValidString(c.Text) {
				t.Fatalf("document %d chunk %d is not valid UTF-8 — a span boundary split a rune", doc, i)
			}
			if !strings.Contains(text, c.Text) {
				t.Fatalf("document %d chunk %d is not a span of the document", doc, i)
			}
		}
	}
}

// A document with no ASCII whitespace in reach reaches the fallback cut, and
// the cut must land on a rune start.
//
// Every boundary bestBoundary looks for is ASCII — two newlines, a full stop
// and a space, a newline, a space — so a Japanese, Chinese or Thai document
// reaches the fallback for EVERY span. A cut in the middle of a 3-byte rune
// produces bytes that are not UTF-8, the insert fails with 22021 from Postgres,
// the ingest burns all three attempts, and the uploader is told the file is
// fine and to try again.
func TestAnUnbrokenCJKRunIsNeverCutMidRune(t *testing.T) {
	// Well past maxChunkChars in bytes, and with no ASCII space anywhere: the
	// fallback is the only branch that can cut it.
	text := strings.Repeat("日本語のテキストです", 400)

	chunks := ChunkText(text)
	if len(chunks) < 2 {
		t.Fatalf("the fixture produced %d chunks; it must be cut for this to test anything", len(chunks))
	}
	for i, c := range chunks {
		if !utf8.ValidString(c.Text) {
			t.Fatalf("chunk %d is not valid UTF-8 — the cut split a rune", i)
		}
	}
	// And nothing was lost or duplicated beyond the declared overlap: every
	// chunk is a real substring of the document.
	for i, c := range chunks {
		if !strings.Contains(text, c.Text) {
			t.Fatalf("chunk %d is not a span of the document", i)
		}
	}
}

// The same document must also survive a round trip through the hash, which is
// what decides whether a re-ingest costs a model call.
func TestACJKChunkHashesConsistently(t *testing.T) {
	chunks := ChunkText(strings.Repeat("日本語のテキストです", 400))
	if len(chunks) == 0 {
		t.Fatal("no chunks")
	}
	again := ChunkText(strings.Repeat("日本語のテキストです", 400))
	for i := range chunks {
		if chunks[i].Hash() != again[i].Hash() {
			t.Fatalf("chunk %d hashed differently across two runs of a pure function", i)
		}
	}
}

// Every chunk knows the line it starts on, and it is the line a reader opening
// the file would land on — the first line of the span's own text, not the blank
// line a paragraph break left in front of it.
func TestEachChunkKnowsTheLineItStartsOn(t *testing.T) {
	// Line numbers written into the text, so the assertion reads as the
	// document does rather than as arithmetic.
	text := "line one\nline two\n\nline four begins a paragraph\n" +
		strings.Repeat("filler that pushes past the chunk ceiling. ", 30) +
		"\n\nlast paragraph starts here"

	chunks := ChunkText(text)
	if len(chunks) < 2 {
		t.Fatalf("the fixture produced %d chunks; it must be cut for this to test anything", len(chunks))
	}
	if chunks[0].StartLine != 1 {
		t.Fatalf("the first chunk starts on line %d, want 1", chunks[0].StartLine)
	}
	lines := strings.Split(text, "\n")
	for i, c := range chunks {
		if c.StartLine < 1 || c.StartLine > len(lines) {
			t.Fatalf("chunk %d claims line %d, and the document has %d", i, c.StartLine, len(lines))
		}
		// The claimed line CONTAINS the start of this chunk's text.
		//
		// Contains rather than begins-with, and the difference is the reason
		// `column` exists: a span cut at the width ceiling starts in the MIDDLE
		// of a long line, so the line it points at legitimately begins earlier
		// than the span does.
		first := strings.SplitN(c.Text, "\n", 2)[0]
		probe := first[:min(len(first), 20)]
		if !strings.Contains(lines[c.StartLine-1], probe) {
			t.Fatalf("chunk %d claims line %d (%q), which does not contain %q",
				i, c.StartLine, lines[c.StartLine-1][:min(len(lines[c.StartLine-1]), 40)], probe)
		}
	}
}

// A chunk that begins after a paragraph break points at its own first line, not
// at the blank line before it.
func TestAChunkAfterABlankLinePointsAtItsOwnText(t *testing.T) {
	head := strings.Repeat("The first paragraph runs on for a while. ", 25)
	text := head + "\n\nThe second paragraph starts here."

	chunks := ChunkText(text)
	if len(chunks) < 2 {
		t.Fatalf("the fixture produced %d chunks", len(chunks))
	}
	last := chunks[len(chunks)-1]
	lines := strings.Split(text, "\n")
	if strings.TrimSpace(lines[last.StartLine-1]) == "" {
		t.Fatalf("the last chunk points at line %d, which is blank", last.StartLine)
	}
}
