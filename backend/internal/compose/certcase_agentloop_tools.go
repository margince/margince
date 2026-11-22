// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The tool surface a certified scheduled agent calls. Reads are answered from
// the fixture, decoded into production types so it serves only real shapes; the
// one write is checked as its store checks it, then acknowledged, never performed.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// agentLoopToolAnswer is what one tool call is answered with. Args, when set,
// is a subset the call must carry to get this answer. So two reads of one tool
// on two records are two entries.
type agentLoopToolAnswer struct {
	Tool   string                     `json:"tool"`
	Args   map[string]json.RawMessage `json:"args,omitempty"`
	Result json.RawMessage            `json:"result,omitempty"`
}

// agentLoopToolSurface is the tool surface a certification run is offered. It
// advertises the registered surface, as a passport admitting every scope would.
// The runner narrows it to the agent's allowlist as it narrows a production run's. It holds no state, so a replay of the same replies meets
// the same answers.
type agentLoopToolSurface struct {
	work    agentLoopWork
	list    agentLoopWorklist
	answers []agentLoopToolAnswer
}

func (agentLoopToolSurface) Specs() []mcp.ToolSpec { return agentLoopRegistry() }

func (agentLoopToolSurface) Runnable(context.Context) []mcp.ToolSpec { return agentLoopRegistry() }

func (agentLoopToolSurface) Explain(_ context.Context, _ string, err error) string {
	return agents.RunRefusal(err)
}

// Invoke answers one call. A read no fixture entry matches is ErrNotFound, the
// existence-hiding answer a read of a record nobody wrote gets in production.
func (s agentLoopToolSurface) Invoke(_ context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	args, err := agents.WithoutReservedArgs(args)
	if err != nil {
		return nil, err
	}
	if name == s.work.write {
		if _, err := s.work.wrote(args, s.list); err != nil {
			return nil, err
		}
		return s.work.acknowledge(args, s.fixedAnswer(name, args))
	}
	if name == s.work.list {
		return s.list.normalized, nil
	}
	if answer := s.fixedAnswer(name, args); answer != nil {
		return answer, nil
	}
	return nil, apperrors.ErrNotFound
}

// fixedAnswer is the first fixture entry for tool whose pinned arguments the
// call carries, or nil when none does.
func (s agentLoopToolSurface) fixedAnswer(tool string, args json.RawMessage) json.RawMessage {
	for _, answer := range s.answers {
		if answer.Tool == tool && len(agentLoopArgDisagreements(answer.Args, args)) == 0 {
			return answer.Result
		}
	}
	return nil
}

// refuseUnanswerableTools names a fixture answer no call of this run could
// reach. That is an answer for a tool the agent is not offered, or one pinning
// an argument the tool's schema does not declare.
func refuseUnanswerableTools(answers []agentLoopToolAnswer, specs []mcp.ToolSpec) error {
	offered := map[string]mcp.ToolSpec{}
	for _, spec := range specs {
		offered[spec.Name] = spec
	}
	for _, answer := range answers {
		spec, ok := offered[answer.Tool]
		if !ok {
			return fmt.Errorf("%s: the fixture answers %q, which this agent is not offered", agentLoopSite, answer.Tool)
		}
		declared := agentLoopSchemaProperties(spec.InputSchema)
		for _, arg := range sortedArgNames(answer.Args) {
			if !declared[arg] {
				return fmt.Errorf("%s: the fixture's answer for %s pins %q, which its input schema does not declare",
					agentLoopSite, answer.Tool, arg)
			}
		}
	}
	return nil
}

// listAnswer is the fixture's one answer to the agent's list read. The list is
// the run's whole worklist, and two answers would leave the grade to whichever
// the model's arguments matched.
func listAnswer(answers []agentLoopToolAnswer, list string) (json.RawMessage, error) {
	var found []json.RawMessage
	for _, answer := range answers {
		if answer.Tool == list {
			found = append(found, answer.Result)
		}
	}
	if len(found) != 1 {
		return nil, fmt.Errorf("%s: the fixture answers %s %d times; the run's worklist is exactly one answer to it",
			agentLoopSite, list, len(found))
	}
	return found[0], nil
}

