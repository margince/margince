// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { CompaniesScreen, CompanyScreen } from "./organizations";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// CompaniesScreen (list) and CompanyScreen (360 Overview) both read through
// the api client on mount — fixtures mirror organizations.test.tsx's `org`
// plus the dormant-strength default the Overview tab always fires.
const meta: Meta = {
  title: "Records/Companies",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const org = {
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  industry: "Automotive",
  size_band: "201-500",
  domains: [{ domain: "brandt.example", is_primary: true }],
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  // The header states when the record was written, so the fixture carries the
  // date the contract requires of every organization — omitted, the identity
  // line formats an unreadable date and the whole page renders as nothing.
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

const dormantStrength = {
  score: 0,
  bucket: "none",
  factors: { recency: 0, frequency: 0, reciprocity: 0, direction: 0 },
  last_interaction: null,
};

// Confirmed profile fields (B5) and site-read facts (B6) — evidence-or-omit:
// each carries provenance + (optional) confidence + a grounding snippet.
const profileFields = [
  {
    field: "legal_name",
    value: "Brandt Automotive GmbH",
    source: "site_read",
    captured_by: "agent:capture",
    evidence_snippet: "Brandt Automotive GmbH, Stuttgart",
    source_url: "https://brandt.example/impressum",
    confidence: 0.95,
    updated_at: "2026-07-01T00:00:00Z",
  },
  {
    field: "value_proposition",
    value: "Fleet retrofits without downtime",
    source: "site_read",
    captured_by: "agent:capture",
    evidence_snippet: "We retrofit fleets without downtime",
    source_url: "https://brandt.example",
    confidence: 0.82,
    updated_at: "2026-07-01T00:00:00Z",
  },
];

const facts = [
  {
    category: "company",
    field: "founded_year",
    value: "1998",
    value_key: "founded_year:1998",
    source: "site_read",
    captured_by: "agent:capture",
    evidence_snippet: "Founded in 1998",
    source_url: "https://brandt.example/about",
    confidence: 0.9,
    updated_at: "2026-07-01T00:00:00Z",
  },
  {
    category: "offering",
    field: "service",
    value: "Fleet retrofits",
    value_key: "service:fleet-retrofits",
    source: "site_read",
    captured_by: "agent:capture",
    updated_at: "2026-07-01T00:00:00Z",
  },
  {
    category: "market",
    field: "served_industry",
    value: "Automotive OEMs",
    value_key: "served_industry:automotive-oems",
    source: "site_read",
    captured_by: "agent:capture",
    updated_at: "2026-07-01T00:00:00Z",
  },
];

// Every story below states its own ADDRESS. The company page reads its open
// tab off the route (useCompanyTab), so a story that left the hash alone would
// draw whichever tab the story before it happened to set.
export const CompaniesList: Story = {
  render: () => {
    globalThis.location.hash = "#/companies";
    installFetchStub({
      "GET /me": meRoute({ organization: ["read", "update"] }),
      "GET /organizations": () =>
        jsonResponse({
          data: [org],
          page: { next_cursor: null, has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <CompaniesScreen />
      </StoryProviders>
    );
  },
};

// The composite read that now serves the whole page. An account with a warm
// contact, an open deal and an overdue task — the state the view was
// designed around, rather than an empty record that shows only chrome.
const emptyPage = { has_more: false, next_cursor: null };

const org360 = {
  as_of: "2026-07-13T09:00:00Z",
  organization: org,
  sections_omitted: [],
  people: {
    data: [
      {
        person_id: "p-1",
        full_name: "Dana Buyer",
        title: "Head of Fleet",
        primary_email: "dana@brandt.example",
        deal_roles: [{ deal_id: "d-1", role: "champion" }],
        consent: { marketing_email: "granted" },
        strength: {
          score: 71,
          bucket: "strong",
          factors: {
            recency: 0.9,
            frequency: 0.6,
            reciprocity: 0.8,
            direction: 0.8,
          },
          last_interaction: "2026-07-10T09:00:00Z",
        },
      },
    ],
    page: emptyPage,
  },
  deals: {
    data: [
      {
        deal_id: "d-1",
        name: "Fleet retrofit 2026",
        status: "open",
        stage_name: "Proposal",
        amount: { amount_minor: 4_800_000, currency: "EUR" },
        stalled: false,
      },
    ],
    page: emptyPage,
    won_lifetime: { amount_minor: 12_000_000, currency: "EUR" },
    lost_count: 1,
  },
  strength: {
    score: 71,
    bucket: "strong",
    contact_count: 1,
    contributor_person_id: "p-1",
    factors: { recency: 0.9, frequency: 0.6, reciprocity: 0.8, direction: 0.8 },
    last_interaction: "2026-07-10T09:00:00Z",
  },
  activities: { data: [], page: emptyPage },
  next_steps: {
    data: [
      {
        activity_id: "a-1",
        subject: "Send the renewal paperwork",
        due_at: "2026-07-01T09:00:00Z",
        overdue: true,
        linked_deal_id: "d-1",
      },
    ],
    page: emptyPage,
  },
  pending_approvals: { data: [], page: emptyPage },
  tags: [{ id: "t-1", workspace_id: "w-1", name: "Key account" }],
  since_last_visit: {
    baseline_at: "2026-07-10T09:00:00Z",
    new_activities: 2,
    deal_stage_moves: 1,
    pending_proposals: 0,
  },
};

const rollup = {
  root_id: "o-1",
  scope: "tree",
  weighted_pipeline: { amount_minor: 2_400_000, currency: "EUR" },
  closed_won: { amount_minor: 12_000_000, currency: "EUR" },
  activity_count_30d: 8,
  aggregated_account_count: 1,
  restricted_excluded: [],
  computed_at: "2026-07-13T09:00:00Z",
};

const overviewRoutes = {
  "GET /organizations/o-1": () => jsonResponse(org),
  "GET /organizations/o-1/360": () => jsonResponse(org360),
  "GET /organizations/o-1/hierarchy-rollup": () => jsonResponse(rollup),
  "GET /organizations/o-1/brief": () =>
    jsonResponse({
      organization_id: "o-1",
      generated_at: "2026-07-13T09:00:00Z",
      generated_by: "model",
      sentences: [
        {
          text: "Fleet retrofit 2026 has been in Proposal since June and has not moved.",
          evidence: [{ entity_type: "deal", entity_id: "d-1" }],
        },
        {
          text: "Dana Buyer replied on 10 July after the board meeting.",
          evidence: [{ entity_type: "activity", entity_id: "a-9" }],
        },
      ],
    }),
  "GET /organizations/o-1/strength": () => jsonResponse(dormantStrength),
  "GET /activities": () => jsonResponse({ data: [] }),
  "GET /signals": () => jsonResponse({ data: [], page: emptyPage }),
  "GET /relationships": () => jsonResponse({ data: [], page: emptyPage }),
  "GET /records/organization/o-1/context": () =>
    jsonResponse({
      anchor: { type: "organization", id: "o-1" },
      sections: [],
    }),
};

// Populated 360: the firmographics/legal card and the facts card both carry
// site-read content, alongside the existing static firmographics dl.
export const CompanyOverview: Story = {
  render: () => {
    globalThis.location.hash = "#/companies/o-1";
    installFetchStub({
      "GET /me": meRoute({ organization: ["read", "update"] }),
      ...overviewRoutes,
      "GET /organizations/o-1/profile-fields": () =>
        jsonResponse({ data: profileFields }),
      "GET /organizations/o-1/facts": () => jsonResponse({ data: facts }),
    });
    return (
      <StoryProviders>
        <CompanyScreen id="o-1" />
      </StoryProviders>
    );
  },
};

// Nothing read yet: the profile card states its honest empty note and the
// facts card renders nothing at all.
export const CompanyOverviewEmpty: Story = {
  render: () => {
    globalThis.location.hash = "#/companies/o-1";
    installFetchStub({
      "GET /me": meRoute({ organization: ["read", "update"] }),
      ...overviewRoutes,
      "GET /organizations/o-1/profile-fields": () => jsonResponse({ data: [] }),
      "GET /organizations/o-1/facts": () => jsonResponse({ data: [] }),
    });
    return (
      <StoryProviders>
        <CompanyScreen id="o-1" />
      </StoryProviders>
    );
  },
};

// A rep whose role cannot read deals: the deals card SAYS so rather than
// drawing the empty state a reader would take for "this account has none".
export const CompanyOverviewWithheldSection: Story = {
  render: () => {
    globalThis.location.hash = "#/companies/o-1";
    installFetchStub({
      "GET /me": meRoute({ organization: ["read", "update"] }),
      ...overviewRoutes,
      "GET /organizations/o-1/360": () =>
        jsonResponse({
          ...org360,
          deals: undefined,
          sections_omitted: ["deals"],
        }),
      "GET /organizations/o-1/profile-fields": () => jsonResponse({ data: [] }),
      "GET /organizations/o-1/facts": () => jsonResponse({ data: [] }),
    });
    return (
      <StoryProviders>
        <CompanyScreen id="o-1" />
      </StoryProviders>
    );
  },
};

// The Deals tab with BOTH its panels: what the account is under contract for,
// and the pipeline under it. The two-panel state is the one worth drawing —
// the work column carries no interval of its own, so this is where a reader
// can see that the tab body takes the record's own rhythm rather than letting
// the two panels meet at the border.
//
// The tab comes off the ADDRESS (useCompanyTab), so the story sets it the way
// a reader arriving on a link would rather than by pressing the strip.
export const CompanyDeals: Story = {
  render: () => {
    globalThis.location.hash = "#/companies/o-1/deals";
    installFetchStub({
      "GET /me": meRoute({ organization: ["read", "update"] }),
      ...overviewRoutes,
      "GET /organizations/o-1/360": () =>
        jsonResponse({
          ...org360,
          // The contract slice is present only for a reader holding the
          // contract grant; without it the upper panel is not drawn at all and
          // the tab is back to a single card.
          state_strip: {
            contracts: {
              active_count: 2,
              priced_count: 2,
              annualized_value_minor_base: 9_600_000,
              base_currency: "EUR",
              nearest_renewal_on: "2027-01-31",
              cancellation_pending: false,
            },
          },
        }),
      "GET /organizations/o-1/profile-fields": () => jsonResponse({ data: [] }),
      "GET /organizations/o-1/facts": () => jsonResponse({ data: [] }),
    });
    return (
      <StoryProviders>
        <CompanyScreen id="o-1" />
      </StoryProviders>
    );
  },
};
