// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

type Automation = components["schemas"]["Automation"];
type CatalogEntry = components["schemas"]["AutomationCatalogEntry"];

const daysSchema = (fallback: number) => ({
  type: "object",
  properties: {
    days: { type: "integer", minimum: 1, maximum: 90, default: fallback },
  },
  required: ["days"],
});

const NO_PARAMS = { type: "object", properties: {} };

export const AUTOMATION_CATALOG: CatalogEntry[] = [
  {
    key: "no_activity_reminder",
    name: "No-activity reminder",
    description:
      "Reminds an entity's owner once its most recent captured activity has gone quiet for N days.",
    trigger: "clock:no_activity_scan",
    action: "create_task",
    tier: "auto_execute",
    params_schema: daysSchema(14),
  },
  {
    key: "renewal_reminder",
    name: "Renewal reminder",
    description:
      "Reminds an entity's owner as its configured renewal date approaches.",
    trigger: "clock:renewal_scan",
    action: "create_task",
    tier: "auto_execute",
    params_schema: daysSchema(30),
  },
  {
    key: "stage_change_notify",
    name: "Stage-change notify",
    description:
      "Notifies the deal's owner when someone else or an automation moves the deal, including closes. The owner's own moves stay in deal history.",
    trigger: "deal.stage_changed",
    action: "notify",
    tier: "auto_execute",
    params_schema: NO_PARAMS,
  },
  {
    key: "close_approval",
    name: "Approval before closing",
    description: "Asks a manager to approve a deal before it closes.",
    trigger: "deal.stage_changed",
    action: "request_approval",
    tier: "confirmation_required",
    params_schema: NO_PARAMS,
  },
  {
    key: "post_meeting_recap",
    name: "Post-meeting recap draft",
    description: "Drafts a follow-up recap whenever a meeting is logged.",
    trigger: "activity.captured",
    action: "draft_email",
    tier: "auto_execute",
    params_schema: NO_PARAMS,
  },
  {
    key: "list_membership_notify",
    name: "Live List alert",
    description: "Tells you when a record joins a Live List you watch.",
    trigger: "list.evaluated",
    action: "notify",
    tier: "auto_execute",
    params_schema: NO_PARAMS,
  },
];

const HOUR_MS = 3_600_000;

/**
 * One configured rule per state the table draws: each last-run outcome, a rule
 * that never ran, and one the system paused. `now` dates the runs.
 */
export function configuredAutomations(now: number): Automation[] {
  const ago = (hours: number) => new Date(now - hours * HOUR_MS).toISOString();
  const base = { version: 2, created_at: "2026-07-01T08:00:00Z" };
  return [
    {
      ...base,
      id: "au-1",
      key: "no_activity_reminder",
      name: "Quiet accounts follow-up",
      status: "enabled",
      params: { days: 14 },
      last_run_at: ago(2),
      last_run_outcome: "fired",
      runs_last_30_days: 9,
    },
    {
      ...base,
      id: "au-2",
      key: "renewal_reminder",
      name: "Renewal reminder",
      status: "enabled",
      params: { days: 30 },
      last_run_at: ago(26),
      last_run_outcome: "failed",
      runs_last_30_days: 3,
    },
    {
      ...base,
      id: "au-3",
      key: "stage_change_notify",
      name: "Stage-change notify",
      status: "enabled",
      params: {},
      last_run_at: ago(72),
      last_run_outcome: "blocked",
      runs_last_30_days: 3,
    },
    {
      ...base,
      id: "au-4",
      key: "close_approval",
      name: "Approval before closing",
      status: "enabled",
      params: {},
      last_run_at: ago(5),
      last_run_outcome: "queued_for_approval",
      runs_last_30_days: 1240,
    },
    {
      ...base,
      id: "au-5",
      key: "post_meeting_recap",
      name: "Post-meeting recap draft",
      status: "paused",
      params: {},
      runs_last_30_days: 0,
    },
    {
      ...base,
      id: "au-6",
      key: "list_membership_notify",
      name: "Tell me about new buyers",
      status: "paused",
      paused_reason: "list_archived",
      params: { list_id: "01a0f000-0000-7000-8000-000000000001" },
      last_run_at: ago(290),
      last_run_outcome: "fired",
      runs_last_30_days: 1,
    },
  ];
}
