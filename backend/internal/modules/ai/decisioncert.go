// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// DecisionCertKey is what a decision binding is certified FOR: one site of one
// task, on one provider and model. The runtime can see exactly these four, so
// the table keys on nothing else.
type DecisionCertKey struct {
	Task     Task
	Site     string
	Provider string
	Model    string
}

// DecisionCert is the stamp a certified row was recorded under: the decision
// scenario stamp and the corpus it was measured on.
type DecisionCert struct {
	PromptVersion string
	CorpusVersion string
}

// decisionIsCertified reads the generated table: a row exists only for a
// certified decision record whose scenario stamps match this build
// (decisioncert_gen.go).
func decisionIsCertified(key DecisionCertKey) bool {
	_, ok := decisionCertified[key]
	return ok
}
