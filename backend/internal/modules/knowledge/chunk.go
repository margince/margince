// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

import "strings"

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
			cut = maxChunkChars
		}
		spans = append(spans, text[start:start+cut])
		// The next span begins overlapChars back, so a sentence crossing this
		// cut is whole in one of the two. A boundary at or inside the overlap
		// width would otherwise hand back a start no further along than this
		// one, and the walk would never terminate.
		next := start + cut - overlapChars
		if next <= start {
			next = start + cut
		}
		start = next
	}
	return spans
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
