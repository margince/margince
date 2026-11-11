// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Translator } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { type CalendarChange, guestLabel } from "../../lib/calendarchange";
import { labelFor } from "../../lib/labelfor";
import { partyName } from "../../lib/worklistcopyreasons";
import type { WorklistItem } from "./worklist.queries";

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

/** The sentence saying what this row asks of its reader. */
export function askSentence(item: WorklistItem, t: Translator): string {
  const change = item.email_summary?.calendar_change;
  // An invitation waits on an RSVP, not on a written answer.
  const invitation = item.email_summary?.calendar_invitation;
  if (!change)
    return t(
      invitation
        ? "worklist.ask.invitation"
        : labelFor(ASK_BY_SOURCE, item.source),
    );
  const name = changedBy(item, change);
  if (invitation)
    return name
      ? t("worklist.ask.changedInvitationNamed", { name })
      : t("worklist.ask.changedInvitation");
  return name
    ? t(labelFor(ASK_BY_NAMED_MEETING_CHANGE, change.kind), { name })
    : t(labelFor(ASK_BY_MEETING_CHANGE, change.kind));
}
