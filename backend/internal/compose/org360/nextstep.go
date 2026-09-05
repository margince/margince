// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

// The no-next-step rule, and the step it prepares.
//
// It is the only rule here that answers with WORK rather than with a reading.
// The others point at something that already happened — a mail nobody answered,
// a deal that stopped moving, a stage the correspondence contradicts — and the
// reader decides what to do about it. This one fires on an ABSENCE, so pointing
// is not available to it: "nothing is scheduled here" restates the empty list
// the reader is looking at.
//
// So it names the step, in the words the button writes, and hands over the body
// that writes it. Still no model and still nothing staged: what to name is
// decided from the same open pipeline the reason sentence is read off, and
// nothing exists until a rep presses the button.

import (
	"fmt"
	"strconv"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// noNextStepSuggestion fires on an account that is live — it has an open
// deal — and has nobody's next action written down.
//
// It is deliberately NOT raised for a quiet account with no open deal: there
// is nothing to advance, and "you have no task on this dormant account" is
// noise the rep would learn to scroll past, which costs the whole surface its
// credibility.
func noNextStepSuggestion(
	orgID ids.OrganizationID, in suggestionInputs,
) *crmcontracts.Organization360Suggestion {
	if in.scheduled || in.open.OpenCount == 0 {
		return nil
	}
	open := in.open
	evidence := openDealEvidence(orgID, open.Open)
	step := recommendedNextStep(orgID, in)
	// The digest over EVERY open deal rides the fingerprint, so closing one or
	// opening another re-raises this rather than leaving a dismissal in force
	// over a pipeline the account no longer has — including a change to a deal
	// no card listed, which a fingerprint built from a fetched page would miss.
	out := &crmcontracts.Organization360Suggestion{
		Kind:        suggestNoNextStep,
		Reason:      noNextStepReason(open),
		Fingerprint: fingerprint(string(suggestNoNextStep), open.OpenDigest, evidence),
		Evidence:    evidence,
		// The ask IS the step, in the words the button will write. It used to
		// read "Set the next step", which hands the reader back their own
		// problem: a card that has already worked out there is nothing
		// scheduled, and which deals that is true of, can say what to schedule.
		Title: ptrString(step.body.Subject),
	}
	// No date: this rule fires on the ABSENCE of a task, and an absence has no
	// date of its own. Inventing one would make a reading into a deadline — the
	// prepared body leaves due_at unset for the same reason.
	setAddTask(out, step)
	return out
}

// nextStep is the step this advice asks for: the body that writes it, and the
// deal it hangs on where there is exactly one to hang it on.
type nextStep struct {
	body   crmcontracts.CreateTaskRequest
	dealID *ids.UUID
}

// recommendedNextStep prepares that step, as the body POST /tasks takes.
//
// What it names is the step the RECORDS support and no more: agree what happens
// next, on the deal or with the account. Naming the work itself — "prep the
// expansion workshop", the row the mockups drew — would be the system inventing
// a commitment nobody made, and a rep who accepted it would be carrying a task
// their account never generated.
//
// It hangs on the DEAL when exactly one is open, because then there is nothing
// to choose. With several it hangs on the ACCOUNT, for the same reason the
// reason sentence names them all rather than picking one: a task filed against
// the wrong deal is worse than one filed against the account, which is where a
// rep would have put it themselves.
func recommendedNextStep(orgID ids.OrganizationID, in suggestionInputs) nextStep {
	open := in.open
	if len(open.Open) == 1 && open.OpenCount == 1 {
		deal := open.Open[0]
		return nextStep{
			body:   taskBody("Agree the next step on "+strconv.Quote(deal.Name), "deal", deal.ID),
			dealID: &deal.ID,
		}
	}
	subject := "Agree the next step with this account"
	if in.orgName != "" {
		subject = "Agree the next step with " + in.orgName
	}
	return nextStep{body: taskBody(subject, "organization", orgID.UUID)}
}

// taskBody is the step as POST /tasks takes it. `source` is the UI because that
// is where the click happens: a rep pressing the button on this card is the
// author of the task, and recording anything else would put an actor in the
// audit trail who did not decide it.
//
//nolint:staticcheck // ST1003: the field names mirror the oapi-codegen type this must assign to
func taskBody(
	subject string, entityType crmcontracts.CreateTaskRequestLinksEntityType, entityID ids.UUID,
) crmcontracts.CreateTaskRequest {
	links := []struct {
		EntityId   openapi_types.UUID                            `json:"entity_id"`
		EntityType crmcontracts.CreateTaskRequestLinksEntityType `json:"entity_type"`
	}{{EntityId: openapi_types.UUID(entityID), EntityType: entityType}}
	return crmcontracts.CreateTaskRequest{Subject: subject, Source: "ui", Links: &links}
}

// How many deals the advice names before it stops naming them. Past three the
// list stops being a reason and becomes an inventory, and the deals section
// above it is already the inventory.
const namedDeals = 3

// The reason, with the deals in it.
//
// A count alone is a claim a reader cannot check without leaving the card, and
// on an account with one open deal it is also worse writing than the deal's own
// name. Past the cap it stays a count: the reader is being told there is no
// next step, not being handed the pipeline.
func noNextStepReason(open pipeline) string {
	names := make([]string, 0, namedDeals)
	for _, deal := range open.Open {
		if len(names) == namedDeals {
			break
		}
		names = append(names, strconv.Quote(deal.Name))
	}
	switch {
	case len(names) == 0:
		// The count survived a read the names did not. Rare, and the advice is
		// still true: something is open and nothing says what happens next.
		return fmt.Sprintf("%d deals are open here and no task says what happens next.", open.OpenCount)
	case open.OpenCount == 1:
		return fmt.Sprintf("%s is open and no task says what happens next.", names[0])
	case open.OpenCount > len(names):
		return fmt.Sprintf(
			"%d deals are open here, including %s, and no task says what happens next.",
			open.OpenCount, strings.Join(names, ", "),
		)
	default:
		return fmt.Sprintf(
			"%s are open and no task says what happens next.", strings.Join(names, ", "),
		)
	}
}

// What the advice was read from: the open deals themselves, so the receipt
// opens the records the claim is about.
//
// The organization stands in when the deals did not survive the read. The
// suggestion is dismissible and its dismissal is keyed on this list, so an
// empty one would key every account's dismissal alike.
func openDealEvidence(
	orgID ids.OrganizationID, deals []openDeal,
) []crmcontracts.OrganizationBriefEvidence {
	if len(deals) == 0 {
		return []crmcontracts.OrganizationBriefEvidence{{
			EntityType: crmcontracts.OrganizationBriefEvidenceEntityTypeOrganization,
			EntityId:   openapi_types.UUID(orgID.UUID),
		}}
	}
	out := make([]crmcontracts.OrganizationBriefEvidence, 0, min(len(deals), namedDeals))
	for _, deal := range deals {
		if len(out) == namedDeals {
			break
		}
		out = append(out, crmcontracts.OrganizationBriefEvidence{
			EntityType: crmcontracts.OrganizationBriefEvidenceEntityTypeDeal,
			EntityId:   openapi_types.UUID(deal.ID),
		})
	}
	return out
}
