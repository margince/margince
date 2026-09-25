// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// The verdict rule's thresholds, reached by the certification page so its
// plain-words grading summary quotes the rule rather than restating it.
const (
	CertifiedPassPercent      = certifiedPassPercent
	CertifiedPassBoundPercent = certifiedPassBoundPercent
	CasePassPercent           = casePassPercent
	VetoPassPercent           = vetoPassPercent
	ConfidenceZ               = confidenceZ
)

// DefaultRepeats is each case's first round unless told otherwise, and
// AdaptiveRound/AdaptiveMaxRuns how a borderline case is extended.
const (
	DefaultRepeats  = defaultRepeats
	AdaptiveRound   = adaptiveRound
	AdaptiveMaxRuns = adaptiveMaxRuns
)

// JudgeOpinions is how many times every run is graded.
const JudgeOpinions = judgeOpinions

// CaseFallsShort says a case passed fewer than casePassPercent of its runs;
// the page counts the cases that do.
func CaseFallsShort(passed, runs int) bool { return passed*100 < casePassPercent*runs }

// MajorityNumerator and MajorityDenominator are the degraded pooled majority as
// a fraction, which the page says in words.
const (
	MajorityNumerator   = majorityNumerator
	MajorityDenominator = majorityDenominator
)
