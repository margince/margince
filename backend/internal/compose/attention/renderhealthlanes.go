// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The sync-health lane's card: one per CONDITION, aggregated by the owning
// module, so a broken connector is a single card rather than a flood.

import (
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
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
		// The mailbox is the condition's NAME as well as part of its identity,
		// and both travel. A group formed on the identity draws from the label;
		// a reader shown the identity reads "capture_health:disconnected:…".
		label := mailbox
		item.CauseLabel = &label
	}
	// WHEN the condition started, for the conditions that have a start. The
	// reader's question about a failing mailbox is how long, not whether — a
	// class alone is the same sentence at one minute and at six hours — and
	// this is the field every other lane already answers it in. Absent for a
	// condition that is a state rather than a streak, because a disconnected
	// mailbox has no duration to report and inventing one from the row's
	// timestamps would date the newest attempt rather than the outage.
	if concern.FailingSince != nil {
		started := *concern.FailingSince
		item.OccurredAt = &started
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
		// The label is the task's DISPLAY NAME, generated from the same
		// declaration as the task keys (api/ai-tasks.yaml), so a task cannot be
		// added without one and the two cannot drift.
		//
		// Not the key. `site_triage`, `signal_extract` and their siblings are
		// generated enum vocabulary, and a rep reading "site_triage failed 8
		// times" learns nothing they can act on — which is the defect the label
		// exists to remove, so sending the key would move the leak rather than
		// close it. Not the run's own summary either: it is written per run, so
		// a group of twelve failures would be named after whichever was sampled.
		//
		// Empty is left unlabelled rather than filled. DisplayName answers ""
		// for a task this build does not know — a kind read back from a row an
		// older binary wrote — and a group with no name it can trust says the
		// generic phrase, which is what it said for every kind before.
		if name := ai.DisplayName(ai.Task(run.Kind)); name != "" {
			label := name
			item.CauseLabel = &label
		}
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
	if detail := bounceDetail(send); detail != "" {
		item.Detail = &detail
	}
	if !send.PersonID.IsZero() {
		item.Subject = subjectOf("person", send.PersonID)
		item.Actions = append(item.Actions, actionOpen)
	}
	return item
}

// bounceDetail is the supporting line: the address that refused, the receiving
// side's own words, or both. The address leads because it is what the reader
// acts on. Empty when the send carries neither, so the card draws no line
// rather than an empty one.
func bounceDetail(send BouncedSend) string {
	switch {
	case send.Recipient != "" && send.Reason != "":
		return send.Recipient + " — " + send.Reason
	case send.Recipient != "":
		return send.Recipient
	default:
		return send.Reason
	}
}

// parkedItem draws one send that was given up on. The subject line is the
// headline and the dispatcher's own reason the supporting line, exactly as the
// bounce card reads — the difference is in the verb the client writes, because
// this message is still unsent and that one is not.
func parkedItem(send ParkedSend) crmcontracts.AttentionItem {
	occurred := send.ParkedAt
	item := crmcontracts.AttentionItem{
		Id:         send.ID.String(),
		Source:     crmcontracts.AttentionItemSource("undelivered"),
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
// the closed failed/blocked vocabulary. No subject: fixing the RULE lives on
// the automation screens, and this row does not pretend to.
//
// A FAILED firing carries `retry`, and a blocked one does not. The difference
// is not tidiness: `blocked` is the permission gate having refused the firing
// on purpose, so a Retry button there would offer to overrule a refusal it
// cannot overrule — the retry endpoint declines it, and a control whose only
// outcome is a refusal is worse than no control.
//
// The verb rides `item.Id`, which is the RUN's id. That is what the endpoint
// takes: a retry re-dispatches one firing, while `cause_ref` below names the
// rule the firings are grouped under.
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
	if run.Outcome == outcomeFailed {
		item.Actions = append(item.Actions, crmcontracts.AttentionItemActionsRetry)
	}
	// The condition is the RULE, not this firing of it and not its name: a
	// name is mutable and not unique, so two rules sharing a name would merge
	// and a renamed rule would split its own history.
	if !run.AutomationID.IsZero() {
		cause := "automation_run:" + run.AutomationID.String()
		item.CauseRef = &cause
		// And the NAME travels beside it, for the reader. This is the case the
		// split exists for: the identity a group is formed on has to be the id
		// for the reason above, and a rep shown "automation_run:01a0…" learns
		// nothing about which rule stopped working.
		if name != "" {
			label := name
			item.CauseLabel = &label
		}
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
//
// The record it is about rides `subject`, which is what makes the row's
// headline a link. "A deal you own changed stage" is true of every deal a rep
// owns, so a reader knew something had moved and had to go find it. A notice
// that names no record — a capture backlog, a coach's word — carries none, and
// so does one naming a type this feed cannot route to: subjectOf refuses it
// rather than guessing, because a card pointing at the wrong record is worse
// than one pointing nowhere.
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
	if notice.TargetType != "" {
		item.Subject = subjectOf(notice.TargetType, notice.TargetID)
	}
	if notice.Body != "" {
		body := notice.Body
		item.Detail = &body
	}
	return item
}

// introductionItem draws one ask a colleague is waiting on this reader to
// answer.
//
// No title. The headline this card needs is the contact's name, and that
// arrives on `subject` through fillSubjectLabels, which reads it under the
// reader's own grants. A title written HERE would be an English sentence on a
// product that ships three languages, and a name resolved here would be a read
// this assembler does not hold the grants to make.
//
// The requester's own reason is the detail, quoted rather than summarised: the
// colleague is deciding whether to spend their own relationship, and a
// paraphrase of why is not what they would be agreeing to.
//
// One verb, `decide`, and it routes to the ask's own endpoint. Answering is
// irreversible and only the introducer may do it, so it is never settled from
// the queue row itself.
func introductionItem(ask PendingIntroduction) crmcontracts.AttentionItem {
	requested := ask.RequestedAt
	due := ask.DueAt
	item := crmcontracts.AttentionItem{
		Id:         ask.ID.String(),
		Source:     crmcontracts.AttentionItemSource("introduction_request"),
		OccurredAt: &requested,
		DueAt:      &due,
		Actions:    []crmcontracts.AttentionItemActions{crmcontracts.AttentionItemActions("decide")},
		Subject:    subjectOf("person", ask.PersonID),
	}
	if ask.Reason != "" {
		reason := ask.Reason
		item.Detail = &reason
	}
	return item
}
