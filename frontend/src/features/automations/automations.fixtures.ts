// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";

type Automation = components["schemas"]["Automation"];
type CatalogEntry = components["schemas"]["AutomationCatalogEntry"];
type AutomationRun = components["schemas"]["AutomationRun"];

const OBJECT = "object";
const NO_PARAMS = { type: OBJECT, additionalProperties: false, properties: {} };
const TIMEZONE = { type: "string", format: "timezone" };
const RECIPIENT = { type: "string", enum: ["record_owner", "authorizer"] };
const TASK_TITLE = { type: "string", minLength: 1, maxLength: 200 };
const dueInDays = (fallback: number) => ({
  type: "integer",
  minimum: 1,
  maximum: 30,
  default: fallback,
});
const quietDays = (fallback: number) => ({
  type: "integer",
  minimum: 1,
  maximum: 365,
  default: fallback,
});
const LIST_DESCRIPTION =
  " It fires from the list's check every 15 minutes, once for each record seen joining or leaving. A record that joins and leaves between two checks is not seen. When one check moves more than 100 records, the rule fires for none of them and pauses itself.";
const listRule = (extra: Record<string, unknown>, required: string[]) => ({
  type: OBJECT,
  additionalProperties: false,
  required: ["list_id", ...required],
  properties: {
    include_imported_changes: { type: "boolean", default: false },
    list_id: { type: "string", format: "live_list" },
    direction: {
      type: "string",
      enum: ["entered", "left", "either"],
      default: "entered",
    },
    ...extra,
  },
});
const stageRule = (task: boolean) => ({
  type: OBJECT,
  additionalProperties: false,
  required: task
    ? ["pipeline_id", "stage_ids", "task_title", "due_in_days"]
    : ["pipeline_id", "stage_ids"],
  properties: {
    pipeline_id: { type: "string", format: "pipeline" },
    stage_ids: {
      type: "array",
      format: "stages",
      items: { type: "string", format: "uuid" },
      minItems: 1,
    },
    ...(task
      ? {
          task_title: TASK_TITLE,
          due_in_days: dueInDays(2),
          timezone: TIMEZONE,
        }
      : {}),
  },
});

// The server's closed catalog as it answers GET /automations/catalog, plus one
// template newer than this client (close_approval), which has no words here.
export const AUTOMATION_CATALOG: CatalogEntry[] = [
  {
    key: "no_activity_reminder",
    group: "deals",
    name: "Remind me when an active deal goes quiet",
    description:
      "Create a task for the owner of a quiet open deal in the selected pipeline. Genuine contact resets its clock.",
    trigger: "clock:no_activity_scan",
    action: "create_task",
    tier: "auto_execute",
    params_schema: {
      type: OBJECT,
      additionalProperties: false,
      required: ["no_activity_days", "pipeline_id"],
      properties: {
        no_activity_days: quietDays(14),
        pipeline_id: { type: "string", format: "pipeline" },
      },
    },
  },
  {
    key: "renewal_reminder",
    group: "customers",
    name: "Prepare for a renewal",
    description:
      "Creates a task for the chosen recipient before a saved renewal date. Annual repeats are optional.",
    trigger: "clock:renewal_scan",
    action: "create_task",
    tier: "auto_execute",
    params_schema: {
      type: OBJECT,
      additionalProperties: false,
      required: ["object", "date_field", "days_before", "recipient"],
      properties: {
        recipient: RECIPIENT,
        days_before: { type: "integer", minimum: 1, maximum: 365, default: 30 },
        object: {
          type: "string",
          enum: ["contact", "company", "deal", "lead", "project"],
        },
        date_field: { type: "string" },
        recurs_yearly: { type: "boolean", default: false },
      },
    },
  },
  {
    key: "stage_change_notify",
    group: "deals",
    name: "Tell the owner when someone moves their deal",
    description:
      "Notifies the deal's owner when someone else or an automation moves the deal, including closes. The owner's own moves stay in deal history.",
    trigger: "deal.stage_changed",
    action: "notify",
    tier: "auto_execute",
    params_schema: stageRule(false),
  },
  {
    key: "route_lead",
    group: "inquiries",
    name: "Route new lead to a task",
    description:
      "Creates a follow-up task the moment a new lead is captured, so every lead gets a first next step.",
    trigger: "lead.created",
    action: "create_task",
    tier: "auto_execute",
    params_schema: {
      type: OBJECT,
      additionalProperties: false,
      properties: { due_in_days: dueInDays(1), timezone: TIMEZONE },
    },
  },
  {
    key: "check_in_cadence",
    group: "customers",
    name: "Keep in touch with selected customers",
    description:
      "Create a task for a customer list member's owner after the chosen period without genuine contact. An open equivalent reminder prevents another task.",
    trigger: "clock:check_in_scan",
    action: "create_task",
    tier: "auto_execute",
    params_schema: {
      type: OBJECT,
      additionalProperties: false,
      required: ["check_in_days", "customer_list_id"],
      properties: {
        check_in_days: quietDays(30),
        customer_list_id: {
          type: "string",
          format: "record_list",
          "x-entity-types": ["contact", "company"],
        },
      },
    },
  },
  {
    key: "post_meeting_recap",
    group: "meetings",
    name: "Draft a follow-up after a meeting",
    description:
      "Drafts a follow-up once a customer meeting is held and has usable notes. The draft waits for a colleague and is never sent by the rule.",
    trigger: "activity.captured",
    action: "draft_email",
    tier: "confirmation_required",
    params_schema: NO_PARAMS,
  },
  {
    key: "website_inquiry",
    group: "inquiries",
    name: "Handle website inquiries",
    description:
      "Route accepted website inquiries and give a colleague one task to answer each request.",
    trigger: "activity.captured",
    action: "create_task",
    tier: "auto_execute",
    params_schema: {
      type: OBJECT,
      additionalProperties: false,
      properties: {
        form_id: { type: "string", format: "uuid" },
        coordinator_id: { type: "string", format: "uuid" },
        mode: {
          type: "string",
          enum: ["one_colleague", "rotate", "coordinator"],
        },
        task_title: TASK_TITLE,
        due_in_days: dueInDays(1),
        timezone: TIMEZONE,
      },
    },
  },
  {
    key: "assign_lead_owner",
    group: "inquiries",
    name: "Assign new leads an owner",
    description:
      "Assigns every new lead an owner: matching rules first, else round-robin across the pool, never exceeding capacity caps.",
    trigger: "lead.created",
    action: "assign_owner",
    tier: "auto_execute",
    params_schema: {
      type: OBJECT,
      additionalProperties: false,
      properties: {
        owners: { type: "array", items: { type: "string", format: "uuid" } },
        cap_per_owner: { type: "integer", minimum: 1 },
        rules: { type: "array" },
      },
    },
  },
  {
    key: "stage_change_create_task",
    group: "deals",
    name: "Add the next task when a deal reaches a stage",
    description:
      "Adds the chosen task when a deal enters a selected open stage. Each visit to the stage gets one task.",
    trigger: "deal.stage_changed",
    action: "create_task",
    tier: "auto_execute",
    params_schema: stageRule(true),
  },
  {
    key: "list_membership_task",
    group: "lists",
    name: "Create a task when someone joins this list",
    description: `Creates a task for the chosen recipient when a record joins or leaves the list.${LIST_DESCRIPTION}`,
    trigger: "list.evaluated",
    action: "create_task",
    tier: "auto_execute",
    params_schema: listRule(
      {
        due_in_days: dueInDays(2),
        timezone: TIMEZONE,
        recipient: RECIPIENT,
        task_title: TASK_TITLE,
      },
      ["recipient", "task_title"],
    ),
  },
  {
    key: "list_membership_notify",
    group: "lists",
    name: "Tell me when this list changes",
    description: `Sends the rule's owner a notice for each record that joins or leaves the list.${LIST_DESCRIPTION}`,
    trigger: "list.evaluated",
    action: "notify",
    tier: "auto_execute",
    params_schema: listRule({}, []),
  },
  {
    key: "list_membership_shortlist",
    group: "lists",
    name: "Save matching records to a list",
    description: `Adds each record that joins or leaves the list to a Shortlist of the same record type. A record already on it stays as it is.${LIST_DESCRIPTION}`,
    trigger: "list.evaluated",
    action: "add_to_shortlist",
    tier: "auto_execute",
    params_schema: listRule(
      { shortlist_id: { type: "string", format: "shortlist" } },
      ["shortlist_id"],
    ),
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
      params: {
        no_activity_days: 14,
        pipeline_id: "01a0f000-0000-7000-8000-0000000000a1",
      },
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
      params: {
        object: "deal",
        date_field: "cf_contract_end",
        days_before: 30,
        recipient: "record_owner",
      },
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
      last_run_at: ago(24 * 40),
      last_run_outcome: "fired",
      runs_last_30_days: 0,
    },
  ];
}

