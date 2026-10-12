// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { approval, auditEntries } from "./fixtures";
import { versionSkew } from "./records";
import { type Handler, type MockRequest, page } from "./server";
import type { LoggedActivity, MockState } from "./state";

// The receipt Home draws last, one line per lane, so the sweep meets the
// panel's rows, counts and way-back column rather than four empty lanes.
export const magicReceipt = {
  as_of: "2026-09-13T08:00:00Z",
  since: "2026-09-12T08:00:00Z",
  done: [
    {
      id: "00000000-0000-7000-8000-00000000ma01",
      occurred_at: "2026-09-13T07:30:00Z",
      lane: "done",
      summary: { key: "magic.action.advance_stage" },
      actor: { type: "agent", id: "auto-apply" },
      undo: { undoable: false, reason: "not_a_replayable_verb" },
    },
  ],
  needs_you: [
    {
      id: "00000000-0000-7000-8000-00000000ma02",
      occurred_at: "2026-09-13T06:10:00Z",
      lane: "needs_you",
      summary: {
        key: "magic.action.approval_send_email",
        values: { kind: "send_email" },
      },
      consequence: "magic.consequence.awaits_your_decision",
      actor: { type: "agent", id: "overnight" },
      undo: { undoable: false, reason: "no_completed_change" },
    },
  ],
  could_not_complete: [],
  watching: [
    {
      id: "00000000-0000-7000-8000-00000000ma03",
      occurred_at: "2026-09-13T08:00:00Z",
      lane: "watching",
      summary: {
        key: "magic.action.capture_reauth_required",
        values: { provider: "Gmail" },
      },
      consequence: "magic.consequence.capture_not_collecting",
      actor: { type: "system", id: "system:capture" },
      undo: { undoable: false, reason: "no_completed_change" },
    },
  ],
  totals: { done: 1, needs_you: 1, could_not_complete: 0, watching: 1 },
  not_shown: [],
  sources_unavailable: [],
};

export const getBrief: Handler = ({ json }, { brief }) => json(brief);

export const runBrief: Handler = ({ json }, { brief }) => json(brief, 201);

// Act and dismiss marks stick within a test.
export const markBriefItem: Handler = ({ params, json }, { brief }) => {
  const item = brief.items.find((entry) => entry.id === params[0]);
  if (!item) {
    return json({ title: "Not Found" }, 404);
  }
  item.state = params[1] === "act" ? "acted" : "dismissed";
  item.state_at = "2026-07-05T06:00:00Z";
  return json(item);
};

// The day's one ranked queue. Its decision is the approval the approvals
// queue serves: two fixtures for one row let a screen pass against a
// decision the product never staged.
export const worklist = {
  as_of: "2026-07-05T06:00:00Z",
  scope: "mine",
  scope_options: ["mine"],
  summary: { urgent: 0, due: 1, lower_priority: 1, total: 2 },
  sources_unavailable: [],
  // Both accountings, because the server sends both.
  reach: [
    { source: "approval", considered: 1, shown: 1, more_available: false },
    { source: "task", considered: 1, shown: 1, more_available: false },
  ],
  counts: [
    { category: "decisions", considered: 1, shown: 1, more_available: false },
    { category: "tasks", considered: 1, shown: 1, more_available: false },
  ],
  // One routine decision and nothing else, so three readings are zero. The
  // money is null, not 0: nothing at risk means nothing to price.
  readings: {
    revenue_at_risk_minor: null,
    buyer_replies: 0,
    prospecting: 0,
    review: 1,
    more_available: false,
  },
  queue: [
    {
      id: approval.id,
      source: "approval",
      category: "decisions",
      level: 6,
      consequence: "data_drifts",
      kind: approval.kind,
      title: approval.summary,
      because: [{ kind: "routine" }],
      actions: ["decide"],
    },
    // A task row, for the disposition verbs and the duration picker. The
    // mobile case measures every control, and these are the widest a row draws.
    {
      id: "01a05500-0000-7000-8000-0000000000d1",
      source: "task",
      category: "tasks",
      level: 3,
      consequence: "task_slips",
      title: "Send the retrofit quote to Turbinenbau",
      due_at: "2026-07-06T13:00:00Z",
      because: [{ kind: "due_today" }, { kind: "unassigned" }],
      actions: [],
      dispositions: ["snooze", "not_mine"],
    },
  ],
};

// The decision lane reads the one approval it shows, whole.
export function isOneApproval(path: string) {
  return /^\/approvals\/[^/]+$/.test(path) && path !== "/approvals/bundles";
}

export const decideApproval: Handler = ({ json }) =>
  json({ ...approval, status: "approved" });

