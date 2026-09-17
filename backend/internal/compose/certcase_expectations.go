// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a scenario's expected answer means, in one place.
//
// Four cases — the company brief, the dossier, the account scan and the corpus
// ask — asked the same question of a reply: did it cite the records this
// scenario says a correct answer rests on. Each spelled the loop itself, so the
// question had four answers that happened to agree, which is three more than it
// needs.
//
// Deliberately NOT a place where a scenario can name alternatives. That was
// written and removed: account_scan's scan_finds_the_promise_we_did_not_keep
// looked like it wanted "either the promise or the nudge", and its own comment
// says the author chose the nudge on purpose, because the nudge is what needs
// an answer today and the site asks for the finding that most needs a contact
// FIRST. Accepting either would have deleted the prioritisation that scenario
// exists to test. Its rubric was the thing out of step, and the rubric moved.
// uncitedExpectations answers which of a scenario's expected records the reply
// failed to cite.
func uncitedExpectations(expected []string, label map[string]string, cited map[string]bool) []string {
	var missing []string
	for _, name := range expected {
		if !cited[label[name]] {
			missing = append(missing, name)
		}
	}
	return missing
}
