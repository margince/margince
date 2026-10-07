// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import "fmt"

// The verdict strings a bench stores on a row. They mirror the DailyVerdict
// constants in internal/compose/integration/daily_budgets_bench_test.go, which
// writes them; loadRecords refuses any other value so a renamed one fails here.
const (
	storedWithin    = "within budget"
	storedOverKnown = "over budget"
	storedOver      = "over budget (unlisted)"
	storedNoData    = "no data"
	storedNotGated  = "not gated"
)

// severity orders verdicts from harmless to worst, so a budget measured by
// several rows reports the one a reader most needs to see.
type severity int

const (
	// sevUngated rows (no data, not gated) answer no budget question and stay
	// out of the summary.
	sevUngated severity = iota
	sevWithin
	sevInconclusive
	sevOverListed
	sevOver
	sevFailed
)

// judgement is a row's verdict in words plus its rank.
type judgement struct {
	text string
	sev  severity
}

// judge reads a stored verdict when the bench wrote one and computes it from
// the numbers otherwise, so a record written before verdicts were stored
// renders as it always did.
func judge(m measurement) judgement {
	if m.Status5xx > 0 {
		return judgement{fmt.Sprintf("**FAILED**: %d server errors", m.Status5xx), sevFailed}
	}
	switch m.Verdict {
	case "":
		return computedJudgement(m)
	case storedNoData, storedNotGated:
		return judgement{m.Verdict, sevUngated}
	case storedOverKnown:
		return judgement{withCaveat(fmt.Sprintf("over budget (#%d)", m.KnownIssue), m.Caveat), sevOverListed}
	case storedOver:
		return judgement{withCaveat("**OVER BUDGET**", m.Caveat), sevOver}
	}
	// A stored pass under a caveat proves nothing, for the reason computedJudgement gives.
	if m.Caveat != "" {
		return judgement{"inconclusive: " + m.Caveat, sevInconclusive}
	}
	return judgement{storedWithin, sevWithin}
}

// computedJudgement is the verdict for a row that stores none.
//
// `>=`, not `>`: every published budget is a strict bound ("< 100 ms"), so a
// p95 landing on the number does not satisfy it. search.BenchReport.Gate() and
// the mobile spec's toBeLessThan read the bound the same way.
//
// A caveat suppresses a pass and never a breach. A caveat says the run did not
// meet the condition the budget binds under, today a smaller tier than the
// budget names. A smaller tier is easier in every seeded dimension, so a breach
// on it is real evidence while a pass proves nothing. If a caveat ever describes
// a harder condition than the real one, this asymmetry has to be revisited.
func computedJudgement(m measurement) judgement {
	if m.P95Ms >= m.BudgetMs {
		return judgement{withCaveat("**OVER BUDGET**", m.Caveat), sevOver}
	}
	if m.Caveat != "" {
		return judgement{"inconclusive: " + m.Caveat, sevInconclusive}
	}
	return judgement{storedWithin, sevWithin}
}

func withCaveat(text, caveat string) string {
	if caveat == "" {
		return text
	}
	return text + ": " + caveat
}

// verdict is the budgets page's wording for one row.
func verdict(m measurement) string { return judge(m).text }

// worse reports whether a ranks above b: by severity, then by the slower p95,
// because among equally bad rows the slowest is the one worth printing.
func worse(a, b measurement) bool {
	sa, sb := judge(a).sev, judge(b).sev
	if sa != sb {
		return sa > sb
	}
	return a.P95Ms > b.P95Ms
}

// validStoredVerdict guards the wire between the bench and this renderer: an
// unknown string would otherwise fall through to "within budget".
func validStoredVerdict(m measurement) error {
	switch m.Verdict {
	case "", storedWithin, storedOver, storedNoData, storedNotGated:
		return nil
	case storedOverKnown:
		if m.KnownIssue <= 0 {
			return fmt.Errorf("row %q says %q without a known_issue", m.Name, m.Verdict)
		}
		return nil
	}
	return fmt.Errorf("row %q has an unknown verdict %q", m.Name, m.Verdict)
}
