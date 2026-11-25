// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package roombrief

import "regexp"

// refusal is a buyer saying they will not go ahead: "will not sign", "can't
// approve", "not going to proceed".
const refusal = `(?:will not|won't|cannot|can't|can not|unable to|not able to|not going to|do not|don't)\s+` +
	`(?:\w+\s+){0,2}?(?:sign|countersign|approve|proceed|go ahead|commit|move forward)`

// conditionOnTerm is the "until", "unless" or "without" that names the term.
const conditionOnTerm = `\b(?:until|unless|without)\b`

// conditionPhrases match a buyer setting a condition for signing. One is a
// refusal followed by what it waits on. The other is a need stated before the
// step it blocks.
var conditionPhrases = []*regexp.Regexp{
	regexp.MustCompile(`(?is)` + refusal + `.*` + conditionOnTerm),
	regexp.MustCompile(`(?is)\b(?:need|needs|require|requires)\b.*\bbefore\s+(?:we|they|i|legal|our \w+)\s+(?:can\s+)?(?:sign|approve|proceed|go ahead)`),
}

// setsCondition says body states a condition the buyer holds back their
// signature on. It marks a thread for the model to read first, and it decides
// nothing: a missed phrasing leaves the thread where it was.
func setsCondition(body string) bool {
	for _, phrase := range conditionPhrases {
		if phrase.MatchString(body) {
			return true
		}
	}
	return false
}
