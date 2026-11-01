// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";

export const reachNames = {
  domain_question: "brief.coverage.source.domain_question",
  approval: "brief.coverage.source.approval",
  dedupe_candidate: "brief.coverage.source.dedupe_candidate",
  contact_case: "brief.coverage.source.contact_case",
  deal_suggestion: "brief.coverage.source.deal_suggestion",
  tag_suggestion: "brief.coverage.source.tag_suggestion",
  task: "brief.coverage.source.task",
  brief_item: "brief.coverage.source.brief_item",
  conversation_claim: "brief.coverage.source.conversation_claim",
  customer_waiting: "brief.coverage.source.customer_waiting",
  awaiting_reply: "brief.coverage.source.awaiting_reply",
  meeting_follow_up: "brief.coverage.source.meeting_follow_up",
  lead_response: "brief.coverage.source.lead_response",
  deal_at_risk: "brief.coverage.source.deal_at_risk",
  meeting: "brief.coverage.source.meeting",
  meeting_outcome: "brief.coverage.source.meeting_outcome",
  relationship_decay: "brief.coverage.source.relationship_decay",
  failed_approval: "brief.coverage.source.failed_approval",
  dsr: "brief.coverage.source.dsr",
  notice_case: "brief.coverage.source.notice_case",
  capture_health: "brief.coverage.source.capture_health",
  ai_work_health: "brief.coverage.source.ai_work_health",
  bounce: "brief.coverage.source.bounce",
  undelivered: "brief.coverage.source.undelivered",
  automation_run: "brief.coverage.source.automation_run",
  notice: "brief.coverage.source.notice",
  introduction_request: "brief.coverage.source.introduction_request",
  batch: "brief.coverage.source.batch",
  weekly_commitment: "brief.coverage.source.weekly_commitment",
} satisfies Record<
  components["schemas"]["WorklistReach"]["source"],
  MessageKey
>;

const coverageNames = {
  ...reachNames,
  deal_advice: "brief.coverage.source.deal_advice",
  calendar: "brief.coverage.source.calendar",
  next_meeting: "brief.coverage.source.next_meeting",
} satisfies Record<string, MessageKey>;

const sourceLabels = new Map<string, MessageKey>(Object.entries(coverageNames));

export function sourceName(source: string, t: Translator): string {
  return t(sourceLabels.get(source) ?? "brief.coverage.source.generic");
}
