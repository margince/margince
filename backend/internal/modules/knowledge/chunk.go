// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode/utf8"
)

// maxChunkChars is the ceiling one chunk may reach. It is a character count
// rather than a token count on purpose: tokens are the embedder's unit and vary
// per binding, and a chunker whose output changes when an operator swaps models
// would re-chunk the whole corpus on a configuration edit.
const maxChunkChars = 800

// overlapChars is how much of the previous chunk each chunk repeats. Overlap
// exists so a sentence spanning a boundary is wholly present in at least one
// chunk — without it, the answer to a question is split across two passages and
// present in neither.
const overlapChars = 120

// Chunk is one embeddable span of a document, in document order.
type Chunk struct {
	Text string
	Ix   int
}

// Hash identifies this span's exact text. It is what decides whether a
// re-ingest costs a model call: the chunker is pure, so identical prose yields
// an identical hash, and vectorkit.Unchanged reads it against the stored one.
//
// Spelled here rather than at the writer because the value belongs to the span,
// not to the row — two writers computing it two ways is how a corpus starts
// paying to re-embed text that never moved.
func (c Chunk) Hash() string {
	sum := sha256.Sum256([]byte(c.Text))
	return hex.EncodeToString(sum[:])
}

// ChunkText splits a document into embeddable spans, preferring paragraph then
// sentence boundaries and never breaking a word.
//
// It is pure: the same text always yields the same chunks, which is what lets
// the content hash decide whether a re-ingest costs any model calls.
func ChunkText(text string) []Chunk {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var chunks []Chunk
	for _, span := range splitToWidth(text) {
		span = strings.TrimSpace(span)
		if span == "" {
			continue
		}
		chunks = append(chunks, Chunk{Text: span, Ix: len(chunks)})
	}
	return chunks
}

// splitToWidth walks the text emitting spans no longer than maxChunkChars,
// cutting at the best boundary available within reach and carrying overlapChars
// of the previous span forward.
func splitToWidth(text string) []string {
	var spans []string
	for start := 0; start < len(text); {
		end := start + maxChunkChars
		if end >= len(text) {
			spans = append(spans, text[start:])
			break
		}
		cut := bestBoundary(text[start:end])
		if cut <= 0 {
			// No boundary anywhere in reach — an unbroken run. Cut at the
			// ceiling rather than returning the rest whole, because a single
			// 10k-character line is exactly the document that would otherwise
			// arrive at the embedder as one span.
			//
			// Backed off to a RUNE start, and this is not a nicety. The
			// boundaries above are all ASCII, so a Japanese, Chinese or Thai
			// document reaches this branch for every span — and a cut in the
			// middle of a 3-byte rune produces bytes that are not UTF-8. The
			// insert then fails with 22021 from Postgres, the ingest burns all
			// three attempts, and the uploader is told the file is fine and to
			// try again, which they will, forever.
			cut = runeStartAtOrBefore(text[start:], maxChunkChars)
		}
		spans = append(spans, text[start:start+cut])
		// The next span begins overlapChars back, so a sentence crossing this
		// cut is whole in one of the two. A boundary at or inside the overlap
		// width would otherwise hand back a start no further along than this
		// one, and the walk would never terminate.
		//
		// Backed off to a RUNE START, exactly as the fallback cut above is and
		// for the same reason — this is the SECOND place a byte offset becomes
		// a span boundary, and it is the one that bites ordinary European
		// prose: overlapChars is a byte count, so a step back of 120 from any
		// boundary lands mid-rune whenever the preceding text holds an odd
		// number of continuation bytes. Measured on random German documents it
		// happened in four fifths of them.
		next := runeStartAtOrBefore(text[start:], cut-overlapChars) + start
		if next <= start {
			next = start + cut
		}
		start = next
	}
	return spans
}

// runeStartAtOrBefore backs a byte offset off to the start of a rune, so a
// fallback cut never splits one.
//
// It walks back at most three bytes — the longest UTF-8 continuation run — and
// gives up rather than looping, because a cut that walked back indefinitely
// through invalid bytes could return 0 and stall the chunker. Invalid input is
// then cut where it was going to be cut, which is no worse than before and
// still terminates.
func runeStartAtOrBefore(text string, offset int) int {
	if offset > len(text) {
		offset = len(text)
	}
	for back := 0; back < utf8.UTFMax && offset-back > 0; back++ {
		if utf8.RuneStart(text[offset-back]) {
			return offset - back
		}
	}
	return offset
}

// bestBoundary returns the offset just past the latest paragraph break, else
// sentence end, else space in the window — the order being how much a reader
// would agree the text divides there. It returns 0 when the window holds none.
func bestBoundary(window string) int {
	for _, sep := range []string{"\n\n", ". ", "\n", " "} {
		if ix := strings.LastIndex(window, sep); ix > 0 {
			return ix + len(sep)
		}
	}
	return 0
}
