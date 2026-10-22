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

// summarise composes the summary for a review whose figures are all known.
func summarise(review TeamReview) string {
	said := teamCopyFor(review.Language)
	if review.Counts.RepsCounted == 0 {
		return ""
	}
	sentences := []string{said.countedSentence(review)}
	if names := repsWith(review.Reps, func(r TeamRep) bool { return r.HelpRequested > 0 }); len(names) > 0 {
		sentences = append(sentences, fmt.Sprintf(said.Say(teamWords.Help), strings.Join(names, ", ")))
	}
	for _, kind := range []string{"month", "quarter"} {
		if s := said.coverageSentence(review.Coverage, kind); s != "" {
			sentences = append(sentences, s)
		}
	}
	if s := said.commonPatternSentence(review.Reps); s != "" {
		sentences = append(sentences, s)
	}
	if names := repsWith(review.Reps, func(r TeamRep) bool { return r.FocusKind == FocusStrongWeek }); len(names) > 0 {
		sentences = append(sentences, fmt.Sprintf(said.Say(teamWords.Strong), strings.Join(names, ", ")))
	}
	return strings.Join(sentences, " ")
}

func (said teamCopy) countedSentence(review TeamReview) string {
	c := review.Counts
	members := c.RepsCounted + review.RepsUnread
	if c.DealsWon == 1 {
		return fmt.Sprintf(said.Say(teamWords.CountedOne), c.RepsCounted, members, c.DealsLost)
	}
	return fmt.Sprintf(said.Say(teamWords.Counted), c.RepsCounted, members, c.DealsWon, c.DealsLost)
}

// coverageSentence names one horizon's coverage against its basis, or nothing
// when the horizon had no basis to read it against.
func (said teamCopy) coverageSentence(coverage []Coverage, kind string) string {
	i := slices.IndexFunc(coverage, func(c Coverage) bool { return c.PeriodKind == kind })
	if i < 0 || coverage[i].Basis == "" {
		return ""
	}
	c := coverage[i]
	horizon := said.horizonWord(kind)
	percent := c.CoverageBp / 100
	if c.ReplanRecommended {
		return fmt.Sprintf(said.Say(teamWords.AtRisk),
			horizon, said.basisWord(c.Basis), percent)
	}
	return fmt.Sprintf(said.Say(teamWords.Holds), horizon, said.basisWord(c.Basis), percent)
}

// commonPatternSentence names the pattern the most reps share. Ties go to the
// one a lead raises first, so the sentence does not change between two reads.
func (said teamCopy) commonPatternSentence(reps []TeamRep) string {
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
	return fmt.Sprintf(said.Say(teamWords.Common), said.focusWord(best), bestCount)
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
