// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The sync-health lane's card: one per CONDITION, aggregated by the owning
// module, so a broken connector is a single card rather than a flood.

import (
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// syncItem draws one sync concern. The card carries no subject and no verbs:
// its subject is the CONNECTION, not a record with a page, and fixing it lives
// on the sync settings screen. `kind` names the condition; `detail` carries
// that condition's facts in the producer's own vocabulary — the affected
// object classes, the failure class, or the budget band — and the client
// writes the sentence in the reader's language.
//
// The id is the concern's kind: a concern is a condition, not a row, and the
// lane carries at most one card per condition.
func syncItem(concern SyncConcern) crmcontracts.AttentionItem {
	kind := concern.Kind
	item := crmcontracts.AttentionItem{
		Id:      concern.Kind,
		Source:  crmcontracts.AttentionItemSource("sync_health"),
		Kind:    &kind,
		Actions: []crmcontracts.AttentionItemActions{},
	}
	switch {
	case len(concern.Objects) > 0:
		classes := strings.Join(concern.Objects, ", ")
		item.Detail = &classes
	case concern.ErrorClass != "":
		errorClass := concern.ErrorClass
		item.Detail = &errorClass
	case concern.Band != "":
		band := concern.Band
		item.Detail = &band
	}
	return item
}

// captureItem draws one capture concern. Like the sync card it carries no
// subject and no verbs — fixing a mailbox lives on the capture settings
// screen. `kind` names the condition; `detail` names the mailbox in the
// reader's own terms: the account label the connector reported, or the
// provider where none was.
func captureItem(concern CaptureConcern) crmcontracts.AttentionItem {
	kind := concern.Kind
	mailbox := concern.AccountLabel
	if mailbox == "" {
		mailbox = concern.Provider
	}
	item := crmcontracts.AttentionItem{
		Id:      concern.ConnectionID.String(),
		Source:  crmcontracts.AttentionItemSource("capture_health"),
		Kind:    &kind,
		Actions: []crmcontracts.AttentionItemActions{},
	}
	// The condition AND the mailbox it is about. Two disconnected mailboxes
	// are two things to reconnect, and one row saying "disconnected" would
	// send the reader to fix one and silently lose the other.
	if mailbox != "" {
		item.Detail = &mailbox
		cause := "capture_health:" + kind + ":" + mailbox
		item.CauseRef = &cause
	}
	return item
}

// aiWorkItem draws one troubled AI run. `kind` is the closed failed/stalled
// vocabulary the client writes its sentence from; the run's own summary, when
// it recorded one, is the headline, and the subject label the supporting
// line. No subject and no verbs: the run's home is the AI activity rail, and
// re-running is a decision this surface does not own.
func aiWorkItem(run TroubledRun) crmcontracts.AttentionItem {
	state := run.State
	occurred := run.OccurredAt
	item := crmcontracts.AttentionItem{
		Id:         run.ID.String(),
		Source:     crmcontracts.AttentionItemSource("ai_work_health"),
		Kind:       &state,
		OccurredAt: &occurred,
		Actions:    []crmcontracts.AttentionItemActions{},
	}
	// The condition is the TASK that ran. Not the run's summary, which is
	// written per run and would draw one incident per failure, and not the
	// state, which is `failed` for every broken task there is.
	if run.Kind != "" {
		cause := "ai_work_health:" + run.Kind
		item.CauseRef = &cause
	}
	if run.Summary != "" {
		summary := run.Summary
		item.Title = &summary
	}
	if run.SubjectLabel != "" {
		about := run.SubjectLabel
		item.Detail = &about
	}
	return item
}

// bounceItem draws one hard-bounced send. The subject line is the headline —
// the name the reader knows the send by — and the receiving side's own reason
// the supporting line. `open` is offered exactly when the send is filed under
// a person, because that page is where fixing the address and resending live.
func bounceItem(send BouncedSend) crmcontracts.AttentionItem {
	kind := "hard"
	occurred := send.BouncedAt
	item := crmcontracts.AttentionItem{
		Id:         send.ID.String(),
		Source:     crmcontracts.AttentionItemSource("bounce"),
		Kind:       &kind,
		OccurredAt: &occurred,
		Actions:    []crmcontracts.AttentionItemActions{},
	}
	if send.Subject != "" {
		subject := send.Subject
		item.Title = &subject
	}
	if send.Reason != "" {
		reason := send.Reason
		item.Detail = &reason
	}
	if !send.PersonID.IsZero() {
		item.Subject = subjectOf("person", send.PersonID)
		item.Actions = append(item.Actions, actionOpen)
	}
	return item
}

// automationItem draws one troubled firing. The rule's own name is the
// headline and the engine's recorded reason the supporting line; `kind` is
// the closed failed/blocked vocabulary. No subject and no verbs: fixing the
// rule lives on the automation screens.
func automationItem(run TroubledAutomationRun) crmcontracts.AttentionItem {
	outcome := run.Outcome
	name := run.Name
	occurred := run.OccurredAt
	item := crmcontracts.AttentionItem{
		Id:         run.ID.String(),
		Source:     crmcontracts.AttentionItemSource("automation_run"),
		Kind:       &outcome,
		Title:      &name,
		OccurredAt: &occurred,
		Actions:    []crmcontracts.AttentionItemActions{},
	}
	// The condition is the RULE, not this firing of it and not its name: a
	// name is mutable and not unique, so two rules sharing a name would merge
	// and a renamed rule would split its own history.
	if !run.AutomationID.IsZero() {
		cause := "automation_run:" + run.AutomationID.String()
		item.CauseRef = &cause
	}
	if run.Reason != "" {
		reason := run.Reason
		item.Detail = &reason
	}
	return item
}

// noticeItem draws one unread notice: its own subject as the headline, its
// body as the supporting line, and acknowledge — the one verb it offers,
// which routes to the notice's read endpoint and takes it off this lane.
func noticeItem(notice UnreadNotice) crmcontracts.AttentionItem {
	kind := notice.Kind
	subject := notice.Subject
	occurred := notice.CreatedAt
	item := crmcontracts.AttentionItem{
		Id:         notice.ID.String(),
		Source:     crmcontracts.AttentionItemSource("notice"),
		Kind:       &kind,
		Title:      &subject,
		OccurredAt: &occurred,
		Actions:    []crmcontracts.AttentionItemActions{crmcontracts.AttentionItemActions("acknowledge")},
	}
	if notice.Body != "" {
		body := notice.Body
		item.Detail = &body
	}
	return item
}
