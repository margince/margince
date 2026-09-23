// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

// RankInMemory — a DECLARED MIRROR of the ranking Store.Retrieve does in SQL.
//
// There are now two implementations of one decision in this tree, and that is a
// hazard, so it is written down rather than implied. The SQL in rankIn is the
// original and the only one production ever runs. This one exists because the
// corpus_ask evaluation loop (`worker aitask retrieve`) must run against a bare
// checkout with no Postgres behind it: an operator reproducing a field failure
// should not have to stand up a stack, and the handbook the failure was asked
// of ships inside the binary. That was a product decision, taken knowing the
// cost — a copy can go green while the original is broken.
//
// WHAT KEEPS THEM HONEST:
//
//   - Neither number is retyped. RetrieveLimit is read by both, and the floor
//     arrives as an argument from whoever knows it — the corpus row in SQL, a
//     flag defaulting to DefaultMinSimilarity in the eval loop.
//   - Postgres leaves TIES in an arbitrary order and this pins them by document
//     and start line, so the two can differ among passages of identical
//     similarity — which is outside what the gate below covers, because its
//     fixture places every passage at a distance of its own. Real embeddings do
//     not tie; a fixture that did would be measuring float equality.
//   - TestTheInMemoryRankerSelectsExactlyWhatTheSQLDoes, in the integration
//     lane, ingests one corpus through the real ingest path and runs BOTH over
//     it, asserting identical selection AND identical order. It is what this
//     file's existence is conditional on. When it goes red, the eval loop and
//     production disagree about which passages an answer may rest on, and every
//     scenario captured since is measuring something production does not do.
//
// The order of operations is part of what is mirrored and is easy to get
// subtly wrong: rankIn takes the closest RetrieveLimit passages FIRST and
// groundedIn applies the floor to those. Filtering by the floor first and then
// taking eight would return a different set whenever more than eight passages
// clear it.

import (
	"math"
	"sort"
)

// EmbeddedPassage is one chunk with its vector: the row the SQL ranks, held in
// memory instead. DocumentName is carried rather than looked up because there
// is no join here to carry it.
type EmbeddedPassage struct {
	DocumentName string
	Chunk        Chunk
	Vector       []float32
}

// RankInMemory returns the passages an ask would be grounded in, given every
// embedded passage of a corpus and the floor that corpus declares.
//
// Vectors of a width other than the question's are skipped rather than ranked.
// That is a CRASH GUARD and not the identity filter the SQL's
// `c.embed_identity = $3` predicate is: two bindings of the same width live in
// different spaces and this check admits both. What keeps one binding's vectors
// in front of this function is the probe's cache, which is keyed per identity —
// so the caller, not this loop, is what makes the comparison meaningful.
func RankInMemory(question []float32, corpus []EmbeddedPassage, floor float64) []Passage {
	ranked := make([]Passage, 0, len(corpus))
	for _, p := range corpus {
		if len(p.Vector) != len(question) {
			continue
		}
		ranked = append(ranked, Passage{
			DocumentName: p.DocumentName,
			Text:         p.Chunk.Text,
			StartLine:    p.Chunk.StartLine,
			Similarity:   cosineSimilarity(question, p.Vector),
		})
	}
	// Stable, and by document then start line on a tie: Postgres is free to
	// return equal distances in any order, and a ranker that reshuffled them
	// run to run would make two captures of one question differ for no reason
	// anybody could explain.
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Similarity != ranked[j].Similarity {
			// DESCENDING, mirroring `ORDER BY sim DESC`: the closest passages
			// are the ones an answer may rest on. Ascending here returns the
			// eight FURTHEST passages that clear the floor, which reads as a
			// plausible retrieval and is the opposite of one.
			return ranked[i].Similarity > ranked[j].Similarity
		}
		if ranked[i].DocumentName != ranked[j].DocumentName {
			return ranked[i].DocumentName < ranked[j].DocumentName
		}
		return ranked[i].StartLine < ranked[j].StartLine
	})
	if len(ranked) > RetrieveLimit {
		ranked = ranked[:RetrieveLimit]
	}
	var grounded []Passage
	for _, p := range ranked {
		if p.Similarity >= floor {
			grounded = append(grounded, p)
		}
	}
	return grounded
}

// cosineSimilarity is `1 - (a <=> b)`, the expression rankIn projects.
//
// Zero for a zero vector on either side rather than NaN. Postgres never reaches
// that case — a zero vector is refused before storage, because `ORDER BY sim
// DESC` sorts NaN first and one stored zero would outrank the whole corpus —
// and answering 0 here keeps the same passage unciteable instead of first.
func cosineSimilarity(a, b []float32) float64 {
	var dot, normA, normB float64
	for i := range a {
		x, y := float64(a[i]), float64(b[i])
		dot += x * y
		normA += x * x
		normB += y * y
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
