// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// The account arc: the few stretches of this relationship that still bear on
// today's meeting.
//
// Built in three steps, each of which throws away more than it keeps. Threads
// separated by a long silence become separate MOMENTS, because a gap is how a
// relationship punctuates itself — the week everyone argued about scope is one
// moment, the three weeks of nothing after it are the boundary. Moments are
// then RANKED, and the top few are put back in date order.
//
// Ranked by relevance rather than recency, which is the whole difference from
// the ten-newest-rows reader this replaces. A commitment made in July outranks
// forty pieces of small talk in August, and on an account with any volume the
// recency ordering buries exactly the thing the meeting is about.

import (
	"fmt"
	"sort"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
)

// arcGapDays is the silence that closes a moment. Three weeks is long enough
// that people came back to a subject rather than continued it, and short
// enough that one negotiation does not fragment into five.
const arcGapDays = 21

// arcCap is how many moments a reader gets. The arc is the second thing they
// read, not the record: five is a shape, twelve is a timeline.
const arcCap = 5

// arcEvidenceCap bounds the citations on one moment's sentence. Three receipts
// are checkable; a moment citing eleven activities is a reader's homework.
const arcEvidenceCap = 3

// ArcMoment is a stretch of the relationship, with the conversations it holds.
type ArcMoment struct {
	From    time.Time
	To      time.Time
	Title   string
	Threads []thread
	// Score is why this moment survived, kept for the ranking's own tests.
	Score int
}

// clusterThreads groups threads into moments by the silences between them.
func clusterThreads(threads []thread) []ArcMoment {
	var moments []ArcMoment
	for _, current := range threads {
		if len(moments) > 0 {
			open := &moments[len(moments)-1]
			if elapsed.Days(open.To, current.First) <= arcGapDays {
				open.Threads = append(open.Threads, current)
				if current.Last.After(open.To) {
					open.To = current.Last
				}
				continue
			}
		}
		moments = append(moments, ArcMoment{
			From: current.First, To: current.Last, Threads: []thread{current},
		})
	}
	return moments
}

// rankMoments scores each moment, keeps the top arcCap, and puts them back in
// date order — a reader reads an arc forwards, whatever the ranking decided.
func rankMoments(moments []ArcMoment, in Input) []ArcMoment {
	sources := claimSourceIDs(in)
	scored := make([]ArcMoment, 0, len(moments))
	for _, moment := range moments {
		moment.Score = scoreMoment(moment, sources, in.Now)
		moment.Title = titleOf(moment)
		scored = append(scored, moment)
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		// A tie goes to the more recent: same weight, fresher bearing.
		return scored[i].To.After(scored[j].To)
	})
	if len(scored) > arcCap {
		scored = scored[:arcCap]
	}
	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].From.Before(scored[j].From)
	})
	return scored
}

// claimSourceIDs are the activities a captured promise, question or decision
// was read from. A moment holding one of them is a moment where something was
// AGREED, which outranks any amount of correspondence.
func claimSourceIDs(in Input) map[string]bool {
	sources := map[string]bool{}
	for _, claim := range in.Commitments {
		if claim.SourceID != "" {
			sources[claim.SourceID] = true
		}
	}
	return sources
}

// scoreMoment weighs what a moment IS, not how much of it there was.
//
// Each signal counts ONCE per moment however many threads carry it. Summing per
// thread made the score a volume count wearing a relevance label: five ordinary
// two-way threads outscored the single conversation where a promise was made,
// four to one, and the arc filled with whichever week had the most email. That
// is the exact failure the ranking exists to prevent, so the signals are asked
// as questions about the moment — was something agreed here, did we meet, is
// this on the deal, did they reply — and each answers once.
func scoreMoment(moment ArcMoment, claimSources map[string]bool, now time.Time) int {
	var agreed, met, onDeal, twoWay bool
	for _, current := range moment.Threads {
		for _, id := range current.IDs {
			if claimSources[id] {
				agreed = true
				break
			}
		}
		met = met || current.HasMeeting
		onDeal = onDeal || current.OnDeal
		twoWay = twoWay || current.Inbound > 0
	}
	score := 0
	if agreed {
		// Something was promised, asked or decided here. It outweighs every
		// other signal together, which is what keeps a July commitment in the
		// arc against an August of small talk.
		score += 8
	}
	if met {
		score += 3
	}
	if onDeal {
		score += 2
	}
	if twoWay {
		// They wrote back. A thread nobody answered is us talking.
		score++
	}
	switch days := elapsed.Days(moment.To, now); {
	case days <= 30:
		score += 3
	case days <= 90:
		score += 2
	default:
		score++
	}
	return score
}

// titleOf names a moment after the conversation that defines it: a meeting
// where there was one, otherwise the thread with the most in it.
func titleOf(moment ArcMoment) string {
	best := ""
	bestWeight := -1
	for _, current := range moment.Threads {
		if current.Subject == "" {
			continue
		}
		weight := len(current.IDs)
		if current.HasMeeting {
			weight += 100
		}
		if weight > bestWeight {
			best, bestWeight = current.Subject, weight
		}
	}
	return best
}

// accountArc renders the moments as cited sentences.
//
// A moment whose every conversation is withheld from this caller is dropped
// rather than titled: it has no readable subject to name and no citation they
// could open. Its existence is still reported, in the activity_history
// omission, which is the honest place for "there is more here you cannot see".
func accountArc(in Input) []ArcMoment {
	if len(in.History) == 0 {
		return nil
	}
	moments := rankMoments(clusterThreads(threadsOf(in.History)), in)
	kept := make([]ArcMoment, 0, len(moments))
	for _, moment := range moments {
		if readableIDs(moment) == nil {
			continue
		}
		kept = append(kept, moment)
	}
	return kept
}

// readableIDs are the moment's activities this caller can open, newest first,
// capped at what a reader will actually check.
func readableIDs(moment ArcMoment) []string {
	var ids []string
	for _, current := range moment.Threads {
		if len(current.IDs) == 0 {
			continue
		}
		for _, id := range current.IDs {
			ids = append(ids, id)
			if len(ids) == arcEvidenceCap {
				return ids
			}
		}
	}
	return ids
}

// arcSummary is the moment's one line: how much happened, over what stretch.
func arcSummary(moment ArcMoment, conversations int) string {
	span := formatDay(moment.From)
	if formatDay(moment.To) != span {
		span = fmt.Sprintf("%s–%s", span, formatDay(moment.To))
	}
	if moment.Title == "" {
		return fmt.Sprintf("%s: %s.", span, plural(conversations, "conversation"))
	}
	return fmt.Sprintf("%s: %s, on %s.", span, plural(conversations, "conversation"), moment.Title)
}

func formatDay(at time.Time) string {
	return at.UTC().Format("2 Jan")
}

func plural(count int, noun string) string {
	if count == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", count, noun)
}
