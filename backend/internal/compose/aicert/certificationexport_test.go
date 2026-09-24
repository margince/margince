// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// The verdict rule's thresholds, reached by the certification page so its
// plain-words grading summary quotes the rule rather than restating it.
const CertifiedPassPercent = certifiedPassPercent

// CaseMajority is the majority a scenario's own runs must reach; the page counts
// the cases that miss it.
func CaseMajority(n int) int { return twoThirds(n) }
