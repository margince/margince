// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { derivationFixture, reportFixtures } from "./fixtures";
import { type Handler, page } from "./server";

// A contact found only through the company the word named, and only for a
// caller that asked for employees, as the server answers.
const jonasThroughBrandt = {
  type: "contact",
  id: "p-jonas",
  title: "Jonas Weiß",
  score: 0.5,
  works_at: { company_id: "o-brandt", company_name: "Brandt Automotive" },
};

// Cross-object: the relink picker finds projects by name beside the seeded
// contact. One hit of every other kind rides along, because the results
// screen groups and filters by kind and is swept for axe and at 390px.
export const search: Handler = ({ url, json }, { projectState }) => {
  const q = (url.searchParams.get("q") ?? "").toLowerCase();
  const projectHits = projectState.projects
    .filter((project) => project.name.toLowerCase().includes(q))
    .map((project) => ({
      type: "project",
      id: project.id,
      title: project.name,
      score: 0.8,
    }));
  const employees =
    q.includes("brandt") && url.searchParams.get("with_employees") === "true";
  const hits = [
    { type: "contact", id: "p-anna", title: "Anna Weber", score: 0.9 },
    ...projectHits,
    {
      type: "company",
      id: "o-brandt",
      title: "Brandt Automotive",
      score: 0.86,
    },
    ...(employees ? [jonasThroughBrandt] : []),
    { type: "deal", id: "d-fleet", title: "Fleet renewal", score: 0.8 },
    {
      type: "product",
      id: "pr-1",
      title: "Diagnostic audit",
      snippet: "GR-AUDIT",
      score: 0.7,
    },
    {
      type: "offer_template",
      id: "ot-1",
      title: "Fleet renewal quote",
      score: 0.66,
    },
    { type: "lead", id: "l-1", title: "Bettina Krause", score: 0.6 },
    { type: "tag", id: "t-1", title: "Key account", carried_by: 4 },
  ];
  // The kind pills narrow on the wire, as the server does.
  const wanted = url.searchParams.getAll("types");
  return json(
    page(
      wanted.length === 0
        ? hits
        : hits.filter((hit) => wanted.includes(hit.type)),
    ),
  );
};

// The frame every Analytics answer sits in. Without it the screen's context
// query throws and the route renders the error boundary, shell included. One
// scope, the default, so no population picker is drawn.
export const analyticsContext = {
  default_scope: { kind: "workspace", label: "Everyone" },
  allowed_scopes: [{ kind: "workspace", label: "Everyone" }],
  capabilities: {
    view_manager_forecast: true,
    submit_manager_forecast: true,
  },
  as_of: "2026-03-04T09:00:00Z",
  timezone: "Europe/Berlin",
  base_currency: "EUR",
};

// Read by the Brief's readings and the Analytics forecast tab. Without a
// currency, formatMoneyCompact takes the whole Home shell down.
export const forecast = {
  period_start: "2026-04-01",
  period_end: "2026-06-30",
  scope_kind: "workspace",
  won_minor: 4_000_000,
  evidence_minor: 12_000_000,
  best_case_minor: 18_000_000,
  open_minor: 25_000_000,
  weighted_minor: 13_000_000,
  eligible_count: 12,
  priced_count: 12,
  confirmed_date_count: 10,
  fx_missing_count: 0,
  as_of: "2026-07-13T00:00:00Z",
  timezone: "Europe/Berlin",
  base_currency: "EUR",
};

// Refused as the server refuses it: a grouping dimension the request names
// without binding, as a value or as `isnull`, explains no one cell.
export const reportDerivation: Handler = ({ url, json }) => {
  const unset = url.searchParams.getAll("isnull");
  const unbound = url.searchParams
    .getAll("by")
    .filter((dim) => !url.searchParams.has(dim) && !unset.includes(dim));
  return unbound.length > 0
    ? json({ code: "report_field_not_allowed", status: 422 }, 422)
    : json(derivationFixture);
};

// The board's own report. The plan echoes the client's grouping by stage and
// currency, and the aliases are the request's: the board reads `row.deals`.
// The weighted total is the server's per-deal-rounded figure.
const dealsByStage = {
  report: "deals-by-stage",
  plan: { group_by: ["stage_id", "currency"] },
  columns: ["stage_id", "currency", "raw_minor", "weighted_minor", "deals"],
  rows: [
    {
      stage_id: "s1",
      raw_minor: 1_250_000,
      weighted_minor: 250_000,
      deals: 1,
      currency: "EUR",
    },
    {
      stage_id: "s2",
      raw_minor: 4_800_000,
      weighted_minor: 1_920_000,
      deals: 1,
      currency: "EUR",
    },
    // A second currency in a stage that has one. The board must show this
    // column's count and refuse its total.
    {
      stage_id: "s2",
      raw_minor: 22_000_000_000,
      weighted_minor: 8_800_000_000,
      deals: 1,
      currency: "VND",
    },
  ],
};

// The report a caller asked for, so each Delivery section gets rows with the
// fields it reads.
export const report: Handler = ({ path, json }) =>
  json(reportFixtures[path.slice("/reports/".length)] || dealsByStage);

export const hierarchyRollup = {
  root_id: "o-brandt",
  scope: "tree",
  weighted_pipeline: { amount_minor: 0, currency: "EUR" },
  closed_won: { amount_minor: 0, currency: "EUR" },
  activity_count_30d: 0,
  aggregated_account_count: 1,
  restricted_excluded: [],
  computed_at: "2026-07-13T00:00:00Z",
};

// The Brief digest card destructures a MorningDigest.
export const digest = {
  date: "2026-07-13",
  generated_at: "2026-07-13T05:00:00Z",
  capture: {
    messages_synced: 24,
    activities_created: 18,
    contacts_created: 3,
    companies_created: 1,
  },
  review: {
    dedupe_open: 2,
    approvals_pending: 1,
    classify: { commitments: 4, meetings: 2, noise: 9 },
  },
  connectors: [
    {
      provider: "gmail",
      status: "connected",
      last_synced_at: "2026-07-13T04:55:00Z",
      last_sync_error_class: null,
    },
  ],
};
