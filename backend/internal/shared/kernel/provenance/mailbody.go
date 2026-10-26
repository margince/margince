// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package provenance

// MailBodyCompleteSQL is the one test automatic work reads a mail body by.
// Promises, requests, settlements and conversation events are read only from
// text the parser decoded in full. Any other body is shown, never judged. An
// unassessed body waits for the data task that checks it against its original.
// A body that is not email was never parsed. alias is a compile-time alias.
//
// Held by: TestAutomaticWorkReadsOnlyMailReadInFull (backend/gates/mailbodycompleteness_test.go)
func MailBodyCompleteSQL(alias string) string {
	return "(" + alias + ".kind <> 'email' OR " + alias + ".body_completeness = 'complete')"
}
