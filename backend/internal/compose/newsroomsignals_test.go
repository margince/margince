// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Placing a headline, and deciding what is still news.
//
// The classification is checked against headlines in the shape companies write
// them, in both languages this reads. What matters as much as the hits is the
// MISS: a headline the rules cannot place must come back `other`, because a
// hiring announcement filed as a funding round is worse on an account page than
// an unclassified line.

import (
	"testing"
	"time"
)

func TestClassifyHeadlinePlacesTheGenresACompanyAnnounces(t *testing.T) {
	cases := []struct {
		headline string
		want     string
	}{
		{"Acme raises $12M Series B to expand its platform", kindFunding},
		{"Globex schließt Finanzierungsrunde über 5 Mio. Euro ab", kindFunding},
		{"Initech appoints Jane Doe as Chief Financial Officer", kindLeadershipChange},
		{"Umbrella ernennt neuen Geschäftsführer", kindLeadershipChange},
		{"Acme opens a second office in Lisbon", kindExpansion},
		{"Globex übernimmt Wettbewerber Initech", kindExpansion},
		{"Initech launches its next-generation analytics suite", kindProductLaunch},
		{"Umbrella stellt vor: die neue Plattform", kindProductLaunch},

		// The miss. Both of these are real newsroom genres this vocabulary
		// deliberately does not name, and inventing a kind for them would put a
		// guess on an account page.
		{"Acme wins the 2026 sustainability award", kindOtherEvent},
		{"Our engineering blog: what we learned scaling Postgres", kindOtherEvent},
	}

	for _, tc := range cases {
		t.Run(tc.headline, func(t *testing.T) {
			if got := classifyHeadline(tc.headline); got != tc.want {
				t.Errorf("classified as %q, want %q", got, tc.want)
			}
		})
	}
}

func TestClassifyHeadlineReadsTheFirstRuleThatClaimsIt(t *testing.T) {
	// A funding announcement routinely names the growth it will pay for, and
	// the money is the event. The rule order is what decides it, so the case is
	// here rather than left to whichever rule happened to be first.
	got := classifyHeadline("Acme raises Series C and opens a new office in Porto")
	if got != kindFunding {
		t.Errorf("classified as %q, want %q — the money is the event", got, kindFunding)
	}
}

func TestClassifyHeadlineDoesNotClaimAWordItMerelySitsInside(t *testing.T) {
	// "praises" contains "raises". A substring match files a company praising
	// its new chief executive as having raised money — which is both wrong and
	// the kind of wrong a reader believes, because funding is what they were
	// hoping to see on the account.
	if got := classifyHeadline("Acme praises its new CEO"); got == kindFunding {
		t.Errorf("classified as %q — \"praises\" is not \"raises\"", got)
	}
	if got := classifyHeadline("Acme raises a Series A"); got != kindFunding {
		t.Errorf("classified as %q, want %q", got, kindFunding)
	}
}

func TestStaleDropsAnArchivedAnnouncementAndKeepsAnUndatedOne(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	old := NewsroomItem{Published: now.Add(-2 * newsroomMaxAge)}
	if !stale(old, now) {
		t.Error("a two-year-old press release is still news — a first read would file a whole archive")
	}

	recent := NewsroomItem{Published: now.Add(-24 * time.Hour)}
	if stale(recent, now) {
		t.Error("yesterday's announcement was dropped")
	}

	// No date is not an old date. A CMS that omits the field would otherwise
	// lose the company its whole newsroom.
	if stale(NewsroomItem{}, now) {
		t.Error("an undated item was dropped as stale")
	}
}

func TestDetectedAtDatesASignalByWhenItHappened(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	published := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)

	if got := detectedAt(NewsroomItem{Published: published}, now); !got.Equal(published) {
		t.Errorf("detected at %v, want the publication date %v", got, published)
	}
	// Only a feed that stated nothing falls back to the read.
	if got := detectedAt(NewsroomItem{}, now); !got.Equal(now) {
		t.Errorf("an undated item detected at %v, want the read %v", got, now)
	}
}
