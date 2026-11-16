// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { type CalendarChange, guestLabel } from "./calendarchange";
import { labelFor } from "./labelfor";
import { mayOnlyRead, partyName } from "./worklistcopyreasons";
import type { WorklistItem } from "./worklisttypes";

// What a row asks of its reader, one sentence per source. A row's reasons say
// why it ranks where it does, and none of them says what to do about it.
//
// A Record over the contract's source union, so a source the server adds fails
// the build here until it has a sentence.
const ASK_BY_SOURCE: Readonly<Record<WorklistItem["source"], MessageKey>> = {
  approval: "worklist.ask.approval",
  dedupe_candidate: "worklist.ask.dedupe_candidate",
  contact_case: "worklist.ask.contact_case",
  deal_suggestion: "worklist.ask.deal_suggestion",
  tag_suggestion: "worklist.ask.tag_suggestion",
  task: "worklist.ask.task",
  brief_item: "worklist.ask.brief_item",
  conversation_claim: "worklist.ask.conversation_claim",
  customer_waiting: "worklist.ask.customer_waiting",
  lead_response: "worklist.ask.lead_response",
  deal_at_risk: "worklist.ask.deal_at_risk",
  meeting: "worklist.ask.meeting",
  relationship_decay: "worklist.ask.relationship_decay",
  failed_approval: "worklist.ask.failed_approval",
  dsr: "worklist.ask.dsr",
  notice_case: "worklist.ask.notice_case",
  capture_health: "worklist.ask.capture_health",
  booking_health: "worklist.ask.booking_health",
  domain_question: "worklist.ask.domain_question",
  ai_work_health: "worklist.ask.ai_work_health",
  bounce: "worklist.ask.bounce",
  undelivered: "worklist.ask.undelivered",
  automation_run: "worklist.ask.automation_run",
  notice: "worklist.ask.notice",
  introduction_request: "worklist.ask.introduction_request",
  meeting_outcome: "worklist.ask.meeting_outcome",
  weekly_commitment: "worklist.ask.weekly_commitment",
  awaiting_reply: "worklist.ask.awaiting_reply",
  meeting_follow_up: "worklist.ask.meeting_follow_up",
  batch: "worklist.ask.batch",
};

// A brief item opens on the fact that put the deal on the queue. The most
// actionable fact comes first, so the card leads with a reason, not a source.
const BRIEF_ASK_BY_REASON = [
  ["closing_soon", "worklist.ask.brief.closing_soon"],
  ["no_next_step", "worklist.ask.brief.no_next_step"],
  ["no_champion", "worklist.ask.brief.no_champion"],
] as const satisfies readonly (readonly [string, MessageKey])[];

const BRIEF_ASK_BY_SIGNAL: Readonly<Record<string, MessageKey>> = {
  closing_soon: "worklist.ask.brief.signal.closing_soon",
  stalled: "worklist.ask.brief.signal.stalled",
  opportunity: "worklist.ask.brief.signal.opportunity",
  moved: "worklist.ask.brief.signal.moved",
};

const ASK_BY_MEETING_CHANGE: Readonly<
  Record<CalendarChange["kind"], MessageKey>
> = {
  updated: "worklist.ask.meeting.updated",
  canceled: "worklist.ask.meeting.canceled",
  proposed: "worklist.ask.meeting.proposed",
  answered: "worklist.ask.meeting.answered",
};

const ASK_BY_NAMED_MEETING_CHANGE: Readonly<
  Record<CalendarChange["kind"], MessageKey>
> = {
  updated: "worklist.ask.meetingNamed.updated",
  canceled: "worklist.ask.meetingNamed.canceled",
  proposed: "worklist.ask.meetingNamed.proposed",
  answered: "worklist.ask.meetingNamed.answered",
};

// Who moved: the guest who answered or proposed, else the mail's sender, who
// is the organizer for an update or a cancellation.
function changedBy(item: WorklistItem, change: CalendarChange): string {
  if (change.by) return guestLabel(change.by);
  return partyName(item)?.trim() ?? "";
}

/** The sentence a row says, naming who changed a meeting when one did. */
export function askSentence(item: WorklistItem, t: Translator): string {
  return meetingAsk(item, t) ?? t(askKey(item));
}

/** What a changed meeting asks, or undefined for a row about no meeting change. */
export function meetingAsk(
  item: WorklistItem,
  t: Translator,
): string | undefined {
  const change = item.email_summary?.calendar_change;
  if (!change) return undefined;
  const name = changedBy(item, change);
  if (item.email_summary?.calendar_invitation)
    return name
      ? t("worklist.ask.changedInvitationNamed", { name })
      : t("worklist.ask.changedInvitation");
  return name
    ? t(labelFor(ASK_BY_NAMED_MEETING_CHANGE, change.kind), { name })
    : t(labelFor(ASK_BY_MEETING_CHANGE, change.kind));
}

/**
 * The reason a brief item's ask already says, so the row does not repeat it.
 * Only a card says its ask, and not while it asks whether it may be done.
 */
export function askedReason(
  item: WorklistItem,
  said = true,
): string | undefined {
  if (!said || !saysOpen(item) || item.source !== "brief_item")
    return undefined;
  const kinds = new Set(item.because.map((reason) => reason.kind));
  return BRIEF_ASK_BY_REASON.find(([kind]) => kinds.has(kind))?.[0];
}

/** The sentence saying what this row asks of its reader. */
export function askKey(item: WorklistItem): MessageKey {
  // An invitation waits on an RSVP, not on a written answer.
  if (item.email_summary?.calendar_invitation) return "worklist.ask.invitation";
  // A reminder whose request cannot be checked will not close by itself.
  if (item.request_check) return "worklist.ask.task_unchecked";
  if (
    item.source === "capture_health" &&
    item.connection?.medium === "calendar"
  )
    return "worklist.ask.capture_health.calendar";
  if (item.source === "brief_item") return briefAskKey(item);
  // The organizer answers it; an attendee who cannot change it is reminded.
  if (item.source === "meeting_outcome" && mayOnlyRead(item))
    return "worklist.ask.meeting_outcome.read_only";
  return labelFor(ASK_BY_SOURCE, item.source);
}

function briefAskKey(item: WorklistItem): MessageKey {
  const stated = askedReason(item);
  const byReason = BRIEF_ASK_BY_REASON.find(([kind]) => kind === stated);
  if (byReason) return byReason[1];
  const bySignal = item.kind ? BRIEF_ASK_BY_SIGNAL[item.kind] : undefined;
  return bySignal ?? "worklist.ask.brief_item";
}

/**
 * A row that asks whether our later mail kept its promise drops the open line.
 * The question takes its place.
 */
export function saysOpen(item: WorklistItem): boolean {
  return !item.may_be_done;
}