// decodeStrict reads raw into the production type v, refusing a field that
// type does not hold: a fixture answer the tool could not have produced.
//
//craft:ignore naked-any it decodes into whichever production result type the caller passes, as encoding/json does.
func decodeStrict(raw json.RawMessage, v any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(v)
}

// writeTool is the agent's one tool that changes a record, derived from the
// registry's own scope on each allowlisted tool. An agent whose allowlist holds
// none or two has no single write a run of it could be graded on.
func writeTool(allowlist []string, specs []mcp.ToolSpec) (string, error) {
	var writes []string
	for _, spec := range specs {
		if slices.Contains(allowlist, spec.Name) && !spec.ReadOnly() {
			writes = append(writes, spec.Name)
		}
	}
	if len(writes) != 1 {
		return "", fmt.Errorf("%s: this agent attaches %d writing tools (%s), and a run is graded on its one write",
			agentLoopSite, len(writes), strings.Join(writes, ", "))
	}
	return writes[0], nil
}

// WalkReplies is a reply calling each tool the fixture answers, in its order,
// and then one that ends the run.
func (c *agentLoopCase) WalkReplies() []string {
	replies := make([]string, 0, len(c.tools.answers)+1)
	for _, answer := range c.tools.answers {
		args := answer.Args
		if args == nil {
			args = map[string]json.RawMessage{}
		}
		step, err := json.Marshal(struct {
			Tool string                     `json:"tool"`
			Args map[string]json.RawMessage `json:"args"`
		}{answer.Tool, args})
		if err == nil {
			replies = append(replies, string(step))
		}
	}
	return append(replies, `{"final":{"summary":"walked every answered tool"}}`)
}

// agentLoopWorklist is the list read as production types: the subjects a run
// must write about, in the list's order, and the answer the tool serves.
type agentLoopWorklist struct {
	normalized json.RawMessage
	subjects   []string
	// evidence is each brief item's recorded evidence, which the brief's store
	// holds a citation to.
	evidence map[string][]ids.UUID
}

// agentLoopWork is one agent's worklist and write. write is filled from the
// allowlist (writeTool), so this table names only what the goal says.
type agentLoopWork struct {
	list  string
	write string
	// onceEach says a subject takes one write and no more. A sweep logs one note
	// per deal, where a later brief pass replaces an earlier finding.
	onceEach bool
	read     func(json.RawMessage) (agentLoopWorklist, error)
	// wrote is the subjects one write names, refusing what the store refuses.
	wrote func(args json.RawMessage, list agentLoopWorklist) ([]string, error)
	// acknowledge is what the tool answers an accepted write with.
	acknowledge func(args, fixed json.RawMessage) (json.RawMessage, error)
}

// agentLoopWorks is keyed by scheduled agent; TestEveryScheduledAgentHasAGradableWork
// holds it total against the catalog.
var agentLoopWorks = map[string]agentLoopWork{
	"morning_brief": {
		list: "read_brief", read: readBriefWorklist, wrote: annotatedItems, acknowledge: acknowledgeAnnotation,
	},
	"overnight_at_risk_sweep": {
		list: "whats_slipping_this_week", onceEach: true, read: readSlippingWorklist, wrote: loggedDeals,
		acknowledge: acknowledgeLoggedNote,
	},
}

func readBriefWorklist(raw json.RawMessage) (agentLoopWorklist, error) {
	var brief agents.ReadBriefResult
	if err := decodeStrict(raw, &brief); err != nil {
		return agentLoopWorklist{}, fmt.Errorf("the read_brief answer is not the brief the tool serves: %w", err)
	}
	if brief.Items == nil {
		brief.Items = []agents.BriefItem{}
	}
	if brief.FactorsOmitted == nil {
		brief.FactorsOmitted = []string{}
	}
	list := agentLoopWorklist{evidence: map[string][]ids.UUID{}}
	for _, item := range brief.Items {
		list.subjects = append(list.subjects, item.ItemID.String())
		list.evidence[item.ItemID.String()] = item.EvidenceIDs
	}
	normalized, err := json.Marshal(brief)
	list.normalized = normalized
	return list, err
}

func readSlippingWorklist(raw json.RawMessage) (agentLoopWorklist, error) {
	var slipping agents.WhatsSlippingResult
	if err := decodeStrict(raw, &slipping); err != nil {
		return agentLoopWorklist{}, fmt.Errorf("the whats_slipping_this_week answer is not the list the tool serves: %w", err)
	}
	var list agentLoopWorklist
	for _, deal := range slipping.Deals {
		list.subjects = append(list.subjects, deal.DealID.String())
	}
	normalized, err := json.Marshal(slipping)
	list.normalized = normalized
	return list, err
}

