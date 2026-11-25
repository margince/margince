// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import "strconv"

// pooledJudgeScores is the median and minimum of what a judge graded, as the
// record holds them. Both are nil when no run was graded, so a task that no
// judge scored never reads as one a judge failed.
func pooledJudgeScores(rs []RunResult) (median, minimum *int) {
	m, lowest, graded := judgeMedianAndMin(rs)
	if !graded {
		return nil, nil
	}
	return &m, &lowest
}

// JudgeScoreCell is a pooled judge score as a page or a log prints it.
func JudgeScoreCell(score *int) string {
	if score == nil {
		return "not judged"
	}
	return strconv.Itoa(*score)
}
