// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The shapes the Questions stories and suites answer with, in one place so a
// story and a test cannot come to model two different servers.

import type {
  AnalyticsAnswer,
  AnalyticsEntity,
  AnalyticsQuery,
} from "./analytics.questions.vocab";

export const ENTITIES: AnalyticsEntity[] = [
  {
    name: "deals-by-stage",
    group_by: [
      "currency",
      "partner_company_id",
      "pipeline_id",
      "stage_id",
      "status",
      "win_probability",
    ],
    measures: ["amount_minor", "weighted_amount_minor"],
  },
  {
    name: "leads-by-status",
    group_by: ["owner_id", "source", "status"],
    measures: [],
  },
  {
    name: "meeting-conversion",
    group_by: ["became_opportunity", "host_user_id"],
    measures: [],
  },
];

export const SCHEMA = { version: "v1", entities: ENTITIES };

export const CONTEXT = {
  default_scope: { kind: "workspace" as const, label: "Whole company" },
  allowed_scopes: [{ kind: "workspace" as const, label: "Whole company" }],
  capabilities: {
    view_manager_forecast: true,
    submit_manager_forecast: true,
  },
  as_of: "2026-09-04T00:00:00Z",
  timezone: "Europe/Berlin",
  base_currency: "EUR",
};

export const STAGES = {
  data: [
    { id: "s-qual", name: "Qualified", pipeline_id: "pl", position: 1 },
    { id: "s-prop", name: "Proposal", pipeline_id: "pl", position: 2 },
  ],
};

export const USERS = {
  data: [
    { id: "u-1", display_name: "Ann Lee" },
    { id: "u-2", display_name: "Bruno Sá" },
  ],
  page: { next_cursor: null, has_more: false },
};

export const QUERY: AnalyticsQuery = {
  entity: "deals-by-stage",
  scope_kind: "workspace",
  group_by: ["stage_id", "currency"],
  measures: [{ fn: "count" }, { fn: "sum", field: "amount_minor" }],
  limit: 100,
};

export const ANSWER: AnalyticsAnswer = {
  columns: ["stage_id", "currency", "count", "sum_amount_minor"],
  rows: [
    {
      stage_id: "s-qual",
      currency: "EUR",
      count: 8,
      sum_amount_minor: 86_200_000,
      _withheld: false,
    },
    {
      stage_id: "s-prop",
      currency: "EUR",
      count: 10,
      sum_amount_minor: 908_834_000,
      _withheld: false,
    },
    {
      stage_id: null,
      currency: "USD",
      count: 6,
      sum_amount_minor: 12_500_000,
      _withheld: false,
    },
  ],
  withheld: false,
  total_safe: true,
  schema_version: "v1",
};

// Groups the floor kept back: every column null, each row still there, and
// scattered among the served ones the way the engine returns them.
const WITHHELD_ROW = {
  stage_id: null,
  currency: null,
  count: null,
  sum_amount_minor: null,
  _withheld: true,
};

export const WITHHELD_ANSWER: AnalyticsAnswer = {
  ...ANSWER,
  rows: [
    WITHHELD_ROW,
    ANSWER.rows[0],
    WITHHELD_ROW,
    ANSWER.rows[1],
    WITHHELD_ROW,
  ],
  withheld: true,
  total_safe: false,
};

export const EMPTY_ANSWER: AnalyticsAnswer = { ...ANSWER, rows: [] };

// Each record named by the server under the reader's grants, `label` last.
export const EXPLANATION = {
  columns: ["id", "stage_id", "currency", "amount_minor", "label"],
  rows: [
    {
      id: "d-1",
      stage_id: "s-qual",
      currency: "EUR",
      amount_minor: 4_200_000,
      label: "Fleet retrofit",
    },
    {
      id: "d-2",
      stage_id: "s-qual",
      currency: "EUR",
      amount_minor: 4_420_000,
      label: "Line QA rollout",
    },
  ],
  withheld: false,
  truncated: false,
};

/** A refusal as the engine answers it, in each of its kinds. */
export function refusal(kind: string, message: string, suggest: string) {
  return {
    type: "about:blank",
    title: "Bad Request",
    status: 400,
    code: "invalid_argument",
    detail: `${kind}: ${message} — ${suggest}`,
    details: { kind, message, suggest },
  };
}
