// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"testing"
	"time"

	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// The fingerprint is what decides freshness, so these prove the two things a
// reader would notice: a card that is rewritten when it should not be costs
// money, and a card that is NOT rewritten when the deal moved is a lie.

func TestTheFingerprintMovesWhenTheFactsDo(t *testing.T) {
	reader := ids.NewV7()
	in := inputWithTimeline()
	before, err := Fingerprint(in, reader, "routing-1")
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	in.Timeline = append(in.Timeline, ActIn{ID: "act-3", Kind: "email", At: "2026-08-22"})
	after, err := Fingerprint(in, reader, "routing-1")
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if before == after {
		t.Fatal("logging an activity left the fingerprint unchanged, so the card would be served stale")
	}
}

func TestTheSameFactsFingerprintTheSame(t *testing.T) {
	reader := ids.NewV7()
	first, err := Fingerprint(inputWithTimeline(), reader, "routing-1")
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	second, err := Fingerprint(inputWithTimeline(), reader, "routing-1")
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if first != second {
		t.Fatal("an unchanged deal fingerprinted differently, so every page load would pay for a rewrite")
	}
}

func TestTwoReadersNeverShareACard(t *testing.T) {
	// The input was assembled under the reader's row scope, so a card written
	// for one is not a card the other may read.
	in := inputWithTimeline()
	mine, err := Fingerprint(in, ids.NewV7(), "routing-1")
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	theirs, err := Fingerprint(in, ids.NewV7(), "routing-1")
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if mine == theirs {
		t.Fatal("two readers fingerprinted the same, so one could be served the other's card")
	}
}

func TestANewRoutingVersionRewritesTheCard(t *testing.T) {
	reader := ids.NewV7()
	in := inputWithTimeline()
	old, err := Fingerprint(in, reader, "routing-1")
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	fresh, err := Fingerprint(in, reader, "routing-2")
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if old == fresh {
		t.Fatal("rebinding the model left cards written by the old one standing")
	}
}

func TestTheFloorIsShorterThanAReadersPatience(t *testing.T) {
	// A floor long enough to notice would make repeated reads of a busy deal
	// read as deterministic for minutes at a time.
	if modelCallFloor > 10*time.Minute {
		t.Fatalf("modelCallFloor = %v, long enough that a reader would see the composition instead", modelCallFloor)
	}
}
