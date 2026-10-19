// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"strings"
)

func validateExpectedMeaning(optional bool, meaning *commitmentMeaning, due, source string) error {
	if meaning == nil {
		if optional {
			return errors.New("only a non-action with assessed meaning may be omitted")
		}
		return nil
	}
	if reason := validateCommitmentMeaning(meaning, due, source); reason != "" {
		return errors.New(reason)
	}
	if optional && meaning.withheldOutcome() == "" {
		return errors.New("an open commitment must be present in the answer")
	}
	return nil
}

func expectedMeaningMatches(want, got *commitmentMeaning) bool {
	if want == nil {
		return true
	}
	return got != nil && want.State == got.State && want.DeadlinePrecision == got.DeadlinePrecision &&
		expectedMeaningQuote(want.Condition, got.Condition) && expectedMeaningQuote(want.DeadlineQuote, got.DeadlineQuote)
}

// The runtime validates every quoted word against the source; the case names the required evidence within that quote.
func expectedMeaningQuote(want, got string) bool {
	if want == "" {
		return got == ""
	}
	return strings.Contains(" "+strings.Join(strings.Fields(got), " ")+" ", " "+strings.Join(strings.Fields(want), " ")+" ")
}