export function configuredAutomation(id: string, now: number): Automation {
  const rule = configuredAutomations(now).find(
    (candidate) => candidate.id === id,
  );
  if (!rule) throw new Error(`No automation fixture named ${id}`);
  return rule;
}

const GLOBEX = {
  type: "deal",
  id: "01a1e5f1-7a5c-7000-8000-00000000d001",
  label: "Globex Renewal",
};

/**
 * Runs as a current server traces them. A task on a named deal, a draft that
 * waits for approval, a notice on a hidden deal. The last is an older server's
 * run, with only its English lines.
 */
export const TRACED_RUNS: readonly AutomationRun[] = [
  {
    id: "r-task",
    automation_id: "au-1",
    occurred_at: "2026-11-20T13:02:00Z",
    outcome: "fired",
    tier: "auto_execute",
    event_id: "01a1e5f1-7a5c-7000-8000-0000000000e1",
    trigger_event_type: "deal.stage_changed",
    trigger_stage_name: "Proposal",
    action_kinds: ["create_task"],
    target: GLOBEX,
    created: { type: "activity", label: "Send the proposal" },
    trigger_evidence: "deal Globex Renewal entered Proposal",
    action_result: "applied create_task",
  },
  {
    id: "r-draft",
    automation_id: "au-1",
    occurred_at: "2026-11-20T12:40:00Z",
    outcome: "queued_for_approval",
    tier: "confirmation_required",
    event_id: "01a1e5f1-7a5c-7000-8000-0000000000e2",
    trigger_event_type: "activity.captured",
    action_kinds: ["draft_email"],
    target: GLOBEX,
    action_result: "queued to approval inbox",
  },
  {
    id: "r-hidden",
    automation_id: "au-1",
    occurred_at: "2026-11-20T11:15:00Z",
    outcome: "fired",
    tier: "auto_execute",
    event_id: "01a1e5f1-7a5c-7000-8000-0000000000e3",
    trigger_event_type: "deal.stage_changed",
    action_kinds: ["notify"],
    target: { type: "deal", id: "01a1e5f1-7a5c-7000-8000-00000000d002" },
  },
  {
    id: "r-older",
    automation_id: "au-1",
    occurred_at: "2026-11-19T09:00:00Z",
    outcome: "fired",
    tier: "auto_execute",
    trigger_evidence: "deal BÄR Pharma entered Negotiation",
    target_ref: "deal:BÄR Pharma",
    action_result: "created follow-up task",
  },
];