// Only rows linked to a deal or a project are remembered.
export const logActivity: Handler = ({ route, json }, { loggedActivities }) => {
  const body = route.request().postDataJSON();
  const links: LoggedActivity["links"] = body.links ?? [];
  const created: LoggedActivity = {
    id: "act-new",
    workspace_id: "w",
    kind: body.kind ?? "note",
    subject: body.subject ?? "",
    body: body.body ?? null,
    occurred_at: "2026-07-06T09:00:00Z",
    thread_key: null,
    links,
    source: "manual",
    captured_by: "human:u1",
    version: 1,
    created_at: "2026-07-06T09:00:00Z",
    updated_at: "2026-07-06T09:00:00Z",
  };
  if (links.some((link) => ["deal", "project"].includes(link.entity_type))) {
    created.id = `act-new-${loggedActivities.length + 1}`;
    loggedActivities.push(created);
  }
  return json(created, 201);
};

// Idempotent like the real one: the same link twice is one link.
export const relinkActivity: Handler = (
  { route, params, json },
  { loggedActivities },
) => {
  const activity = loggedActivities.find((row) => row.id === params[0]);
  if (!activity) {
    return json({ title: "Not Found", status: 404 }, 404);
  }
  const body = route.request().postDataJSON();
  if (body.replace_existing_of_type) {
    activity.links = activity.links.filter(
      (link) => link.entity_type !== body.entity_type,
    );
  }
  const linked = activity.links.some(
    (link) =>
      link.entity_type === body.entity_type &&
      link.entity_id === body.entity_id,
  );
  if (!linked) {
    activity.links.push({
      entity_type: body.entity_type,
      entity_id: body.entity_id,
    });
  }
  return json(activity);
};

export const listActivities: Handler = (
  { url, json },
  { loggedActivities },
) => {
  const entityType = url.searchParams.get("entity_type");
  const entityId = url.searchParams.get("entity_id");
  if (entityType !== "deal" && entityType !== "project") {
    return json(page([]));
  }
  return json(
    page(
      loggedActivities.filter((row) =>
        row.links.some(
          (link) =>
            link.entity_type === entityType && link.entity_id === entityId,
        ),
      ),
    ),
  );
};

export const consentPurposes = page([
  {
    id: "cp1",
    workspace_id: "w",
    key: "marketing_email",
    label: "Marketing",
    requires_double_opt_in: true,
    created_at: "2026-06-01T00:00:00Z",
  },
]);

// Filtered by actor, entity type and action, two rows to a page.
export const auditLog: Handler = ({ url, json }) => {
  const actor = url.searchParams.get("actor");
  const entityType = url.searchParams.get("entity_type");
  const action = url.searchParams.get("action");
  const cursor = url.searchParams.get("cursor");
  const rows = auditEntries.filter(
    (entry) =>
      (!actor || entry.actor_id === actor) &&
      (!entityType || entry.entity_type === entityType) &&
      (!action || entry.action === action),
  );
  if (!cursor && rows.length > 2) {
    return json({ data: rows.slice(0, 2), page: { next_cursor: "c1" } });
  }
  return json({
    data: cursor ? rows.slice(2) : rows,
    page: { next_cursor: null },
  });
};

export const listAutomations: Handler = ({ json }, { automations }) =>
  json(page(automations));

// A new automation starts paused, so the create, paused, enable flow holds.
export const createAutomation: Handler = ({ route, json }, state) => {
  const body = route.request().postDataJSON();
  const created = {
    id: `au-${state.automations.length + 1}`,
    key: String(body.key),
    name: String(body.name),
    params: body.params,
    status: "paused",
    version: 1,
    created_at: "2026-07-05T08:00:00Z",
  };
  state.automations = [...state.automations, created];
  return json(created, 201);
};

type Automation = MockState["automations"][number];

// The contract's optimistic lock: a PATCH without the row's version is a
// conflict, so a UI that forgets If-Match fails this harness loudly.
function patchAutomation({ route, json }: MockRequest, existing: Automation) {
  if (route.request().headers()["if-match"] !== String(existing.version)) {
    return json(versionSkew, 409);
  }
  const body = route.request().postDataJSON();
  if (typeof body.name === "string") {
    existing.name = body.name;
  }
  if (body.params) {
    existing.params = body.params;
  }
  if (body.status === "enabled" || body.status === "paused") {
    existing.status = body.status;
  }
  existing.version += 1;
  return json(existing);
}

export const automationById: Handler = (request, state) => {
  const { route, path, method, json } = request;
  const id = path.slice("/automations/".length);
  const existing = state.automations.find((entry) => entry.id === id);
  if (!existing) {
    return json({ title: "Not Found" }, 404);
  }
  if (method === "PATCH") {
    return patchAutomation(request, existing);
  }
  if (method === "DELETE") {
    state.automations = state.automations.filter((entry) => entry.id !== id);
    return route.fulfill({ status: 204 });
  }
  return json(existing);
};
