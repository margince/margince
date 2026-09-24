// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// The verdict rule's thresholds, reached by the certification page so its
// plain-words grading summary quotes the rule rather than restating it.
const CertifiedPassPercent = certifiedPassPercent

// DefaultRepeats is how many times a run tries each case unless told otherwise.
const DefaultRepeats = defaultRepeats

// RejudgeOpinions is how many more opinions a low judge score is weighed against.
const RejudgeOpinions = rejudgeOpinions

// CaseMajority is the majority a scenario's own runs must reach; the page counts
// the cases that miss it.
func CaseMajority(n int) int { return majorityOf(n) }

// MajorityNumerator and MajorityDenominator are the majority as a fraction, which
// the page says in words.
const (
	MajorityNumerator   = majorityNumerator
	MajorityDenominator = majorityDenominator
)
