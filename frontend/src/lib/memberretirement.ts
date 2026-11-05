// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";

type Blocker = components["schemas"]["MemberRetirementBlocker"];
type Consequence = components["schemas"]["MemberRetirementConsequence"];
const BLOCKERS: Readonly<Record<Blocker, MessageKey>> = {
  approval_decisions: "users.retirement.approval_decisions",
  automations: "users.retirement.automations",
  booking_pages: "users.retirement.booking_pages",
  data_requests: "users.retirement.data_requests",
  deal_rooms: "users.retirement.deal_rooms",
  introductions: "users.retirement.introductions",
  lists: "users.retirement.lists",
  stewarded_lists: "users.retirement.stewarded_lists",
  mail_drafts: "users.retirement.mail_drafts",
  meeting_invitations: "users.retirement.meeting_invitations",
  meeting_proposals: "users.retirement.meeting_proposals",
  privacy_notices: "users.retirement.privacy_notices",
  record_responsibilities: "users.retirement.record_responsibilities",
  reports: "users.retirement.reports",
  report_schedules: "users.retirement.report_schedules",
  saved_views: "users.retirement.saved_views",
  scheduled_mail: "users.retirement.scheduled_mail",
  handoffs: "users.retirement.handoffs",
  voice_profiles: "users.retirement.voice_profiles",
  web_forms: "users.retirement.web_forms",
  webhooks: "users.retirement.webhooks",
  weekly_plans: "users.retirement.weekly_plans",
  analytics_shares: "users.retirement.analytics_shares",
  source_used: "users.retirement.source_used",
  self_survivor: "users.retirement.self_survivor",
  deactivate_first: "users.retirement.deactivate_first",
  choose_survivor: "users.retirement.choose_survivor",
  choose_receiver: "users.retirement.choose_receiver",
  private_tasks: "users.retirement.private_tasks",
  work_not_transferable: "users.retirement.work_not_transferable",
  state_changed: "users.retirement.state_changed",
};
const CONSEQUENCES: Readonly<Record<Consequence, MessageKey>> = {
  source_signed_out: "users.retirement.source_signed_out",
  connectors_reconnect: "users.retirement.connectors_reconnect",
  survivor_access: "users.retirement.survivor_access",
  original_attribution: "users.retirement.original_attribution",
  private_alerts_archived: "users.retirement.private_alerts_archived",
  crm_private_records_move: "users.retirement.crm_private_records_move",
  private_audiences_retained: "users.retirement.private_audiences_retained",
  email_reserved: "users.retirement.email_reserved",
  survivor_inactive: "users.retirement.survivor_inactive",
};
export const MEMBER_RETIREMENT_EFFECT_LABELS = Object.fromEntries(
  Object.entries(CONSEQUENCES).map(([code, key]) => [`member_${code}`, key]),
);
export function memberRetirementBlocker(code: string, t: Translator): string {
  const key = Object.entries(BLOCKERS).find(
    ([candidate]) => candidate === code,
  )?.[1];
  return t(key ?? "users.retirement.unknown");
}
export function memberRetirementConsequence(
  code: Consequence,
  t: Translator,
): string {
  return t(CONSEQUENCES[code]);
}
