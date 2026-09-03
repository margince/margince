// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

// What a run entitles a reader to conclude.
//
// The rule that matters is the one about coverage: readiness NEVER says Ready
// when a required source could not be read. A manager told "Ready" by a pass
// that could not open the mailbox has been told the pipeline is sound when what
// happened is that nobody looked — and that is a worse answer than any number
// of exceptions, because it cannot be acted on.
//
// So `checks_incomplete` outranks every finding-based verdict. It is not a
// worse `needs_review`: one says the pipeline has problems, the other says we
// could not tell.

// requiredSources are the sources a run must reach before it may pronounce on
// the pipeline at all.
//
// Mail and offers, and only those two. A close date is confirmed in
// correspondence and an amount is checked against what was sent, so a pass
// missing either is guessing about the two commonest findings. The rest —
// calendar, documents, contracts, incumbent — sharpen a finding without being
// what it rests on, and requiring them would make readiness depend on
// connectors most installations do not have.
var requiredSources = []string{"mail", "offers"}

// Readiness answers what a run's own result entitles a reader to conclude.
func Readiness(coverage []SourceCoverage, findings []Finding, cfg Config) string {
	if !reachedEveryRequiredSource(coverage) {
		return ReadinessChecksIncomplete
	}
	material := 0
	for _, f := range findings {
		if f.Severity == SeverityHigh || isMaterial(f, cfg) {
			material++
		}
	}
	switch {
	case material > 0:
		return ReadinessNeedsReview
	case len(findings) > 0:
		return ReadinessReadyWithExceptions
	default:
		return ReadinessReady
	}
}

// reachedEveryRequiredSource answers whether the run read what it needs to.
//
// A source ABSENT from the coverage list counts as not reached. A run that
// forgot to record a source looks identical to one that could not read it, and
// the safe reading of both is that we do not know — silence is not coverage.
func reachedEveryRequiredSource(coverage []SourceCoverage) bool {
	state := make(map[string]string, len(coverage))
	for _, c := range coverage {
		state[c.Source] = c.State
	}
	for _, source := range requiredSources {
		if state[source] != CoverageChecked {
			return false
		}
	}
	return true
}

// isMaterial answers whether a finding is worth interrupting somebody for.
//
// Materiality changes SEVERITY and urgency, never coverage. A finding below the
// threshold is still recorded and still counted — what it does not do is turn a
// run into one that needs review.
func isMaterial(f Finding, cfg Config) bool {
	if f.AffectedMinor == nil {
		return false
	}
	amount := *f.AffectedMinor
	if amount < 0 {
		amount = -amount
	}
	return amount >= cfg.MaterialMinor
}
