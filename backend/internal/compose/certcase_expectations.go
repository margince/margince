// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a scenario's expected answer means, in one place.
//
// Four cases — the company brief, the dossier, the account scan and the corpus
// ask — ask the same question of a reply: did it cite the records this scenario
// says a correct answer rests on. Each spelled the loop itself, so the question
// had four answers that happened to agree.

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
