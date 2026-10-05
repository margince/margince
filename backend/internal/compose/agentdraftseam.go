// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// draft_email's first message, drafted by the engines the web composer uses
// and left where the human it is for will find it.
//
// The composer drafts a first message on a contact page through
// POST /contacts/{id}/draft-email, on a company page with a chosen recipient
// through POST /companies/{id}/draft-email, and on a lead through
// POST /leads/{id}/draft-email. draft_email picks the same engine by the same
// rule from the links it is given, so one record and one intent send one
// request to the model whichever surface asked.

import (
	"context"
	"errors"

	"github.com/margince/margince/backend/internal/compose/accountdraft"
	"github.com/margince/margince/backend/internal/compose/contactdraft"
	"github.com/margince/margince/backend/internal/compose/leaddraft"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// firstMessageEngines are the three drafters the web composer serves, held on
// the server so draft_email reaches the same instances, model lanes included.
type firstMessageEngines struct {
	contact *contactdraft.Service
	account *accountdraft.Service
	lead    *leaddraft.Service
}

// firstMessage is what draft_email's links say: who the message is to, and
// which company, deal and project it is about.
type firstMessage struct {
	recipient activities.MailDraftAnchor
	company   ids.UUID
	deal      ids.UUID
	project   *ids.ProjectID
}

// firstMessageOf reads the links. A draft is written from ONE recipient's record,
// so the links must name exactly one contact or lead; a company, deal or
// project only narrows what the message is about, and only where the engine
// that writes it can read one. A link no engine reads is refused, so a draft
// is never written from less than the caller named.
func firstMessageOf(links []agents.RecordLink) (firstMessage, error) {
	var out firstMessage
	for _, link := range links {
		switch crmcontracts.MailDraftAnchorType(link.EntityType) {
		case crmcontracts.MailDraftAnchorTypeContact, crmcontracts.MailDraftAnchorTypeLead:
			if !out.recipient.ID.IsZero() {
				return firstMessage{}, badFirstMessage("links name more than one contact or lead; a draft is written to one recipient")
			}
			out.recipient = activities.MailDraftAnchor{Type: crmcontracts.MailDraftAnchorType(link.EntityType), ID: link.EntityID}
		case crmcontracts.MailDraftAnchorTypeCompany:
			if !out.company.IsZero() {
				return firstMessage{}, badFirstMessage("links name more than one company; name the one the recipient works at")
			}
			out.company = link.EntityID
		case crmcontracts.MailDraftAnchorTypeDeal:
			if !out.deal.IsZero() {
				return firstMessage{}, badFirstMessage("links name more than one deal; a draft is about one")
			}
			out.deal = link.EntityID
		case crmcontracts.MailDraftAnchorTypeProject:
			if out.project != nil {
				return firstMessage{}, badFirstMessage("links name more than one project; a draft is about one")
			}
			project := ids.From[ids.ProjectKind](link.EntityID)
			out.project = &project
		}
	}
	if out.recipient.ID.IsZero() {
		return firstMessage{}, badFirstMessage(
			"links must name the contact or lead the message is to; the draft is written from that recipient's record")
	}
	return out, out.refuseUnreadContext()
}

// refuseUnreadContext names a link the chosen engine cannot draft from: a
// lead's draft reads the lead alone, and a contact's reads a deal only through
// the company it belongs to.
func (m firstMessage) refuseUnreadContext() error {
	if m.recipient.Type == crmcontracts.MailDraftAnchorTypeLead {
		if !m.company.IsZero() || !m.deal.IsZero() || m.project != nil {
			return badFirstMessage("a draft to a lead is written from the lead alone; " +
				"remove the company, deal or project link, or write to a contact instead")
		}
		return nil
	}
	if !m.deal.IsZero() && m.company.IsZero() {
		return badFirstMessage("a deal link needs the company link beside it; " +
			"the draft reads the deal through the contact's company")
	}
	return nil
}

func badFirstMessage(reason string) error {
	return &agents.BadArgsError{Cause: errors.New(reason)}
}

// draft asks the engine the web composer would: a lead's own, the account's
// when a company is named beside the contact, and the contact's otherwise.
func (e *firstMessageEngines) draft(ctx context.Context, m firstMessage, intent string) (crmcontracts.CompanyEmailDraft, error) {
	switch {
	case m.recipient.Type == crmcontracts.MailDraftAnchorTypeLead:
		return e.lead.Draft(ctx, ids.From[ids.LeadKind](m.recipient.ID), leaddraft.Request{Intent: intent})
	case !m.company.IsZero():
		req := accountdraft.Request{ContactID: m.recipient.ID.String(), ProjectID: m.project, Intent: intent}
		if !m.deal.IsZero() {
			req.DealID = m.deal.String()
		}
		return e.account.Draft(ctx, ids.From[ids.CompanyKind](m.company), req)
	default:
		return e.contact.Draft(ctx, ids.From[ids.ContactKind](m.recipient.ID),
			contactdraft.Request{Intent: intent, ProjectID: m.project})
	}
}

// DraftCompanyEmail drafts the first message to a record and leaves it for
// review.
//
// The draft is saved to the composer's saved-draft store for the human the
// agent acts for, anchored on the recipient, so their contact or lead page
// shows it waiting and their composer opens with it. It is NOT an activity:
// the timeline records what happened, and an unsent draft has not.
//
// agents.FollowUpDrafter files its drafts on deal timelines instead, and says
// why at its own declaration: it drafts many for triage on the deals they are
// about, where this drafts one for one recipient.
func (c commsAdapter) DraftCompanyEmail(
	ctx context.Context, links []agents.RecordLink, intent string,
) (agents.FirstDraft, error) {
	if len(links) == 0 {
		return agents.FirstDraft{}, badFirstMessage(
			"links must name at least one record this conversation is filed under; " +
				"a first message has no thread to inherit them from")
	}
	message, err := firstMessageOf(links)
	if err != nil {
		return agents.FirstDraft{}, err
	}
	if c.firstDrafts == nil {
		return agents.FirstDraft{}, errors.New("compose: this surface has no first-message drafting engine wired")
	}
	drafted, err := c.firstDrafts.draft(ctx, message, intent)
	if err != nil {
		return agents.FirstDraft{}, err
	}
	out := firstDraftOf(drafted)
	saved, err := c.store.SaveAgentMailDraft(ctx, message.recipient, activities.MailDraftContent{
		To: out.To, Subject: out.Subject, Body: out.Body,
	})
	if errors.Is(err, activities.ErrOwnDraftWaiting) {
		out.NotSaved = err.Error()
		return out, nil
	}
	if err != nil {
		return agents.FirstDraft{}, err
	}
	out.SavedDraftID = &saved.ID
	return out, nil
}

func firstDraftOf(drafted crmcontracts.CompanyEmailDraft) agents.FirstDraft {
	out := agents.FirstDraft{Subject: drafted.Subject, Body: drafted.Body}
	if drafted.To != nil {
		for _, address := range *drafted.To {
			out.To = append(out.To, string(address))
		}
	}
	if drafted.AiGenerated != nil {
		out.AIGenerated = *drafted.AiGenerated
	}
	if drafted.AiDisclosure != nil {
		out.AIDisclosure = *drafted.AiDisclosure
	}
	return out
}
