// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/retentionscope"
)

// activityKeeper is one thing that keeps the activity aliased a from a
// destructive retention action, and the reason the archive review reports.
type activityKeeper struct {
	reason string
	holds  string
}

// activityKeepers is what keeps an activity. That is a statutory restriction, a
// legal hold on a linked record, an opt-out receipt inside its floor, or a
// Handelsbrief inside the statutory window. The nightly selector, the archive
// deletion and the copy retention all read it. So none of them deletes what
// another keeps. currentArg and carriedArg are the floor's classes.
func activityKeepers(currentArg, carriedArg int) []activityKeeper {
	return []activityKeeper{
		{reason: keptByLaw, holds: "a.restricted_at IS NOT NULL"},
		{reason: keptLegalHold, holds: retentionscope.HeldThroughAnyLink("a.id")},
		{reason: keptOptOutRecord, holds: receiptKept("a.id")},
		{reason: keptByLaw, holds: handelsbriefShielded(currentArg, carriedArg)},
	}
}

// excluding is the filter arm that leaves out the rows predicate holds.
func excluding(predicate string) string {
	return "AND NOT (" + predicate + ")"
}

// unkeptActivitySQL is the filter that leaves out every activity a keeper
// holds.
func unkeptActivitySQL(currentArg, carriedArg int) string {
	arms := []string{}
	for _, keeper := range activityKeepers(currentArg, carriedArg) {
		arms = append(arms, excluding(keeper.holds))
	}
	return strings.Join(arms, "\n\t\t  ")
}

// keptActivitySQL holds when any keeper holds the activity.
func keptActivitySQL(currentArg, carriedArg int) string {
	arms := []string{}
	for _, keeper := range activityKeepers(currentArg, carriedArg) {
		arms = append(arms, "("+keeper.holds+")")
	}
	return strings.Join(arms, " OR ")
}

// activityKeptReasonSQL names the first keeper that holds the activity, or
// empty when none does.
func activityKeptReasonSQL(currentArg, carriedArg int) string {
	var b strings.Builder
	b.WriteString("CASE")
	for _, keeper := range activityKeepers(currentArg, carriedArg) {
		b.WriteString(" WHEN " + keeper.holds + " THEN '" + keeper.reason + "'")
	}
	b.WriteString(" ELSE '' END")
	return b.String()
}
