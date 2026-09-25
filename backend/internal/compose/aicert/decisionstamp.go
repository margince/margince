// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// The decision stamp: what a decision record says it was scored against. It is
// separate from the LLM stamp (ScenarioStamps) on purpose. A site's decision
// form and its LLM prompt move independently, and folding one into the other
// would stale every completion record the day a decision criterion changed —
// records that still describe the prompt they measured.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/decision"
)

// decisionGradingRule versions how a decision run is graded: the site's own
// gate keeps or drops the answer, and a kept answer is right only when it is
// the fixture's expected label. Bump it when that rule changes, so every
// decision record graded the old way reads stale.
const decisionGradingRule = "decision-rule-1"

// DecisionScenarioStamps is the decision stamp of each scenario whose case has
// a decision form, keyed by scenario name. A scenario whose case has none is
// absent: it has no decision question to stamp.
//
// A stamp is three fixed-width digests, laid out as ScenarioStamps lays its
// own out so splitStampChange reads both: the scenario whole, the decision
// request the site builds from it with the site's floors, and the grading rule.
func DecisionScenarioStamps(scenarios []Scenario, census *aitasks.Registry) (map[string]string, error) {
	if census == nil {
		return nil, fmt.Errorf("aicert: decision stamp: no census supplied — only the census says which case builds a site's decision request")
	}
	stamps := map[string]string{}
	for _, sc := range scenarios {
		dc, ok, err := decisionCaseFor(sc, census)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		encoded, err := json.Marshal(sc)
		if err != nil {
			return nil, fmt.Errorf("aicert: decision stamp: scenario %q cannot be digested: %w", sc.Name, err)
		}
		request, err := canonicalDecisionDigest(dc.DecisionRequest(), dc.Floors())
		if err != nil {
			return nil, fmt.Errorf("aicert: decision stamp: scenario %q: %w", sc.Name, err)
		}
		if _, clash := stamps[sc.Name]; clash {
			return nil, fmt.Errorf("aicert: decision stamp: two scenarios are both named %q — a stamp is keyed by name, so one would go unrecorded", sc.Name)
		}
		sum := sha256.Sum256(encoded)
		stamps[sc.Name] = hex.EncodeToString(sum[:]) + request + gradedBy(decisionGradingRule, "")
	}
	return stamps, nil
}

// CurrentDecisionStamps is DecisionScenarioStamps over a whole corpus, keyed
// "task/variant" then scenario name: the per-site form DecisionReadiness reads.
// Stamped task by task, because a scenario name is unique within its task and
// nothing makes it unique across the corpus.
func CurrentDecisionStamps(corpus []Scenario, census *aitasks.Registry) (map[string]map[string]string, error) {
	byTask := map[string][]Scenario{}
	for _, sc := range corpus {
		byTask[sc.Task] = append(byTask[sc.Task], sc)
	}
	perSite := map[string]map[string]string{}
	for task, scenarios := range byTask {
		stamps, err := DecisionScenarioStamps(scenarios, census)
		if err != nil {
			return nil, fmt.Errorf("task %s: %w", task, err)
		}
		for _, sc := range scenarios {
			stamp, stamped := stamps[sc.Name]
			if !stamped {
				continue
			}
			key := sc.Task + "/" + sc.Site
			if perSite[key] == nil {
				perSite[key] = map[string]string{}
			}
			perSite[key][sc.Name] = stamp
		}
	}
	return perSite, nil
}

// decisionCaseFor prepares sc's case and reports whether it has a decision
// form. A case whose decision site is not the scenario's own site is refused:
// its record would be keyed by one site and measured on another, and the
// certification row would name a site the corpus never ran.
//
//nolint:ireturn // the case is the site's own type, reachable only through the optional interface it implements.
func decisionCaseFor(sc Scenario, census *aitasks.Registry) (aitasks.DecisionCase, bool, error) {
	factory, bound := census.CaseFor(ai.Task(sc.Task), sc.Site)
	if !bound {
		return nil, false, fmt.Errorf("aicert: decision stamp: scenario %q names site %s/%s, which binds no certification case", sc.Name, sc.Task, sc.Site)
	}
	prepared, err := factory.Prepare(json.RawMessage(sc.Fixture), json.RawMessage(sc.Expect.Answer))
	if err != nil {
		return nil, false, fmt.Errorf("aicert: decision stamp: scenario %q: preparing the case: %w", sc.Name, err)
	}
	dc, ok := prepared.(aitasks.DecisionCase)
	if !ok {
		return nil, false, nil
	}
	if dc.DecisionSite() != sc.Site {
		return nil, false, fmt.Errorf("aicert: decision stamp: scenario %q runs on site %s but its case asks its decision at %q",
			sc.Name, sc.Site, dc.DecisionSite())
	}
	return dc, true, nil
}

// labelFloor is one label's floor, as the digest spells it.
type labelFloor struct {
	Label string  `json:"label"`
	Floor float64 `json:"floor"`
}

// canonicalDecisionDigest hashes what the decision model is shown — the state
// and the questions — and the floors the site's gate keeps an answer at. The
// lane's model is a binding, not a question, and is left out, as the LLM
// digest leaves out the served model. Per-call ids are swept for the reason
// canonicalRequestDigest sweeps them.
func canonicalDecisionDigest(req decision.Request, floors map[string]float64) (string, error) {
	ordered := make([]labelFloor, 0, len(floors))
	for label, floor := range floors {
		ordered = append(ordered, labelFloor{Label: label, Floor: floor})
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Label < ordered[j].Label })
	material, err := json.Marshal(struct {
		State     json.RawMessage              `json:"state"`
		Questions map[string]decision.Question `json:"questions"`
		Floors    []labelFloor                 `json:"floors"`
	}{State: req.State, Questions: req.Questions, Floors: ordered})
	if err != nil {
		return "", fmt.Errorf("the decision request cannot be digested: %w", err)
	}
	sum := sha256.Sum256(perCallID.ReplaceAll(material, []byte(canonicalID)))
	return hex.EncodeToString(sum[:]), nil
}
