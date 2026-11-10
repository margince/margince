// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { REPORTING_FIXTURE_ZONE } from "../../testing/reporting.fixtures";
import type {
  SalesGoalPlan,
  SalesProgress,
  SalesProgressMonth,
} from "./progress.queries";

const window2026 = (month: number) => ({
  start_at: new Date(Date.UTC(2026, month - 1, 1)).toISOString(),
  end_at: new Date(Date.UTC(2026, month, 1)).toISOString(),
});

const months: SalesProgressMonth[] = Array.from({ length: 12 }, (_, at) => {
  const month = at + 1;
  const started = month <= 3;
  return {
    month,
    window: window2026(month),
    state: started ? "ended" : "not_started",
    goal: {
      status: "set",
      amount_minor: 10000000,
      months_set: 1,
      months_needed: 1,
    },
    closed: started
      ? { status: "ok", amount_minor: 8000000 + month * 1000000 }
      : { status: "not_started" },
  };
});

/** A seller's March, a little short of a €100,000 goal. */
export const sellerMarch: SalesProgress = {
  scope: { kind: "owner", id: "0194f8a0-0000-7000-8000-000000000001" },
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
  scope: { kind: "team", id: "0194f8a0-0000-7000-8000-0000000000aa" },
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
