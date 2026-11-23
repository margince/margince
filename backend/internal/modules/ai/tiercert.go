// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"encoding/json"
	"slices"
	"strings"
)

// What the certification records say about a model a tier binding serves, per
// task. The table (tiercert_gen.go) is generated from the committed records.
// The AI admin screen reads it through TierCertificationFor and prints the
// verdict, so no failure text is typed by hand.

// The verdicts a record carries, spelled as in the records.
const (
	CertVerdictCertified         = "certified"
	CertVerdictSupportedDegraded = "supported_degraded"
	CertVerdictNotSupported      = "not_supported"
)

// TierCertKey is what a record measures a task on: the provider, the model it
// served, the location profile, and the settings that change the request. A
// record of one thinking level does not grade a binding at another, so the key
// carries the same settings the certification check compares.
type TierCertKey struct {
	Task     Task
	Provider string
	Model    string
	Profile  Profile
	Thinking string
	Floors   string
	Upstream string
}

// NewTierCertKey spells a key from the parts a record or a binding carries.
// The table's generator and the runtime lookup both build keys here, so the
// two cannot spell one measurement two ways.
func NewTierCertKey(task Task, provider, model string, profile Profile, thinking string, floors map[string]string, upstream *OpenRouterRouting) TierCertKey {
	names := make([]string, 0, len(floors))
	for site := range floors {
		names = append(names, site)
	}
	slices.Sort(names)
	var spelled strings.Builder
	for _, site := range names {
		spelled.WriteString(site + "=" + floors[site] + ";")
	}
	var routing string
	if upstream != nil {
		if raw, err := json.Marshal(upstream); err == nil {
			routing = string(raw)
		}
	}
	return TierCertKey{
		Task: task, Provider: provider, Model: model, Profile: profile,
		Thinking: thinking, Floors: spelled.String(), Upstream: routing,
	}
}

func tierCertKeyFor(task Task, binding ProviderConfig, profile Profile) TierCertKey {
	return NewTierCertKey(task, binding.Provider, binding.Model, profile, binding.ThinkingLevel,
		SiteThinkingLevels(binding, task), UpstreamPreferencesFor(binding))
}

// TierCert is a task's verdict for one model. It is the worst across the task's
// sites and across the records of that model. Current says whether every one of
// them still describes the prompts this build sends.
type TierCert struct {
	Verdict string
	Current bool
}

// TierCertificationFor is the verdict of the record that measured this
// binding, and false when none does. Another provider's records can grade a
// binding: Vertex is graded by the AI Studio record for the same model. That
// binding is read through the other provider, under the profile those records
// are filed under.
func TierCertificationFor(task Task, binding ProviderConfig, profile Profile) (TierCert, bool) {
	if cert, ok := tierCertified[tierCertKeyFor(task, binding, profile)]; ok {
		return cert, true
	}
	if by := MeasuredBy(binding.Provider); by != binding.Provider {
		binding.Provider, binding.Location = by, ""
		cert, ok := tierCertified[tierCertKeyFor(task, binding, ProfileCloudFrontier)]
		return cert, ok
	}
	return TierCert{}, false
}
