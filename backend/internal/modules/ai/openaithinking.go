// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// A thinking floor on the OpenAI Responses wire, as `reasoning.effort`. Only a
// reasoning model takes the field, and a non-reasoning one answers it with a
// 400, so an id this table does not know is sent nothing.

import "strings"

// openaiEffortDefaults is each reasoning family's default effort, from the
// vendor's model pages: 5.1, 5.2 and 5.4 default to none; gpt-5, 5.5, 5.6, 6
// and the o-series to medium, except o1-mini and o1-preview, which answer the
// field with a 400. Longer prefixes come first so they win. least is the
// shallowest level a family takes besides its default: the none-default
// families and GPT-6 answer minimal with a 400, and GPT-6 answers none too.
var openaiEffortDefaults = []struct{ prefix, effort, least string }{
	{"gpt-5-chat", "", ""},
	{"o1-mini", "", ""},
	{"o1-preview", "", ""},
	{"gpt-5.1", effortNone, effortLow},
	{"gpt-5.2", effortNone, effortLow},
	{"gpt-5.4", effortNone, effortLow},
	{"gpt-5.5", effortMedium, ""},
	{"gpt-5.6", effortMedium, ""},
	{"gpt-5-", effortMedium, ""},
	{"gpt-6", effortMedium, effortLow},
	{"o1", effortMedium, ""},
	{"o3", effortMedium, ""},
	{"o4", effortMedium, ""},
}

// openaiDefaultEffort is modelID's default effort, and whether it reasons.
func openaiDefaultEffort(modelID string) (string, bool) {
	def, _ := openaiFamilyEfforts(modelID)
	return def, def != ""
}

// openaiFamilyEfforts is modelID's default effort and the shallowest other
// level it takes, both empty for a model that does not reason.
func openaiFamilyEfforts(modelID string) (def, least string) {
	if modelID == "gpt-5" {
		return effortMedium, ""
	}
	for _, family := range openaiEffortDefaults {
		if strings.HasPrefix(modelID, family.prefix) {
			return family.effort, family.least
		}
	}
	return "", ""
}

// openaiTakenEffort is level, raised to the shallowest effort modelID takes
// when the model would answer level with a 400.
func openaiTakenEffort(modelID, level string) string {
	def, least := openaiFamilyEfforts(modelID)
	if least != "" && level != def && !effortAtLeast(level, least) {
		return least
	}
	return level
}

// openaiEffortFor is the effort a floor sends, empty where the model's default
// already meets it.
func openaiEffortFor(modelID, floor string) string {
	def, reasons := openaiDefaultEffort(modelID)
	if floor == "" || !reasons || effortAtLeast(def, floor) {
		return ""
	}
	return openaiTakenEffort(modelID, floor)
}
