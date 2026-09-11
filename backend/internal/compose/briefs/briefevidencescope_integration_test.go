// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package briefs

// A brief cites activities, and a cited id is a read of that activity. The deal
// it hangs off being readable says nothing about the activity: a message under a
// workspace-readable deal can be limited to the people who were on it, and its
// id and its moment are the protected fact as much as its body is.

import (
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// narrowAwayFromTheRep limits one activity to a colleague, through the audience
// writer a human uses. The rep reading the brief is not in that audience.
func narrowAwayFromTheRep(t *testing.T, b *briefEnv) {
	t.Helper()
	_, err := activities.NewStore(b.DB()).SetAudience(b.Admin(), ids.From[ids.ActivityKind](b.activityOnA),
		activities.SetAudienceInput{
			Audience: "selected",
			Members:  []activities.AudienceMember{{SubjectType: "user", SubjectID: b.Rep3}},
		})
	if err != nil {
		t.Fatalf("limiting the activity to a colleague: %v", err)
	}
}

// queuedEvidence answers the evidence a ranking cites for one deal.
func queuedEvidence(t *testing.T, ranking BriefRanking, dealID ids.UUID) []ids.UUID {
	t.Helper()
	for _, item := range ranking.Queue {
		if item.DealID == dealID {
			return item.EvidenceIDs
		}
	}
	t.Fatalf("deal %s is not in the queue, so its evidence cannot be checked", dealID)
	return nil
}

func TestTheRankingDoesNotRestOnAnActivityTheRepIsNotInTheAudienceOf(t *testing.T) {
	b := setupBrief(t)

	// The positive control first: the same rep, the same activity, cited while
	// its audience is the workspace — so the absence below is the audience.
	before, err := b.engine.Rank(b.repCtx, briefClock)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(queuedEvidence(t, before, b.dealA), b.activityOnA) {
		t.Fatalf("the fixture's activity is not cited before it is narrowed, so nothing below tests the audience")
	}

	narrowAwayFromTheRep(t, b)
	after, err := b.engine.Rank(b.repCtx, briefClock)
	if err != nil {
		t.Fatal(err)
	}
	evidence := queuedEvidence(t, after, b.dealA)
	if slices.Contains(evidence, b.activityOnA) {
		t.Error("the ranking cited an activity limited to its participants to a rep who is not one")
	}
	if len(evidence) == 0 || evidence[0] != b.dealA {
		t.Errorf("the deal's own id left its evidence too: %v", evidence)
	}
}

func TestAServedBriefDropsAnActivityNarrowedAfterTheRunCitedIt(t *testing.T) {
	b := setupBrief(t)

	run, err := b.engine.SnapshotRun(b.repCtx, briefClock)
	if err != nil {
		t.Fatal(err)
	}
	cited, queued := itemsByDeal(t, run)[b.dealA]
	if !queued || !slices.Contains(cited.EvidenceIDs, b.activityOnA) {
		t.Fatalf("the run did not cite the fixture's activity on deal A (queued=%v), so nothing below tests the read", queued)
	}

	// Somebody limits the message after the run recorded it. The run is served
	// for the rest of the day, and the stored id must not outlive the limit.
	narrowAwayFromTheRep(t, b)
	served, err := b.engine.LatestRun(b.repCtx, briefClock.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	item, present := itemsByDeal(t, served)[b.dealA]
	if !present {
		t.Fatal("the served run lost deal A itself; only the narrowed activity should have gone")
	}
	if slices.Contains(item.EvidenceIDs, b.activityOnA) {
		t.Error("the served brief still cites an activity narrowed away from its reader after the run recorded it")
	}
	if len(item.EvidenceIDs) == 0 || item.EvidenceIDs[0] != b.dealA {
		t.Errorf("the served evidence lost the deal's own id: %v", item.EvidenceIDs)
	}
}
