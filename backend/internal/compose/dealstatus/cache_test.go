// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"testing"
	"time"

	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/textlang"
)

// The fingerprint is what decides freshness, so these prove the two things a
// reader would notice: a card that is rewritten when it should not be costs
// money, and a card that is NOT rewritten when the deal moved is a lie.

func TestTheFingerprintMovesWhenTheFactsDo(t *testing.T) {
	reader := ids.NewV7()
	in := inputWithTimeline()
	before, err := Fingerprint(in, reader, "routing-1", testNow, string(textlang.English))
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	in.Timeline = append(in.Timeline, ActIn{ID: "act-3", Kind: "email", At: "2026-08-22"})
	after, err := Fingerprint(in, reader, "routing-1", testNow, string(textlang.English))
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if before == after {
		t.Fatal("logging an activity left the fingerprint unchanged, so the card would be served stale")
	}
}

func TestTheSameFactsFingerprintTheSame(t *testing.T) {
	reader := ids.NewV7()
	first, err := Fingerprint(inputWithTimeline(), reader, "routing-1", testNow, string(textlang.English))
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	second, err := Fingerprint(inputWithTimeline(), reader, "routing-1", testNow, string(textlang.English))
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if first != second {
		t.Fatal("an unchanged deal fingerprinted differently, so every page load would pay for a rewrite")
	}
}

func TestTheCardGoesStaleWhenTheDayTurns(t *testing.T) {
	// The card says "the last contact was 4 days ago" — prose rendered from
	// the clock over facts that have not moved. Without the day in the key a
	// card written on Monday still says "4 days ago" on Friday.
	reader := ids.NewV7()
	in := inputWithTimeline()
	monday, err := Fingerprint(in, reader, "routing-1", testNow, string(textlang.English))
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	friday, err := Fingerprint(in, reader, "routing-1", testNow.AddDate(0, 0, 4), string(textlang.English))
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if monday == friday {
		t.Fatal("four days passed and the fingerprint did not move, so the card still says the old age")
	}
}

func TestTheSameDayDoesNotRewriteAQuietDeal(t *testing.T) {
	// A day's granularity is what those sentences resolve to. Anything finer
	// would rewrite a deal nobody touched, for prose that reads identically.
	reader := ids.NewV7()
	in := inputWithTimeline()
	morning, err := Fingerprint(in, reader, "routing-1", testNow, string(textlang.English))
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	evening, err := Fingerprint(in, reader, "routing-1", testNow.Add(7*time.Hour), string(textlang.English))
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	if morning != evening {
		t.Fatal("the same day fingerprinted differently, so a quiet deal pays for a rewrite on every read")
	}
}

func TestANewRoutingVersionRewritesTheCard(t *testing.T) {
	reader := ids.NewV7()
	in := inputWithTimeline()
	old, err := Fingerprint(in, reader, "routing-1", testNow, string(textlang.English))
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	fresh, err := Fingerprint(in, reader, "routing-2", testNow, string(textlang.English))
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

func TestTheWordingChangesOnlyWhenTheCacheKeyDoes(t *testing.T) {
	// The card is keyed on the UTC day, and its "yesterday"/"3 days ago"
	// wording has to turn over on the same boundary. Counting elapsed hours
	// instead would flip the words mid-afternoon while the key waited for
	// midnight, and the card would spend the gap saying the wrong thing with
	// nothing able to notice.
	contact := time.Date(2026, 8, 20, 14, 0, 0, 0, time.UTC)
	sameDay := time.Date(2026, 8, 20, 23, 30, 0, 0, time.UTC)
	if got := since(sameDay, contact); got != "today" {
		t.Fatalf("late on the same day = %q, want today", got)
	}
	// 24 hours have NOT elapsed, but the calendar day has turned — and so has
	// the cache key, so the wording must turn with it.
	justAfterMidnight := time.Date(2026, 8, 21, 0, 30, 0, 0, time.UTC)
	if got := since(justAfterMidnight, contact); got != "yesterday" {
		t.Fatalf("just after midnight = %q, want yesterday", got)
	}
	// The reverse: 24 hours HAVE elapsed but the day has not turned again.
	sameDayNextAfternoon := time.Date(2026, 8, 21, 15, 0, 0, 0, time.UTC)
	if got := since(sameDayNextAfternoon, contact); got != "yesterday" {
		t.Fatalf("a day later in the afternoon = %q, want yesterday — the key has not moved again", got)
	}
}
