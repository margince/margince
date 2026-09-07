// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for stage_evidence_extract/criteria.
//
// It certifies the shipped path: the request comes from stageEvidenceRequest
// and the reply is judged by validateStageEvidencePayload, the same builder
// and the same validator the engine uses. A case that rebuilt either would
// measure a copy, and a copy stays green through the change that breaks the
// original.
//
// What the expectation MEANS here: which criteria the conversation settles,
// and which way. A scenario names a criterion by its key and says met true or
// false; it does not name the line, because a criterion can be settled by more
// than one passage and grading the citation's exact location would grade
// phrasing rather than reading. THAT the citation is grounded is the
// validator's job, and it runs first.
//
// The empty expectation is the important one. Most conversations settle no
// criterion at all, and a site that invents one puts a claim in front of a rep
// that nobody made — which is the failure this whole ledger exists to prevent.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/claims"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// stageEvidenceFixture is one conversation and the criteria it is read
// against, in the shape the engine hands the site.
type stageEvidenceFixture struct {
	Criteria []stageEvidenceCriterion `json:"criteria"`
	Spans    []stageEvidenceSpan      `json:"spans"`
}

// stageEvidenceExpectation is one criterion the scenario says the conversation
// settles, which way, and where the passage that settles it lives.
//
// The exact LINE is deliberately not pinned: a criterion can be settled by more
// than one passage, and a scenario naming the line would fail a correct reading
// that quoted the other one. What is pinned is `settled_by` — a phrase that
// must appear in the quote. The validator only proves a quote is grounded
// SOMEWHERE in the lines it cites, so without this a reply could settle
// economic_buyer_identified while quoting "helpful, thank you" and be graded
// correct: grounded, on the right criterion, and evidence of nothing.
//
// Empty means the scenario does not constrain the passage, for a criterion
// whose settling sentence has no single stable phrase.
type stageEvidenceExpectation struct {
	CriterionKey string `json:"criterion_key"`
	Met          bool   `json:"met"`
	// SettledBy is a phrase the quote must contain, compared under the same
	// whitespace collapsing the grounding check uses.
	SettledBy string `json:"settled_by,omitempty"`
}

// stageEvidenceCases serves the one site that reads a deal's exit criteria
// against what was actually said.
type stageEvidenceCases struct{}

func (stageEvidenceCases) Site() aitasks.Site {
	return aitasks.Site{
		Task:    ai.TaskStageEvidenceExtract,
		Variant: "criteria",
		Kind:    ai.SiteKindOneShot,
	}
}

// CertifiedScope is the site's own kind: reading a conversation IS one call.
// A claim under the floor is dropped rather than asked about again, so the
// call this case makes is the whole path.
func (stageEvidenceCases) CertifiedScope() string { return aitasks.ScopeFullInvocation }

// Prepare turns one conversation and the criteria it settles into a runnable
// case.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (stageEvidenceCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var conversation stageEvidenceFixture
	if err := json.Unmarshal(fixture, &conversation); err != nil {
		return nil, fmt.Errorf(
			"stage_evidence_extract/criteria: the fixture is not the shape this site takes: %w", err)
	}
	if err := refuseUnreadableConversation(conversation); err != nil {
		return nil, err
	}
	var want []stageEvidenceExpectation
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf(
			"stage_evidence_extract/criteria: the expected answer is not a list of settled criteria: %w", err)
	}
	if err := refuseUnreachableCriteria(want, conversation); err != nil {
		return nil, err
	}
	return &stageEvidenceCase{fixture: conversation, expected: want}, nil
}

// refuseUnreadableConversation names a fixture the site could never have been
// given, so a scenario that measures nothing cannot sit in the corpus looking
// like coverage.
func refuseUnreadableConversation(f stageEvidenceFixture) error {
	if len(f.Criteria) == 0 {
		return errors.New("stage_evidence_extract/criteria: the fixture offers no criterion, " +
			"so there is nothing for a claim to name")
	}
	if len(f.Spans) == 0 {
		return errors.New("stage_evidence_extract/criteria: the fixture supplies no span, " +
			"so there is no conversation to read")
	}
	keys := map[string]bool{}
	for _, c := range f.Criteria {
		if c.Key == "" || c.Label == "" {
			return errors.New("stage_evidence_extract/criteria: a criterion carries no key or no label, " +
				"and the model is offered both")
		}
		if keys[c.Key] {
			return fmt.Errorf("stage_evidence_extract/criteria: criterion key %q appears twice", c.Key)
		}
		keys[c.Key] = true
	}
	ids := map[string]bool{}
	for _, span := range f.Spans {
		if span.SourceID == "" {
			return errors.New("stage_evidence_extract/criteria: a span carries no id, " +
				"so no claim could cite it")
		}
		if ids[span.SourceID] {
			return fmt.Errorf("stage_evidence_extract/criteria: span id %q appears twice", span.SourceID)
		}
		ids[span.SourceID] = true
		if len(span.Lines) == 0 {
			return fmt.Errorf("stage_evidence_extract/criteria: span %q holds no line", span.SourceID)
		}
		for i, line := range span.Lines {
			if strings.TrimSpace(line) == "" {
				return fmt.Errorf("stage_evidence_extract/criteria: span %q line %d is blank, "+
					"and a claim citing it would quote nothing as its evidence", span.SourceID, i+1)
			}
		}
	}
	return nil
}

