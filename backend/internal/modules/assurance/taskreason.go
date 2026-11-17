// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import "github.com/margince/margince/backend/internal/shared/kernel/systemtext"

// TaskReason says what a night's check found on one deal, by its most
// material finding, so the deal's task is never only a count. It names the
// kind of finding and no value, so a cleared finding leaves no stale number.
func TaskReason(findings []Exception) *systemtext.Component {
	if len(findings) == 0 {
		return nil
	}
	code := "assurance.finding." + findings[0].Type
	if !systemtext.Has(code) {
		return nil
	}
	if more := len(findings) - 1; more > 0 {
		return systemtext.Sentence("assurance.why_more", systemtext.MessageArgument(code), systemtext.Count(int64(more)))
	}
	return systemtext.Sentence("assurance.why", systemtext.MessageArgument(code))
}

// TaskSubject is the count a rep reads on their list. The deal is named by the
// task's link, which the list draws beside it; TaskReason says what was found.
func TaskSubject(findings int) *systemtext.Component {
	if findings == 1 {
		return systemtext.Sentence("assurance.inputs_one")
	}
	return systemtext.Sentence("assurance.inputs_many", systemtext.Count(int64(findings)))
}
