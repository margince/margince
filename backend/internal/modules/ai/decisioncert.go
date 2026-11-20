// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"log/slog"
	"strings"
)

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

// UncertifiedDecisionSites names each decision site, as task/site, that the
// bound lane's provider and model hold no certified row for in the generated
// table (decisioncert_gen.go). The lane answers those sites all the same: a row
// is the operator's evidence for the binding, never a gate on it.
func (cfg RoutingConfig) UncertifiedDecisionSites() []string {
	lane := cfg.Decisions
	if lane == nil {
		return nil
	}
	var sites []string
	for _, task := range DecisionTasks() {
		for _, site := range SitesFor(task) {
			key := DecisionCertKey{Task: task, Site: site.Name, Provider: lane.Provider, Model: lane.Model}
			if _, certified := decisionCertified[key]; !certified {
				sites = append(sites, string(task)+"/"+site.Name)
			}
		}
	}
	return sites
}

// LogUncertifiedDecisionSites warns, at boot and at save, where the lane
// answers with no certified record behind it.
func LogUncertifiedDecisionSites(log *slog.Logger, cfg RoutingConfig) {
	sites := cfg.UncertifiedDecisionSites()
	if len(sites) == 0 {
		return
	}
	log.Warn("ai: routing: the decisions lane is bound to a model with no current certified record for these sites; "+
		"it answers them all the same",
		"provider", cfg.Decisions.Provider, "model", cfg.Decisions.Model, "sites", strings.Join(sites, ","))
}
