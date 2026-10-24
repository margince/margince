// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package accountdraft

// The fold of a caller's own 360 view into one draft's Input.
//
// Beside the types rather than among them: this is the half that reads a view,
// and it never re-queries.

import (
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/draftcore"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// FromView projects the caller's 360 onto the one recipient and one deal this
// draft is about. It returns the recipient it resolved, or an error naming the
// field, so the caller's own refusal comes from one place.
func FromView(
	view crmcontracts.Company360, req Request,
) (Input, error) {
	contact, err := findContact(view, req.ContactID)
	if err != nil {
		return Input{}, err
	}
	in := Input{
		Intent:     strings.TrimSpace(req.Intent),
		RewriteOf:  strings.TrimSpace(req.RewriteOf),
		Envelope:   req.Envelope,
		CompanyID:  view.Company.Id.String(),
		Company:    view.Company.DisplayName,
		Recipient:  recipientOf(contact),
		Recent:     foldRecent(view),
		Commitment: foldCommitment(view),
	}
	if view.Company.Industry != nil {
		in.Industry = contacts.IndustryWords(string(*view.Company.Industry), req.Envelope.Lang())
	}
	if view.Company.Description != nil {
		in.Description = *view.Company.Description
	}
	// An unnamed deal is the account in general, which is ordinary. So the
	// lookup only runs when the caller named one.
	if req.DealID != "" {
		deal, dealErr := findDeal(view, req.DealID)
		if dealErr != nil {
			return Input{}, dealErr
		}
		in.Deal = &deal
	}
	if req.ProjectID != nil {
		project, projectErr := findProject(view, *req.ProjectID)
		if projectErr != nil {
			return Input{}, projectErr
		}
		in.Project = &project
	}
	return in, nil
}

// findProject folds the named project off the view's own projects section.
// The scoped read has already refused a project the caller cannot see. What
// is left to refuse is a project the caller can see that belongs to another
// account. A draft about this account may not be grounded in it.
func findProject(view crmcontracts.Company360, projectID ids.ProjectID) (ProjectIn, error) {
	if view.Projects == nil {
		return ProjectIn{}, fieldError("project_id", "the account's projects are not readable by you")
	}
	for _, project := range *view.Projects {
		if ids.UUID(project.ProjectId) != projectID.UUID {
			continue
		}
		return projectFact(project, view), nil
	}
	return ProjectIn{}, fieldError("project_id", "that project is not on this account, or you cannot see it")
}

// projectFact is the one project as the draft reads it. The open-commitment
// count comes from the SCOPED next-steps section, which is this project's
// open tasks and the unfiled ones, never another project's.
func projectFact(project crmcontracts.Company360Project, view crmcontracts.Company360) ProjectIn {
	out := ProjectIn{
		ID:    project.ProjectId.String(),
		Name:  project.Name,
		Phase: string(project.Phase),
	}
	if project.Key != nil {
		out.Key = *project.Key
	}
	if project.TargetEndDate != nil {
		out.TargetEnd = project.TargetEndDate.Format(isoDate)
	}
	if view.NextSteps != nil {
		out.OpenCommitments = len(view.NextSteps.Data)
	}
	return out
}

// isoDate is how a date fact is written to the model: the calendar date alone,
// because a project's target end has no time of day.
const isoDate = "2006-01-02"

func recipientOf(contact crmcontracts.Company360Contact) RecipientIn {
	out := RecipientIn{
		ID:        contact.ContactId.String(),
		Name:      contact.FullName,
		FirstName: firstName(contact.FullName),
		LastName:  lastName(contact.FullName),
	}
	if contact.Title != nil {
		out.Title = *contact.Title
	}
	if contact.PrimaryEmail != nil {
		out.Email = *contact.PrimaryEmail
	}
	out.Bucket = string(contact.Strength.Bucket)
	if contact.Strength.LastInteraction != nil {
		out.LastInteraction = contact.Strength.LastInteraction.UTC().Format(rfc3339)
	}
	return out
}

const rfc3339 = "2006-01-02T15:04:05Z"

// firstName is what the greeting uses. Everything before the first space, or
// the whole name when it has none. A one-word name is still a name.
func firstName(full string) string {
	if cut, _, found := strings.Cut(strings.TrimSpace(full), " "); found && cut != "" {
		return cut
	}
	return strings.TrimSpace(full)
}

// lastName is what follows the first space in the display name. That is the
// surname for a two-word name and not always one otherwise: "Dr. Anne Weiss"
// yields "Anne Weiss", a given name inside the result. No rule over display
// names gets every name right. This input carries no name columns to consult:
// the 360 contact it folds has a full name and nothing else.
//
// A single-word name yields empty, which sends the greeting to the familiar
// form rather than to a formal one addressed to a first name.
//
// contactdraft.surname answers the same question from a Contact record and
// prefers that record's own last_name, which is the better answer where it
// exists. The two are not shared because their INPUTS differ, not their
// answer: unifying them would mean passing a Contact into a fold that has none.
//
// The prompt rule protects the output where this is wrong. A formal greeting
// is used only where a surname was given, and the model is told never to
// complete or invent one.
func lastName(full string) string {
	if _, rest, found := strings.Cut(strings.TrimSpace(full), " "); found {
		return strings.TrimSpace(rest)
	}
	return ""
}

// foldCommitment takes the soonest open task. `next_steps.data` arrives
// ordered overdue → due → undated, so the head is it and this makes no
// ordering decision of its own.
func foldCommitment(view crmcontracts.Company360) *TaskIn {
	if view.NextSteps == nil || len(view.NextSteps.Data) == 0 {
		return nil
	}
	step := view.NextSteps.Data[0]
	out := TaskIn{ID: step.ActivityId.String(), Name: step.Subject}
	if step.DueAt != nil {
		out.Due = step.DueAt.UTC().Format(rfc3339)
	}
	return &out
}

// ConversationState reads where this account's correspondence stands off the
// view's own last-message stamps and the notes and meetings logged on it.
//
// Both are absent when the caller holds no activity grant, which reads as a
// first touch. That is the conservative end of the axis and the right answer
// here. A caller who cannot see the history has no basis for a draft that
// refers to it.
func ConversationState(view crmcontracts.Company360, now time.Time) convstate.State {
	var activities []crmcontracts.Activity
	if view.Activities != nil {
		activities = view.Activities.Data
	}
	return draftcore.ClassifyWithLogged(now, instant(view.LastInboundAt), instant(view.LastOutboundAt), activities)
}

// instant reads one optional stamp, treating an absent one as never.
func instant(at *time.Time) time.Time {
	if at == nil {
		return time.Time{}
	}
	return *at
}

// CorrespondenceText is what this account has been written to and from,
// newest first, for detecting the language its correspondence runs in.
//
// Subjects and bodies both, because a subject line rarely carries enough words
// to clear the detector's floor on its own. This drafter opens a new
// conversation, so the text is evidence about the account's language rather
// than about a thread being answered. A German account gets a German first
// touch even though nothing is being replied to.
func CorrespondenceText(view crmcontracts.Company360) string {
	if view.Activities == nil {
		return ""
	}
	var text strings.Builder
	for i, act := range view.Activities.Data {
		if i == draftInputActivities {
			break
		}
		if act.Subject != nil {
			text.WriteString(*act.Subject + "\n")
		}
		if act.Body != nil {
			text.WriteString(*act.Body + "\n\n")
		}
	}
	return text.String()
}

func foldRecent(view crmcontracts.Company360) []ActIn {
	if view.Activities == nil {
		return nil
	}
	out := make([]ActIn, 0, draftInputActivities)
	for _, act := range view.Activities.Data {
		if len(out) == draftInputActivities {
			break
		}
		item := ActIn{
			ID:      act.Id.String(),
			Kind:    string(act.Kind),
			At:      act.OccurredAt.UTC().Format(rfc3339),
			Inbound: act.Direction != nil && *act.Direction == crmcontracts.ActivityDirectionInbound,
		}
		if act.Subject != nil {
			item.Subject = *act.Subject
		}
		if act.Body != nil {
			item.Snippet = textlang.MessageOpening(*act.Body, draftInputSnippetRunes)
		}
		out = append(out, item)
	}
	return out
}

// findContact resolves the named recipient within the caller's own 360, which
// makes the lookup a permission check as well as a lookup. A contact that
// caller cannot see is not in the view, and the refusal is the same 422 as a
// contact id that names nobody.
//
// Not a separate contacts read: that would find contacts the 360 withheld.
func findContact(
	view crmcontracts.Company360, contactID string,
) (crmcontracts.Company360Contact, error) {
	if view.Contacts == nil {
		return crmcontracts.Company360Contact{}, fieldError("contact_id",
			"the account's contacts are not readable by you, so there is nobody here to write to")
	}
	for _, contact := range view.Contacts.Data {
		if contact.ContactId.String() == contactID {
			return contact, nil
		}
	}
	return crmcontracts.Company360Contact{}, fieldError("contact_id",
		"that contact is not a contact you can see on this account")
}

// findDeal resolves the named deal the same way. The caller checks for an
// unnamed one before calling.
//
// A draft about the account in general is an ordinary case rather than a
// missing field. So answering "nothing, and that is fine" is not this
// function's business.
func findDeal(view crmcontracts.Company360, dealID string) (DealIn, error) {
	if view.Deals == nil {
		return DealIn{}, fieldError("deal_id",
			"the account's deals are not readable by you")
	}
	for _, deal := range view.Deals.Data {
		if deal.DealId.String() != dealID {
			continue
		}
		out := DealIn{ID: dealID, Name: deal.Name}
		if deal.StageName != nil {
			out.Stage = *deal.StageName
		}
		if deal.Amount != nil && deal.Amount.AmountMinor != nil && deal.Amount.Currency != nil {
			out.AmountMinor = *deal.Amount.AmountMinor
			out.Currency = *deal.Amount.Currency
		}
		return out, nil
	}
	return DealIn{}, fieldError("deal_id", "that deal is not open on this account, or you cannot see it")
}
