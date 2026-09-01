// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The buying-role reading's certification case.
//
// What this site must get right is not prose but RESTRAINT: the expensive
// mistake is a confident role read out of a job title, and the answer that
// earns a pass on such a scenario is no proposal at all. So the expected value
// is the set of roles that should survive, and an empty set is a real
// expectation rather than a missing one — the "Managing Director who never
// wrote anything" case is scored on producing nothing.
//
// Run calls proposeroles.Request and Evaluate calls proposeroles.Gate. A case
// that rebuilt either would measure a copy, and a copy stays green through the
// change that breaks the original.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/proposeroles"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// proposeRolesSite is the site name, as ai-tasks.yaml declares it.
const proposeRolesSite = "propose_roles/committee"

// proposeRolesFixture is one deal's contacts as this site is handed them.
type proposeRolesFixture struct {
	Deal       string                   `json:"deal"`
	Candidates []proposeroles.Candidate `json:"candidates"`
}

// proposeRolesCases serves the committee reading.
type proposeRolesCases struct{}

func (proposeRolesCases) Site() aitasks.Site {
	return aitasks.Site{
		Task:    ai.TaskProposeRoles,
		Variant: "committee",
		Kind:    ai.SiteKindOneShot,
	}
}

// Prepare reads the fixture as the input this site takes, and refuses a
// scenario that cannot be scored.
//
// The refusals are the ones that would otherwise certify a call the product
// never makes: a candidate with no messages is never assembled (the input
// builder drops them, because a name and a title with no words under them is
// the title-only reading the contract forbids), and a fixture whose expected
// roles name somebody outside the candidate set states an outcome the gate
// would refuse whatever the model said.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (proposeRolesCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var in proposeRolesFixture
	if err := json.Unmarshal(fixture, &in); err != nil {
		return nil, fmt.Errorf("%s: the fixture is not the shape this site takes: %w", proposeRolesSite, err)
	}
	if in.Deal == "" {
		return nil, fmt.Errorf(
			"%s: the fixture names no deal, and the deal is what a role is a role ON — "+
				"a reading with no deal is asking which committee, of none", proposeRolesSite)
	}
	if len(in.Candidates) == 0 {
		return nil, fmt.Errorf(
			"%s: the fixture offers no candidates, so there is nobody a role could be read for",
			proposeRolesSite)
	}
	known := map[string]bool{}
	for _, candidate := range in.Candidates {
		if len(candidate.Messages) == 0 {
			return nil, fmt.Errorf(
				"%s: candidate %q carries no messages, and the input builder never assembles such a "+
					"candidate — their own words are the only evidence this site may read, so a "+
					"fixture supplying a bare name and title certifies a call the product never makes",
				proposeRolesSite, candidate.PersonID)
		}
		known[candidate.PersonID] = true
	}
	// A map of person id to the role that person should end up holding. Empty
	// is the correct expectation for a restraint scenario.
	var want map[string]string
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf(
			"%s: the expected value is not a person-to-role map: %w", proposeRolesSite, err)
	}
	for personID := range want {
		if !known[personID] {
			return nil, fmt.Errorf(
				"%s: the scenario expects a role for %q, who is not a candidate — the gate refuses a "+
					"proposal for anybody this call did not offer, so no model answer could satisfy it",
				proposeRolesSite, personID)
		}
	}
	return &proposeRolesCase{in: in, expected: want}, nil
}

// proposeRolesCase is one committee reading ready to be answered.
type proposeRolesCase struct {
	in       proposeRolesFixture
	expected map[string]string
}

// Run issues the one request this site sends, through the production builder.
func (c *proposeRolesCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := proposeroles.Request(c.in.Deal, c.in.Candidates)
	trace := aitasks.Trace{Requests: []model.Request{req}}
	res, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("%s: %w", proposeRolesSite, err)
	}
	trace.Output = res.Text
	return trace, nil
}

// Evaluate runs the production gate and asks whether what survived is what the
// scenario says the evidence supports.
//
// A reply the gate empties is NOT invalid — refusing weak evidence is the
// behaviour this site is certified for. It is wrong only when the scenario
// expected a role to survive, or when a role survived that the scenario says
// the evidence does not support.
func (c *proposeRolesCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	proposals, err := proposeroles.Parse(trace.Output)
	if err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	got := map[string]string{}
	for _, kept := range proposeroles.Gate(proposals, c.in.Candidates) {
		got[kept.PersonID] = kept.Role
	}
	if detail := disagreement(c.expected, got); detail != "" {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: detail}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}

// disagreement names every way the surviving roles differ from the expected
// ones, in one message so a reader fixes them together.
func disagreement(want, got map[string]string) string {
	var faults []string
	for person, role := range want {
		switch actual, ok := got[person]; {
		case !ok:
			faults = append(faults, fmt.Sprintf("%s: no role survived, wanted %s", person, role))
		case actual != role:
			faults = append(faults, fmt.Sprintf("%s: read %s, wanted %s", person, actual, role))
		}
	}
	for person, role := range got {
		if _, ok := want[person]; !ok {
			faults = append(faults, fmt.Sprintf("%s: read %s, wanted none", person, role))
		}
	}
	sort.Strings(faults)
	return strings.Join(faults, "; ")
}