// refuseUnreachableCriteria names an expectation the validator can never
// satisfy. An EMPTY expectation is not one of them — it is the abstention
// scenario, and it is the one most conversations deserve.
func refuseUnreachableCriteria(want []stageEvidenceExpectation, f stageEvidenceFixture) error {
	if len(want) > maxStageEvidenceClaims {
		return fmt.Errorf("stage_evidence_extract/criteria: the scenario expects %d claims, "+
			"but this site reads at most %d", len(want), maxStageEvidenceClaims)
	}
	offered := map[string]bool{}
	for _, c := range f.Criteria {
		offered[c.Key] = true
	}
	seen := map[string]bool{}
	for i, e := range want {
		if !offered[e.CriterionKey] {
			return fmt.Errorf("stage_evidence_extract/criteria: expectation %d names criterion %q, "+
				"which this fixture does not offer — the validator would refuse the reply that satisfied it",
				i+1, e.CriterionKey)
		}
		if seen[e.CriterionKey] {
			return fmt.Errorf("stage_evidence_extract/criteria: criterion %q is expected twice, "+
				"and one reading settles it once", e.CriterionKey)
		}
		seen[e.CriterionKey] = true
	}
	return nil
}

// stageEvidenceCase is one conversation ready to be read, closed over the
// criteria it was asked about and the answers the scenario expects.
type stageEvidenceCase struct {
	fixture  stageEvidenceFixture
	expected []stageEvidenceExpectation
}

// Run issues the one request this site sends, bare: production wraps the same
// request in the shape-retry when the brain supports one, and a case that did
// too would certify the answer a model gives after being told to try again.
//
// English, pinned, rather than the installation's base language: a
// certification record grades a fixed corpus, and a score that moved with a
// settings row would not be comparable between two installations or across one
// that changed its mind.
func (c *stageEvidenceCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := stageEvidenceRequest(c.fixture.Criteria, c.fixture.Spans, string(textlang.English))
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("stage_evidence_extract/criteria: %w", err)
	}
	trace.Output = resp.Text
	return trace, nil
}

// Evaluate applies the engine's own checks in the engine's own order — parse,
// then the validator against the conversation that was asked about — and only
// then asks whether the criteria settled are the ones the scenario expects. A
// reply that fails the validator has no claims to disagree with.
//
// The confidence floor is deliberately not applied. It is the engine's decision
// about what to do with a reading it already believes, not a judgement on
// whether the reading is usable, and folding it in would report a hedged
// correct reading as a broken reply.
func (c *stageEvidenceCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	var payload stageEvidencePayload
	if err := json.Unmarshal([]byte(ai.Unfence(trace.Output)), &payload); err != nil {
		return aitasks.Outcome{
			Result: aitasks.OutcomeInvalid,
			Detail: fmt.Sprintf("unparseable model output: %v", err),
		}
	}
	keys := make(map[string]bool, len(c.fixture.Criteria))
	for _, criterion := range c.fixture.Criteria {
		keys[criterion.Key] = true
	}
	byID := make(map[string]stageEvidenceSpan, len(c.fixture.Spans))
	for _, span := range c.fixture.Spans {
		byID[span.SourceID] = span
	}
	if msg := validateStageEvidencePayload(payload, keys, byID); msg != "" {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: msg}
	}
	if disagreements := c.disagreements(payload); len(disagreements) > 0 {
		return aitasks.Outcome{
			Result: aitasks.OutcomeWrongAnswer,
			Detail: strings.Join(disagreements, "; "),
		}
	}
	// A correct reading that settles NOTHING is an abstention, and it gets its
	// own word rather than being folded into acceptance. Most conversations in
	// a real pipeline settle no criterion, so this is the common answer — and
	// the rate at which a model abstains where it should is the number the
	// corpus exists to measure. Reporting it as an ordinary acceptance would
	// hide a site that had started inventing claims behind a score that stayed
	// green.
	if len(payload.claims()) == 0 {
		return aitasks.Outcome{Result: aitasks.OutcomeAbstained}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}

// disagreements names every criterion the scenario expects and the reply
// missed, every criterion the reply claims and the scenario does not, and every
// one it settles the wrong way — all of them, because a reading that found one
// of three is not the near miss one line would read as.
//
// Only criterion keys are named in the detail. The conversation's own words are
// a correspondent's, and they have no business being echoed into a record.
func (c *stageEvidenceCase) disagreements(payload stageEvidencePayload) []string {
	wanted := make(map[string]bool, len(c.expected))
	for _, e := range c.expected {
		wanted[e.CriterionKey] = e.Met
	}
	settledBy := make(map[string]string, len(c.expected))
	for _, e := range c.expected {
		settledBy[e.CriterionKey] = e.SettledBy
	}
	var out []string
	answered := map[string]bool{}
	for _, claim := range payload.claims() {
		want, expected := wanted[claim.CriterionKey]
		if !expected {
			out = append(out, fmt.Sprintf(
				"the reply settles %q, which the conversation does not", claim.CriterionKey))
			continue
		}
		answered[claim.CriterionKey] = true
		if got := claim.Met == stageEvidenceMet; got != want {
			out = append(out, fmt.Sprintf(
				"the reply reads %q as met=%t and the conversation states met=%t",
				claim.CriterionKey, got, want))
		}
		// The quote must be the passage that SETTLES it, not merely a grounded
		// one. The validator proves a quote is somewhere in the cited lines; a
		// pleasantry from the same conversation clears that bar and evidences
		// nothing.
		if phrase := settledBy[claim.CriterionKey]; phrase != "" &&
			!strings.Contains(claims.CollapseSpace(claim.Quote), claims.CollapseSpace(phrase)) {
			out = append(out, fmt.Sprintf(
				"the reply settles %q quoting a passage that does not say it",
				claim.CriterionKey))
		}
	}
	for key := range wanted {
		if !answered[key] {
			out = append(out, fmt.Sprintf(
				"the conversation settles %q and the reply does not claim it", key))
		}
	}
	// Map iteration is unordered, and a detail line that reshuffles between
	// runs reads as a different failure each time.
	sort.Strings(out)
	return out
}
