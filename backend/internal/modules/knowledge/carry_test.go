// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

import (
	"slices"
	"testing"
)

const liveBinding = "fake/embed@8"

func heldVector(text, identity string, first float32) carriedVector {
	return carriedVector{hash: Chunk{Text: text}.Hash(), identity: identity, vector: []float32{first, 0.5}}
}

func TestAPassageWhoseTextADocumentAlreadyHeldTakesThatVector(t *testing.T) {
	carried := carriedVectors{}
	carried.add("Kept as it was.", heldVector("Kept as it was.", liveBinding, 0.1))
	pending := []pendingChunk{{text: "Kept as it was."}, {text: "Reworded since."}}

	reused, vectors, owed := carried.take(pending, liveBinding)
	if len(reused) != 1 || reused[0].text != "Kept as it was." || !slices.Equal(vectors[0], []float32{0.1, 0.5}) {
		t.Fatalf("reused %+v with %v, want the unchanged passage with its own vector", reused, vectors)
	}
	if len(owed) != 1 || owed[0].text != "Reworded since." {
		t.Fatalf("owed %+v, want only the reworded passage", owed)
	}
}

func TestAVectorFromAnotherBindingAnswersNoPassage(t *testing.T) {
	carried := carriedVectors{}
	carried.add("Kept as it was.", heldVector("Kept as it was.", "fake/older@8", 0.1))

	reused, _, owed := carried.take([]pendingChunk{{text: "Kept as it was."}}, liveBinding)
	if len(reused) != 0 || len(owed) != 1 {
		t.Fatalf("reused %d and owed %d, want the passage owed a model call under the new binding", len(reused), len(owed))
	}
}

// Of two bindings a document held for one text, the live one is taken.
func TestTheLiveBindingsVectorIsTakenOfSeveralForOneText(t *testing.T) {
	carried := carriedVectors{}
	carried.add("Kept as it was.", heldVector("Kept as it was.", "fake/older@8", 0.1))
	carried.add("Kept as it was.", heldVector("Kept as it was.", liveBinding, 0.2))

	_, vectors, _ := carried.take([]pendingChunk{{text: "Kept as it was."}}, liveBinding)
	if len(vectors) != 1 || vectors[0][0] != 0.2 {
		t.Fatalf("took %v, want the live binding's vector", vectors)
	}
}

// A row whose text no longer hashes to its stored hash holds a vector of prose
// it lost. That vector is never handed to a passage.
func TestARowWhoseTextDisagreesWithItsHashCarriesNothing(t *testing.T) {
	carried := carriedVectors{}
	carried.add("Edited outside the writer.", heldVector("Kept as it was.", liveBinding, 0.1))

	if reused, _, _ := carried.take([]pendingChunk{{text: "Kept as it was."}}, liveBinding); len(reused) != 0 {
		t.Fatalf("a vector of lost prose was reused for %+v", reused)
	}
}

// A first ingest holds nothing to carry, and every passage is owed.
func TestNothingCarriedLeavesEveryPassageOwed(t *testing.T) {
	var carried carriedVectors
	pending := []pendingChunk{{text: "One."}, {text: "Two."}}

	if reused, _, owed := carried.take(pending, liveBinding); len(reused) != 0 || len(owed) != 2 {
		t.Fatalf("reused %d and owed %d, want both owed", len(reused), len(owed))
	}
}
