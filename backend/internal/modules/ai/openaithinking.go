// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// A thinking floor on the OpenAI Responses wire, as `reasoning.effort`. Only a
// reasoning model takes the field, and a non-reasoning one answers it with a
// 400, so an id this table does not know is sent nothing.

import "strings"

// openaiEffortDefaults is each reasoning family's default effort, from the
// vendor's model pages: 5.1, 5.2 and 5.4 default to none; gpt-5, 5.5, 5.6, 6
// and the o-series to medium. Longer prefixes come first so they win.
var openaiEffortDefaults = []struct{ prefix, effort string }{
	{"gpt-5-chat", ""},
	{"gpt-5.1", effortNone},
	{"gpt-5.2", effortNone},
	{"gpt-5.4", effortNone},
	{"gpt-5.5", effortMedium},
	{"gpt-5.6", effortMedium},
	{"gpt-5-", effortMedium},
	{"gpt-6", effortMedium},
	{"o1", effortMedium},
	{"o3", effortMedium},
	{"o4", effortMedium},
}

// openaiDefaultEffort is modelID's default effort, and whether it reasons.
func openaiDefaultEffort(modelID string) (string, bool) {
	if modelID == "gpt-5" {
		return effortMedium, true
	}
	for _, family := range openaiEffortDefaults {
		if strings.HasPrefix(modelID, family.prefix) {
			return family.effort, family.effort != ""
		}
	}
	return "", false
}

// openaiEffortFor is the effort a floor sends, empty where the model's default
// already meets it. A none-default family does not take minimal, so a minimal
// floor asks it for low.
func openaiEffortFor(modelID, floor string) string {
	def, reasons := openaiDefaultEffort(modelID)
	if floor == "" || !reasons || effortAtLeast(def, floor) {
		return ""
	}
	if def == effortNone && floor == effortMinimal {
		return effortLow
	}
	return floor
}
