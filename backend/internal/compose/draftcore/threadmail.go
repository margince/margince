// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcore

import (
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// Exchange is a recent activity as a draft surface reads it.
type Exchange interface {
	ActivityKind() string
	ActivitySubject() string
}

// ThreadMail is the newest email in recent, newest first, when it has a
// subject. A task or a logged call is not a message in the thread, and its
// title is internal. One recorded after their mail still leaves that mail as
// the thread a draft answers.
func ThreadMail[A Exchange](recent []A) (A, bool) {
	for _, act := range recent {
		if act.ActivityKind() == string(crmcontracts.ActivityKindEmail) {
			return act, act.ActivitySubject() != ""
		}
	}
	var none A
	return none, false
}

// SubjectForDraft omits a project tag alone because it supplies no topic.
func SubjectForDraft(subject, projectKey string) string {
	subject = strings.TrimSpace(subject)
	if projectKey != "" && strings.EqualFold(subject, "["+projectKey+"]") {
		return ""
	}
	return subject
}
