// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailsubject

// TopicsDifferSQL is the SQL test for a reply that starts its own conversation.
// The rule lives in the mail_topics_differ database function. Capture and the
// send path both call it through this spelling, so they cannot split mail apart
// by two rules. Both arguments are SQL expressions written by the caller.
func TopicsDifferSQL(parentSubject, replySubject string) string {
	return "mail_topics_differ(" + parentSubject + ", " + replySubject + ")"
}