// annotatedItems refuses what the brief's store refuses: an item that is not
// in the run, and a citation outside that item's evidence (briefs.CitationsWithin).
func annotatedItems(args json.RawMessage, list agentLoopWorklist) ([]string, error) {
	var in agents.AnnotateBriefArgs
	if err := decodeStrict(args, &in); err != nil {
		return nil, httperr.Validation("items", "invalid", err.Error())
	}
	named := make([]string, 0, len(in.Items))
	for _, item := range in.Items {
		evidence, inRun := list.evidence[item.ItemID.String()]
		if !inRun {
			return nil, apperrors.ErrNotFound
		}
		ann := briefs.ItemAnnotation{ItemID: item.ItemID, Finding: item.Finding, CitedEvidence: item.CitedEvidence}
		if err := briefs.CitationsWithin(ann, evidence); err != nil {
			return nil, err
		}
		named = append(named, item.ItemID.String())
	}
	return named, nil
}

// acknowledgeAnnotation answers what annotate_brief answers a write it stored.
func acknowledgeAnnotation(args, _ json.RawMessage) (json.RawMessage, error) {
	var in agents.AnnotateBriefArgs
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	return json.Marshal(agents.AnnotateBriefResult{ItemsAnnotated: len(in.Items), NarrativeWritten: in.Narrative != ""})
}

// agentLoopLoggedNote is what the sweep's grader reads of one log_activity call.
type agentLoopLoggedNote struct {
	Kind  string  `json:"kind"`
	Body  *string `json:"body"`
	Links []struct {
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
	} `json:"links"`
}

// loggedDeals is the deals one note is about. It refuses only what the activity
// store refuses of a scheduled run's note, a missing body. So a note on the
// wrong deal is acknowledged and graded wrong.
func loggedDeals(args json.RawMessage, _ agentLoopWorklist) ([]string, error) {
	var note agentLoopLoggedNote
	if err := json.Unmarshal(args, &note); err != nil {
		return nil, httperr.Validation("kind", "invalid", err.Error())
	}
	if err := activities.RunNoteBodyRefusal(note.Body); err != nil {
		return nil, err
	}
	var deals []string
	for _, link := range note.Links {
		if link.EntityType == "deal" {
			deals = append(deals, link.EntityID)
		}
	}
	return deals, nil
}

// acknowledgeLoggedNote answers with the activity the fixture says was logged:
// the created record is the store's, and nothing here can mint one.
func acknowledgeLoggedNote(_, fixed json.RawMessage) (json.RawMessage, error) {
	if fixed == nil {
		return nil, fmt.Errorf("%s: the fixture gives no log_activity answer to acknowledge a note with", agentLoopSite)
	}
	return fixed, nil
}

// gradeWrites holds the writes a run made to its worklist. Every subject is
// written about, none outside the list is, and each once where the agent logs once.
func (w agentLoopWork) gradeWrites(writes []json.RawMessage, list agentLoopWorklist) []string {
	listed := map[string]bool{}
	for _, subject := range list.subjects {
		listed[subject] = true
	}
	counts := map[string]int{}
	var problems []string
	for _, args := range writes {
		subjects, err := w.wrote(args, list)
		if err != nil {
			problems = append(problems, "a write the store would refuse was recorded as written: "+err.Error())
			continue
		}
		if len(subjects) == 0 {
			problems = append(problems, w.write+" was called about no subject on the list")
		}
		for _, subject := range subjects {
			if !listed[subject] {
				problems = append(problems, fmt.Sprintf("%s wrote about %s, which is not on the %s list", w.write, subject, w.list))
			}
			counts[subject]++
		}
	}
	for _, subject := range list.subjects {
		switch n := counts[subject]; {
		case n == 0:
			problems = append(problems, fmt.Sprintf("nothing was written about %s from the %s list", subject, w.list))
		case n > 1 && w.onceEach:
			problems = append(problems, fmt.Sprintf("%s was written about %d times; the goal asks for one", subject, n))
		}
	}
	return problems
}
