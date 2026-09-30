// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// Decision readiness: whether each decision record still describes the
// decision question this build asks. It is the same per-scenario judgement a
// completion record gets (scenarioStanding), read against the decision stamps
// rather than the LLM ones, because the two forms of a site move separately.

// DecisionRow is one decision record and what it is still right about.
type DecisionRow struct {
	// Site is "task/variant", as every tree here spells a site.
	Site     string
	Record   Record
	Standing Standing
}

// Status is the row's state in readiness's words. A decision record always
// carries per-scenario stamps, so it is never judged by its task stamp alone.
func (r DecisionRow) Status() string {
	switch {
	case r.Standing.Stale:
		return StatusStale
	case r.Standing.Pending > 0:
		return StatusPartial
	default:
		return StatusCurrent
	}
}

// DecisionReadiness judges every decision record against the decision stamps
// this build computes (CurrentDecisionStamps), keyed "task/variant". Completion
// records are Readiness's and are skipped here.
func DecisionReadiness(decisionStamps map[string]map[string]string, records []Record) []DecisionRow {
	var rows []DecisionRow
	for _, rec := range records {
		if rec.Kind != KindDecision {
			continue
		}
		site := rec.Task + "/" + rec.Site
		current := decisionStamps[site]
		rows = append(rows, DecisionRow{
			Site: site, Record: rec,
			Standing: scenarioStanding(rec, rec.Site, current, FoldScenarioStamps(current)),
		})
	}
	return rows
}
