// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

var changeLeadStateCopy = toolCopy{
	Purpose: "Move a lead through its lifecycle. qualify fills what the lead's own data already " +
		"implies — today the company name, from its email domain — and names the qualification " +
		"fields still empty. promote turns a lead who has genuinely engaged into a contact, " +
		"carrying their history across. disqualify closes out a lead that is going nowhere. " +
		"demote reverses a promotion that should not have happened.",
	Limits: "qualify fills only empty fields and never overwrites or invents a value. promote " +
		"needs a trigger naming the engagement — a reply, a booked or held meeting, or a human's " +
		"decision; cold outreach nobody answered never promotes. disqualify keeps the record and " +
		"its history. demote needs a reason and is refused once the promoted contact owns a deal.",
	Instead: "Use enrich to learn about a company from its website, and update_record to edit a " +
		"lead's fields.",
	Retain: "The gaps qualify reports are what a human still has to supply. A promotion answers " +
		"with the new contact's id. Where an installation has raised an action to confirm first, " +
		"the answer is a staged approval: keep its id and do not report the change until the " +
		"retry carrying it has answered.",
}
