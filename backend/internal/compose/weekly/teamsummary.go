// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// The paragraph at the top of the team's review.
//
// Composed by fixed rules from the figures the snapshot freezes, never by a
// model, so it says nothing the snapshot does not hold. Six rules, at most one
// sentence each, in order: counted and closed, help asked, month and quarter
// coverage, the most common pattern, the strong weeks.

import (
	"fmt"
	"slices"
	"strings"
)

// basisWords is how a sentence names what coverage was measured against.
var basisWords = map[string]string{
	"manager_call":      "the manager's call",
	"historical_median": "the historical median",
}

// summarise composes the summary for a review whose figures are all known.
func summarise(review TeamReview) string {
	if review.Counts.RepsCounted == 0 {
		return ""
	}
	sentences := []string{countedSentence(review)}
	if names := repsWith(review.Reps, func(r TeamRep) bool { return r.HelpRequested > 0 }); len(names) > 0 {
		sentences = append(sentences, "Asked for help: "+strings.Join(names, ", ")+".")
	}
	for _, kind := range []string{"month", "quarter"} {
		if s := coverageSentence(review.Coverage, kind); s != "" {
			sentences = append(sentences, s)
		}
	}
	if s := commonPatternSentence(review.Reps); s != "" {
		sentences = append(sentences, s)
	}
	if names := repsWith(review.Reps, func(r TeamRep) bool { return r.FocusKind == FocusStrongWeek }); len(names) > 0 {
		sentences = append(sentences, "Strong week: "+strings.Join(names, ", ")+".")
	}
	return strings.Join(sentences, " ")
}

func countedSentence(review TeamReview) string {
	c := review.Counts
	members := c.RepsCounted + review.RepsUnread
	return fmt.Sprintf("%d of %d members counted; %s won and %d lost.",
		c.RepsCounted, members, plural(c.DealsWon, "deal"), c.DealsLost)
}

// coverageSentence names one horizon's coverage against its basis, or nothing
// when the horizon had no basis to read it against.
func coverageSentence(coverage []Coverage, kind string) string {
	i := slices.IndexFunc(coverage, func(c Coverage) bool { return c.PeriodKind == kind })
	if i < 0 || coverage[i].Basis == "" {
		return ""
	}
	c := coverage[i]
	horizon := strings.ToUpper(kind[:1]) + kind[1:]
	percent := c.CoverageBp / 100
	if c.ReplanRecommended {
		return fmt.Sprintf("%s: coverage at risk against %s (%d%%); a replan is recommended.",
			horizon, basisWords[c.Basis], percent)
	}
	return fmt.Sprintf("%s: coverage holds against %s (%d%%).", horizon, basisWords[c.Basis], percent)
}

// commonPatternSentence names the pattern the most reps share. Ties go to the
// one a lead raises first, so the sentence does not change between two reads.
func commonPatternSentence(reps []TeamRep) string {
	counts := map[string]int{}
	for _, rep := range reps {
		for _, p := range rep.Patterns {
			counts[p.Kind]++
		}
	}
	best, bestCount := "", 0
	for _, kind := range focusPriority {
		if counts[kind] > bestCount {
			best, bestCount = kind, counts[kind]
		}
	}
	if bestCount < 2 {
		return ""
	}
	return fmt.Sprintf("Most common pattern: %s (%d members).", strings.ReplaceAll(best, "_", " "), bestCount)
}

// repsWith names the reps a rule picks, in the order the snapshot holds them.
func repsWith(reps []TeamRep, picked func(TeamRep) bool) []string {
	var names []string
	for _, rep := range reps {
		if picked(rep) {
			names = append(names, rep.DisplayName)
		}
	}
	return names
}
