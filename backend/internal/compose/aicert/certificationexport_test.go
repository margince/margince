// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import "github.com/margince/margince/backend/internal/modules/ai"

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

// ReaskBandMargin, ReaskDisagreement and MaxJudgeOpinions are when a run is
// graded again, and JudgeScoreSDFloor the least spread an average score's bound
// assumes.
const (
	ReaskBandMargin   = reaskBandMargin
	ReaskDisagreement = reaskDisagreement
	MaxJudgeOpinions  = maxJudgeOpinions
	JudgeScoreSDFloor = judgeScoreSDFloor
)

// CaseFallsShort says a case passed fewer than casePassPercent of its runs;
// the page counts the cases that do.
func CaseFallsShort(passed, runs int) bool { return passed*100 < casePassPercent*runs }

// MajorityNumerator and MajorityDenominator are the degraded pooled majority as
// a fraction, which the page says in words.
const (
	MajorityNumerator   = majorityNumerator
	MajorityDenominator = majorityDenominator
)

// BoundRung and RungsBound are the rungs a routed run certifies a task on, which
// the page reads to say which model answers a feature and which one it falls to.
type BoundRung = boundRung

func RungsBound(routing ai.RoutingConfig, task ai.Task) []BoundRung {
	return boundLadder(routing, task)
}
