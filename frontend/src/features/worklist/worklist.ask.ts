// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../../i18n/en";
import type { WorklistItem } from "./worklist.queries";

// What a row asks of its reader, one sentence per source. A row's reasons say
// why it ranks where it does, and none of them says what to do about it.
//
// A Record over the contract's source union, so a source the server adds fails
// the build here until it has a sentence.
const ASK_BY_SOURCE: Readonly<Record<WorklistItem["source"], MessageKey>> = {
  approval: "worklist.ask.approval",
  dedupe_candidate: "worklist.ask.dedupe_candidate",
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

/** The sentence saying what this row asks of its reader. */
export function askKey(item: WorklistItem): MessageKey {
  return ASK_BY_SOURCE[item.source];
}
