// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// WHAT THE DAY COULD NOT SEE, in the two shapes a reader needs it.
//
// A lane can fall short two ways, and telling them apart is the whole of this
// file. It can answer to its own bound — everything it returned is real, and
// there is more behind it — or it can not answer at all, because the caller may
// not read it or because it failed. The first is a limit on depth, the second a
// hole in the day.
//
// Both exist for one reason: a queue must never read as a clear day over
// something it did not actually see. That is the single lie this surface cannot
// tell, and every figure above is derived from these two answers rather than
// from a count of rows that happened to arrive.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// boundedSources names the lanes that came back exactly at their own work
// bound, and therefore may have had more behind them.
//
// A lane read to its limit cannot tell a reader how many it did not see: the
// bound is a limit on work, not a count. So the source is marked as having more
// rather than reporting a total it does not know.
//
// Under-reporting is the one way this must not fail. A source silently marked
// complete tells a rep there is no more work of that kind, and there is no
// failing row to notice — which is why every lane with a bound appears here and
// `TestEveryBoundedLaneIsNamedInTheBoundsTable` fails when a new one is not.
// The bounds of the lanes whose limit lives behind their seam, where this
// package cannot reach it. They are MIRRORS: `compose.slippingScanLimit` and
// `compose.decayCandidateCap` are the real numbers, and
// `backend/gates/worklistbounds_test.go` fails in both directions when either
// side moves, because a mirror nobody checks is a wrong number waiting.
//
// The waiting source held a third mirror and no longer does. A mirror exists to
// be compared against a ROW COUNT, and for that source the row count is the
// wrong evidence: its seam filters after the scan, so the survivors are fewer
// than what was read and a short answer is what truncation looks like. The lane
// reports its own scan depth instead, which is the shape any source that
// filters after it scans needs.
const (
	quietDealBound = 50
	decayBound     = 40
)

func boundedSources(day crmcontracts.Attention) map[crmcontracts.WorklistItemSource]bool {
	bounded := map[crmcontracts.WorklistItemSource]bool{}
	// A lane the read never asked for is absent; a lane it asked and found
	// empty is present and false. That distinction is the difference between
	// "nothing today" and "this source was not read", and a reader cannot
	// recover it from an absence.
	atCap := func(source crmcontracts.WorklistItemSource, lane *[]crmcontracts.AttentionItem, bound int) {
		if lane == nil {
			return
		}
		bounded[source] = len(*lane) >= bound
	}
	// The health and receipt lanes share one bound.
	atCap("failed_approval", day.DidNotRun, doneCap)
	atCap("dsr", day.Dsr, doneCap)
	atCap("ai_work_health", day.AiWorkHealth, doneCap)
	atCap("notice", day.Notices, doneCap)
	atCap("automation_run", day.AutomationHealth, doneCap)
	atCap("bounce", day.Bounces, doneCap)
	atCap("introduction_request", day.Introductions, doneCap)
	// Each of these carries its own, declared where the lane is read.
	atCap("task", &day.Planned, plannedCap)
	atCap(sourceAtRisk, day.AtRisk, quietDealBound)
	atCap("relationship_decay", day.RelationshipDecay, decayBound)
	atCap("conversation_claim", day.Commitments, doneCap)
	// The decision lane is read deeper than the rest, because a batch row
	// counts a pile and a count taken from a page of ten would report ten over
	// a hundred and fifty. Approvals and duplicate pairs share that ONE bound,
	// so filling it says the LANE was truncated and neither source can claim to
	// be complete — the conservative reading, since the alternative is telling a
	// rep there are no more of a kind when there are.
	bounded["approval"] = len(day.NeedsYou) >= batchScanDepth
	bounded["dedupe_candidate"] = bounded["approval"]
	return bounded
}

// unavailable turns the assembled day's withheld lanes into the queue's own
// vocabulary.
//
// The lane feed already names what a caller may not read; this widens the same
// promise to say WHY. A day cannot read as clear while something that would
// have filled it never answered, which is the one lie a worklist must not tell.
func unavailable(day crmcontracts.Attention) []crmcontracts.WorklistSourceUnavailable {
	out := []crmcontracts.WorklistSourceUnavailable{}
	if day.LanesOmitted == nil {
		return out
	}
	for _, lane := range *day.LanesOmitted {
		// The DSR lane is withheld BY ROLE for every reader who is not a
		// privacy admin, permanently and by design. Naming it would put "part
		// of your day is hidden" on every rep's page forever, which drowns the
		// warning this list exists to give.
		//
		// This suppression is WIDER than it should be, and the difference is
		// worth stating rather than hiding: the DSR read also refuses a reader
		// who has the admin role but lost `person:read`, and that refusal is
		// real news this list swallows. Telling the two apart needs a reason on
		// the refusal, which the lane contract does not carry — issue filed.
		if lane == laneDSR {
			continue
		}
		out = append(out, crmcontracts.WorklistSourceUnavailable{
			Source: string(lane),
			Reason: crmcontracts.WorklistSourceUnavailableReasonWithheld,
		})
	}
	return out
}

// laneDSR is the one lane whose withholding is a permanent role fact rather
// than news about this reader's day.
const laneDSR = crmcontracts.AttentionLanesOmitted("dsr")
