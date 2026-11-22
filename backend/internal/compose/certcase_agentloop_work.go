// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What each scheduled agent's run is FOR, as a grader can check it: the list
// its goal tells it to read first, the subjects that list names, and which of
// them one write is about. A run is right when its writes cover the list and
// nothing outside it, which is the boundary both goals state.

import (
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

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
	// onceEach says a subject takes exactly one write: a sweep logs one note per
	// deal, where a later brief pass replaces an earlier finding.
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

// loggedDeals is the deals one note is about. It refuses what the activity
// store refuses of a scheduled run's note, a missing body, and nothing more, so
// a note on the wrong deal is acknowledged and graded wrong.
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

// gradeWrites holds the writes a run made to its worklist: every subject
// written about, none outside the list, and once each where the agent logs once.
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
