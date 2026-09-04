// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// TestTheBucketsPartitionTheDay is the property the additive sentence rests on.
//
// The four figures are drawn as "3 urgent · 5 due today · 4 planned · 5 review
// — 17 total", so a day where they do not add up is a sentence that contradicts
// itself in front of the reader. Asserted over a MIXED day rather than a
// uniform one: a fixture of one kind of row would sum correctly under a
// classifier that sent everything to one bucket.
func TestTheBucketsPartitionTheDay(t *testing.T) {
	t.Parallel()
	day := mixedDestinationsDay()
	out := (&Service{}).worklistFrom(context.Background(), day, scopeAll, "", 50,
		waitingRead{}, leadRead{}, worklistCursor{}, nil)

	buckets := out.Summary.Buckets
	if buckets == nil {
		t.Fatal("the summary carries no buckets, so the additive sentence has nothing to draw")
	}
	sum := buckets.Urgent + buckets.DueToday + buckets.Planned + buckets.Review
	if sum != out.Summary.Total {
		t.Errorf("the buckets sum to %d over a day of %d: %+v — the sentence would not add up",
			sum, out.Summary.Total, *buckets)
	}
	// A partition of one bucket is a partition that proves nothing. This fixture
	// holds seller work AND judgements, so a classifier that sent every row to
	// one screen still sums correctly and is still wrong.
	if buckets.Review == 0 {
		t.Error("no row reached the review bucket, so this fixture cannot tell a partition from a single bucket")
	}
	if buckets.Urgent+buckets.DueToday+buckets.Planned == 0 {
		t.Error("no row reached a seller bucket, so this fixture cannot tell a partition from a single bucket")
	}
}

// TestAJudgementIsNeverUrgentSellerWork holds the precedence's leading arm.
//
// An approval that has waited is not a seller's morning however long it has
// waited. Reading the level first would count it as urgent and put it in a
// sentence a rep reads as work they can execute.
func TestAJudgementIsNeverUrgentSellerWork(t *testing.T) {
	t.Parallel()
	buckets := crmcontracts.WorklistBuckets{}
	judgement := ranked{item: crmcontracts.WorklistItem{
		Source: crmcontracts.WorklistItemSourceApproval,
		Level:  levelWaiting,
	}}
	bucketOf(judgement, levelWaiting, &buckets)
	if buckets.Urgent != 0 {
		t.Error("an approval at the waiting level was counted as urgent seller work")
	}
	if buckets.Review != 1 {
		t.Errorf("an approval reached %+v, want it counted as review", buckets)
	}
}

// TestAnOverdueSellerRowIsCountedOnceHoldsThePrecedence.
//
// `due` beside it is asked of every row whatever its level, so an overdue
// promise counts twice there — deliberately. The buckets slice instead, so the
// same row must reach exactly one of them.
func TestAnOverdueSellerRowIsCountedOnce(t *testing.T) {
	t.Parallel()
	overdue := true
	buckets := crmcontracts.WorklistBuckets{}
	promise := ranked{item: crmcontracts.WorklistItem{
		Source:  crmcontracts.WorklistItemSourceConversationClaim,
		Level:   levelPromise,
		Overdue: &overdue,
	}}
	bucketOf(promise, levelPromise, &buckets)
	total := buckets.Urgent + buckets.DueToday + buckets.Planned + buckets.Review
	if total != 1 {
		t.Errorf("one overdue promise landed in %d buckets: %+v", total, buckets)
	}
	if buckets.Urgent != 1 {
		t.Errorf("an overdue promise reached %+v, want urgent — the level leads inside seller work", buckets)
	}
}

// TestAnOverdueTaskIsDueRatherThanPlanned separates the two seller buckets.
//
// Without it the partition still sums and every other assertion here still
// passes while `due_today` is dead: an overdue task falls through to `planned`,
// the four figures add up, and the sentence tells a rep nothing is due while a
// deadline sits in their queue. Found by mutation — removing the due arm broke
// no test until this one existed.
func TestAnOverdueTaskIsDueRatherThanPlanned(t *testing.T) {
	t.Parallel()
	overdue := true
	buckets := crmcontracts.WorklistBuckets{}
	task := ranked{item: crmcontracts.WorklistItem{
		Source:  crmcontracts.WorklistItemSourceTask,
		Level:   levelAgreed,
		Overdue: &overdue,
	}}
	bucketOf(task, levelAgreed, &buckets)
	if buckets.DueToday != 1 {
		t.Errorf("an overdue task reached %+v, want it counted as due today", buckets)
	}
	if buckets.Planned != 0 {
		t.Error("an overdue task was counted as planned, so the sentence says nothing is due")
	}
	// And the same task without the deadline is planned, which is what makes
	// the assertion above about the DEADLINE rather than about the source.
	fresh := crmcontracts.WorklistBuckets{}
	bucketOf(ranked{item: crmcontracts.WorklistItem{
		Source: crmcontracts.WorklistItemSourceTask,
		Level:  levelAgreed,
	}}, levelAgreed, &fresh)
	if fresh.Planned != 1 {
		t.Errorf("a task with no deadline reached %+v, want planned", fresh)
	}
}

// mixedDestinationsDay is a day holding both seller work and judgements, so a
// partition over it can be told from a single bucket.
//
// The approvals are what make the fixture worth having: a day of tasks alone
// sums correctly under any classifier, including one that sends every row to
// the same place.
func mixedDestinationsDay() crmcontracts.Attention {
	return crmcontracts.Attention{
		AsOf: rankInstant,
		Planned: []crmcontracts.AttentionItem{
			item("task-soon", "task", withDue(rankInstant.Add(time.Hour))),
			item("task-later", "task", withDue(rankInstant.Add(48*time.Hour))),
		},
		NeedsYou: []crmcontracts.AttentionItem{
			item("approval-one", "approval"),
			item("approval-two", "approval"),
		},
	}
}
