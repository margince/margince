// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
)

// Every verdict this site will ACCEPT is a verdict its prompt TEACHES.
//
// `unsure` was accepted in four places and taught in none: the response
// schema's enum offered the token, the reply validator admitted it, the
// database CHECK stores it, and a certification scenario is built entirely
// around expecting it — while the system prompt defined "settled" and
// "still_owed" and never mentioned it at all.
//
// A model cannot answer a question it was not asked. Worse, the prompt pushed
// the other way: still_owed is defined as "did not address what they asked at
// all", which is exactly what a bare "Ok." looks like, so the one reading the
// prompt supports is the one the scenario marks wrong. Three of Gemma 4's nine
// failures on this task were that, and no model could have done better from
// this prompt.
//
// A gate rather than a fixed sentence, because the next verdict added to the
// enum will be added for the same reason and forgotten in the same place.
func TestEveryVerdictTheSiteAcceptsIsDefinedInItsPrompt(t *testing.T) {
	t.Parallel()
	prompt := settleSystem
	for verdict := range settleVerdicts {
		// Quoted, as the prompt names the other two: a verdict mentioned in
		// passing is not a verdict defined, and the quoted form is how this
		// prompt introduces one.
		if !strings.Contains(prompt, `"`+verdict+`"`) {
			t.Errorf("the reply validator accepts verdict %q and the prompt never defines it, so a "+
				"model is being asked for a token it was never taught", verdict)
		}
	}
}

// And the three the enum offers are exactly the three the validator admits.
//
// Held separately because the enum and the map are two lists of one thing: a
// verdict added to the schema and not the map reaches the model, comes back,
// and is refused as an unknown word after the call is paid for.
func TestTheSchemasVerdictsAreTheOnesTheValidatorAdmits(t *testing.T) {
	t.Parallel()
	declared := string(settleSchema())
	for verdict := range settleVerdicts {
		if !strings.Contains(declared, `"`+verdict+`"`) {
			t.Errorf("the validator admits %q and the response schema does not offer it", verdict)
		}
	}
	for _, verdict := range []string{activities.RequestSettled, activities.RequestStillOwed, activities.RequestUnsure} {
		if !settleVerdicts[verdict] {
			t.Errorf("the schema offers %q and the validator refuses it", verdict)
		}
	}
}
