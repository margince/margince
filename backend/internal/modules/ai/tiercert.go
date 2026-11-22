// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

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
// served and the location profile it was bound under.
type TierCertKey struct {
	Task     Task
	Provider string
	Model    string
	Profile  Profile
}

// TierCert is a task's verdict for one model. It is the worst across the task's
// sites and across the records of that model. Current says whether every one of
// them still describes the prompts this build sends.
type TierCert struct {
	Verdict string
	Current bool
}

// TierCertificationFor is the verdict of the records for a binding, and false
// when none exists. Another provider's records can grade a binding: Vertex is
// graded by the AI Studio record for the same model. That binding is read
// through the other provider, under the profile those records are filed under.
func TierCertificationFor(task Task, binding ProviderConfig, profile Profile) (TierCert, bool) {
	if cert, ok := tierCertified[TierCertKey{task, binding.Provider, binding.Model, profile}]; ok {
		return cert, true
	}
	if by := MeasuredBy(binding.Provider); by != binding.Provider {
		cert, ok := tierCertified[TierCertKey{task, by, binding.Model, ProfileCloudFrontier}]
		return cert, ok
	}
	return TierCert{}, false
}
