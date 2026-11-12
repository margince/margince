// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { deal } from "../../testing/brief.fixtures";
import { REPORTING_FIXTURE_ZONE } from "../../testing/reporting.fixtures";
import type { WorkDeal } from "./progress.deals";
import type {
  SalesGoalPlan,
  SalesGoalRoster,
  SalesProgress,
  SalesProgressMonth,
} from "./progress.queries";

const window2026 = (month: number) => ({
  start_at: new Date(Date.UTC(2026, month - 1, 1)).toISOString(),
  end_at: new Date(Date.UTC(2026, month, 1)).toISOString(),
});

// A seasonal year: goals in thousands of euros, closed revenue through March.
const GOALS = [80, 90, 100, 100, 110, 120, 90, 80, 110, 120, 130, 150];
const CLOSED = [72, 95, 60];

const months: SalesProgressMonth[] = GOALS.map((goal, at) => {
  const month = at + 1;
  const closed = CLOSED[at];
  const goalMinor = goal * 100000;
  return {
    month,
    window: window2026(month),
    state: closed === undefined ? "not_started" : "ended",
    goal: {
      status: "set",
      amount_minor: goalMinor,
      months_set: 1,
      months_needed: 1,
    },
    closed:
      closed === undefined
        ? { status: "not_started" }
        : { status: "ok", amount_minor: closed * 100000 },
    ...(closed === undefined
      ? {}
      : { delta_minor: closed * 100000 - goalMinor }),
  };
});

/** A seller's March, a little short of a €100,000 goal. */
export const sellerMarch: SalesProgress = {
  scope: {
    kind: "owner",
    id: "0194f8a0-0000-7000-8000-000000000001",
    label: "Alex Brandt",
  },
  period: { kind: "month", year: 2026, month: 3 },
  horizon: window2026(3),
  actual_window: window2026(3),
  cutoff: "2026-03-31T22:59:59Z",
  timezone: REPORTING_FIXTURE_ZONE,
  currency: "EUR",
  state: "ended",
  closed: { status: "ok", amount_minor: 6000000 },
  goal: {
    status: "set",
    amount_minor: 10000000,
    months_set: 1,
    months_needed: 1,
  },
  completion: 0.6,
  delta_minor: -4000000,
  categories: {
    status: "ok",
    product_minor: 4000000,
    service_minor: 1500000,
    unresolved_minor: 500000,
    product_items: ["Fleet Suite", "Telematics", "Driver App", "Route Planner"],
    service_items: ["Onboarding"],
    invalid_deals: 1,
  },
  planned: { status: "ok", amount_minor: 22000000 },
  planned_valued_at: "2026-03-31T22:59:59Z",
  months,
  members: [],
  members_withheld: false,
  capabilities: { write_goals: false },
};

/** A team's March with one seller still without a goal. */
export const teamMarch: SalesProgress = {
  ...sellerMarch,
  scope: {
    kind: "team",
    id: "0194f8a0-0000-7000-8000-0000000000aa",
    label: "Sales DACH",
  },
  goal: {
    status: "incomplete",
    amount_minor: 10000000,
    months_set: 0,
    months_needed: 1,
  },
  completion: undefined,
  delta_minor: undefined,
  members: [
    {
      owner_id: "0194f8a0-0000-7000-8000-000000000001",
      name: "Alex Brandt",
      goal: sellerMarch.goal,
      closed: sellerMarch.closed,
      delta_minor: sellerMarch.delta_minor,
      planned: sellerMarch.planned,
    },
    {
      owner_id: "0194f8a0-0000-7000-8000-000000000002",
      name: "Sam Okafor",
      goal: { status: "missing", months_set: 0, months_needed: 1 },
      closed: { status: "ok", amount_minor: 0 },
      planned: { status: "ok", amount_minor: 0 },
    },
  ],
};

/** A plan with a saved draft a manager may publish. */
export const draftPlan: SalesGoalPlan = {
  owner_id: "0194f8a0-0000-7000-8000-000000000001",
  owner_name: "Alex Brandt",
  year: 2026,
  currency: "EUR",
  state: "draft",
  version: 1,
  live_months: Array.from({ length: 12 }, () => null),
  draft: {
    revision: 1,
    months: Array.from({ length: 12 }, () => 10000000),
    declared_total_minor: 120000000,
    reason: "Agreed in the yearly review",
    author_name: "Dana Manager",
    created_at: "2026-01-05T09:00:00Z",
    origin: "manual",
  },
  history: [
    {
      revision: 1,
      kind: "draft",
      reason: "Agreed in the yearly review",
      author_name: "Dana Manager",
      created_at: "2026-01-05T09:00:00Z",
      total_minor: 120000000,
    },
  ],
  capabilities: { edit: true },
};

const ALEX = "0194f8a0-0000-7000-8000-000000000001";
const SAM = "0194f8a0-0000-7000-8000-000000000002";
const owned = (
  id: string,
  name: string,
  owner: string,
  close: string | null,
  amount: number,
) =>
  deal(id, name, {
    amount_minor: amount,
    owner_id: owner,
    expected_close_date: close,
    company_id: owner === ALEX ? "company-nordwind" : "company-acme",
  });

/** The open deals of the work list: each sale type, close flag and task state. */
export const workDeals: WorkDeal[] = [
  {
    deal: owned("d-1", "Fleet retrofit", ALEX, "2026-03-20", 8_400_000),
    sale_type: "product_and_service",
    next_task: {
      subject: "Send the revised offer",
      due_at: "2026-03-24T09:00:00Z",
      assignee_id: SAM,
    },
  },
  {
    deal: owned("d-2", "PIM rollout", SAM, "2026-02-27", 2_650_000),
    sale_type: "product",
    next_task: { subject: "Chase the signature", due_at: null },
  },
  {
    deal: owned("d-3", "Depot lighting", ALEX, "2026-06-15", 890_000),
    sale_type: "service",
  },
  {
    deal: owned("d-9", "Ostwind refit", SAM, null, 5_200_000),
    sale_type: "partly_classified",
    next_task: { subject: "Agree the scope", due_at: "2026-03-28T13:00:00Z" },
  },
  {
    deal: owned("d-10", "Cold store retrofit", ALEX, "2026-03-30", 3_400_000),
    sale_type: "unclassified",
  },
];

/** The sellers a manager plans for, one in each state. */
export const goalRoster: SalesGoalRoster = {
  year: 2026,
  currency: "EUR",
  data: [
    {
      owner_id: draftPlan.owner_id,
      name: "Alex Brandt",
      state: "draft",
      can_edit: true,
    },
    {
      owner_id: "0194f8a0-0000-7000-8000-000000000002",
      name: "Sam Okafor",
      state: "published",
      total_minor: 114000000,
      can_edit: true,
    },
    {
      owner_id: "0194f8a0-0000-7000-8000-000000000003",
      name: "Maya Lindqvist",
      state: "no_goal",
      can_edit: true,
    },
  ],
};

/** What a seller reads: their published, seasonal year. */
export const publishedPlan: SalesGoalPlan = {
  ...draftPlan,
  state: "published",
  live_months: GOALS.map((goal) => goal * 100000),
  draft: undefined,
  capabilities: { edit: false },
};
