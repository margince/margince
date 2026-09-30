// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// A buyer asks "is Gemini good enough?", not "is gemini-3.1-flash-lite?". The
// certification page answers per model family, so this file decides which served
// models are one family.
//
// It is not the self-judge rule. selfJudged treats two identities as one vendor
// when EITHER their publisher OR their line agrees, which is right for "may this
// model mark its own homework" and wrong here: publisher alone would fold
// google/gemma-4 into Gemini, and line alone would split ministral from mistral.
// A family is the line, with the few merges a buyer expects spelled out below.

import "strings"

// familyAliases folds a line into the family a buyer knows it by. Only a merge
// the line alone cannot see belongs here.
var familyAliases = map[string]string{
	"ministral": "mistral",
}

// familyNames spells a family the way a buyer writes it. A line missing from
// here is shown capitalised, which is right for most of them.
var familyNames = map[string]string{
	"gpt": "GPT",
	"glm": "GLM",
}

// OtherFamily names the models whose served identity carries no line at all.
const OtherFamily = "Other"

// ModelFamily names the family of a served model identity: "mistralai/ministral-8b-2512"
// and "mistral-large-2512" are both Mistral, "google/gemma-4-31b-it" is Gemma.
func ModelFamily(servedModel string) string {
	_, line := modelLineage(servedModel)
	if alias, ok := familyAliases[line]; ok {
		line = alias
	}
	if line == "" {
		return OtherFamily
	}
	if name, ok := familyNames[line]; ok {
		return name
	}
	return strings.ToUpper(line[:1]) + line[1:]
}
