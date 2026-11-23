// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
)

// CertKey is everything a completion record must share with a binding to grade
// it: the task, the binding as served, and its environment class. The lane and
// the runtime both build it here, so the grade each shows reads one rule.
type CertKey struct {
	Task          Task
	Provider      string
	Model         string
	Profile       string
	ThinkingLevel string
	// SiteThinking and Upstream spell the site floors and the broker preferences
	// as strings, so the key stays comparable; "" when there are none.
	SiteThinking string
	Upstream     string
}

// NewCertKey is a key from a record's own fields.
func NewCertKey(task Task, provider, model, profile, thinking string, siteThinking map[string]string, upstream *OpenRouterRouting) CertKey {
	return CertKey{
		Task: task, Provider: provider, Model: model, Profile: profile, ThinkingLevel: thinking,
		SiteThinking: siteThinkingKey(siteThinking), Upstream: upstreamKey(upstream),
	}
}

// CertKeyFor is the key a binding answers task with under profile: the floors
// its sites ask and the broker preferences it is served under included.
func CertKeyFor(task Task, binding ProviderConfig, profile Profile) CertKey {
	return NewCertKey(task, binding.Provider, binding.Model, string(profile), binding.ThinkingLevel,
		SiteThinkingLevels(binding, task), UpstreamPreferencesFor(binding))
}

// MeasuredAs is the binding and profile a record names when it grades binding
// on another provider's measurement (MeasuredBy). Vertex serves Gemini's
// weights on Gemini's wire, so it is graded by the AI Studio record, which is
// filed under cloud_frontier.
func MeasuredAs(binding ProviderConfig, profile Profile) (ProviderConfig, Profile) {
	by := MeasuredBy(binding.Provider)
	if by == binding.Provider {
		return binding, profile
	}
	binding.Provider, binding.Location = by, ""
	return binding, ProfileCloudFrontier
}

// The grades a task can show for the model that answers it.
const (
	CertGradeReady        = "ready"
	CertGradeWithCare     = "usable_with_care"
	CertGradeNotReliable  = "not_reliable"
	CertGradeNotMeasured  = "not_measured"
	certStateCurrent      = "current"
	certStateAbsent       = "absent"
	certVerdictCertified  = "certified"
	certVerdictDegraded   = "supported_degraded"
	certVerdictNotSupport = "not_supported"
)

// CertGrade is one record's standing on its task by the readiness report's
// rule. It holds the worst verdict and state over every site the task ships.
type CertGrade struct {
	Verdict string
	State   string
}

// Certification is what Settings shows beside a task: its grade on the model
// that answers it, and whether that grade waits on a re-check.
type Certification struct {
	Grade          string
	RecheckPending bool
}

// CertificationFor grades binding on task from the table the committed records
// generate (taskgrades_gen.go): its own record, else the provider's it is
// measured by. A binding no record measures, or one a shipped site of the task
// has no record for, is not measured.
func CertificationFor(task Task, binding ProviderConfig, profile Profile) Certification {
	grade, found := taskGrades[CertKeyFor(task, binding, profile)]
	if !found {
		as, asProfile := MeasuredAs(binding, profile)
		if as.Provider != binding.Provider {
			grade, found = taskGrades[CertKeyFor(task, as, asProfile)]
		}
	}
	if !found || grade.State == certStateAbsent {
		return Certification{Grade: CertGradeNotMeasured}
	}
	return Certification{Grade: certGradeOf(grade.Verdict), RecheckPending: grade.State != certStateCurrent}
}

func certGradeOf(verdict string) string {
	switch verdict {
	case certVerdictCertified:
		return CertGradeReady
	case certVerdictDegraded:
		return CertGradeWithCare
	case certVerdictNotSupport:
		return CertGradeNotReliable
	default:
		// A verdict this build does not know grades nothing, so it cannot read
		// better than the worst grade a measurement gives.
		return CertGradeNotReliable
	}
}

// siteThinkingKey spells the floors site by site in name order.
func siteThinkingKey(levels map[string]string) string {
	parts := make([]string, 0, len(levels))
	for _, site := range slices.Sorted(maps.Keys(levels)) {
		parts = append(parts, site+"="+levels[site])
	}
	return strings.Join(parts, ",")
}

// upstreamKey is the preferences' JSON, which keeps struct field order. A value
// that cannot be encoded spells the reason instead: it matches no record, so
// the binding reads not measured rather than borrowing another's grade.
func upstreamKey(upstream *OpenRouterRouting) string {
	if upstream == nil {
		return ""
	}
	raw, err := json.Marshal(upstream)
	if err != nil {
		return "unencodable: " + err.Error()
	}
	return string(raw)
}
