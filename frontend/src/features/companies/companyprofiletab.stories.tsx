// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "../../testing/story-utils";
import { CompanyProfileTab } from "./companies.profile";
import { CompanyProfileForm } from "./companyprofiletab";
import "./company360.css";
import "./companytabs.css";

type Company = components["schemas"]["Company"];
type ProfileField = components["schemas"]["CompanyProfileField"];

// The Profile tab's own body, in the two states that differ by WRITE STANDING:
// every field editable, and the same fields with the verbs gone and one
// sentence at the top saying why. That sentence is the tab's read-only notice —
// it carries the id every refused control below points at, so the notice's
// heading IS the description rather than a second copy of it.

const COMPANY = "01a04298-1971-7076-8076-8064da20fdff";

const company: Company = {
  writable: true,
  id: COMPANY,
  workspace_id: "01a04298-1971-7076-8076-8064da20fd01",
  display_name: "Brandt Automotive GmbH",
  legal_name: "Brandt Automotive GmbH",
  status: "customer",
  owner_id: "01a04298-1971-7076-8076-8064da20fd02",
  industry: "automotive",
  size_band: "51-200",
  domains: [
    {
      id: "dom-1",
      domain: "brandt.example",
      is_primary: true,
      source: "manual",
      captured_by: "human:u1",
    },
  ],
  description: "Fleet electrification pilot, renewing in Q3.",
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

function frame(
  company: Company,
  allow: Parameters<typeof meRoute>[0],
  fields: ProfileField[] = [],
  readFinished = false,
) {
  const routes: RouteMap = {
    "GET /me": meRoute(allow),
    [`GET /companies/${COMPANY}/profile-fields`]: () =>
      jsonResponse({ data: fields }),
    [`GET /companies/${COMPANY}/facts`]: () => jsonResponse({ data: [] }),
    [`GET /companies/${COMPANY}/site-reads/latest`]: () =>
      readFinished
        ? jsonResponse({
            read_id: "read-profile",
            company_id: COMPANY,
            seed_url: "https://brandt.example",
            status: "done",
            status_detail: null,
            pages: [],
            skipped: [],
            proposal_ids: [],
            created_at: "2026-06-01T08:00:00Z",
          })
        : new Response(null, { status: 204 }),
  };
  installFetchStub(routes);
  return (
    <StoryProviders>
      <CompanyProfileForm
        company={company}
        onOpenReceipt={() => undefined}
        tools={null}
      />
    </StoryProviders>
  );
}

const WRITER = { company: ["read", "update"] } as const;

const meta: Meta<typeof CompanyProfileForm> = {
  title: "Records/Company 360/Profile tab",
  component: CompanyProfileForm,
};
export default meta;

type Story = StoryObj<typeof CompanyProfileForm>;

/** The ordinary tab: the account's own fields, every one of them editable. */
export const Editable: Story = { render: () => frame(company, WRITER) };

const OFFER: ProfileField = {
  id: "pf-offer",
  field: "offer_summary",
  value: "Charging systems for commercial electric fleets.",
  source: "site_read",
  captured_by: "agent:site-read",
  evidence_snippet:
    "We supply charging systems for commercial electric fleets.",
  source_url: "https://brandt.example/solutions",
  updated_at: "2026-06-01T08:00:00Z",
  version: 1,
};

export const DuplicateOfferDescription: Story = {
  render: () =>
    frame({ ...company, description: OFFER.value }, WRITER, [OFFER]),
};

export const DistinctDescription: Story = {
  render: () => frame(company, WRITER, [OFFER]),
};

export const EmptyFieldsAfterWebsiteRead: Story = {
  render: () => frame(company, WRITER, [OFFER], true),
};

/**
 * Archived, which is the more specific of the two denials and the one a reader
 * can act on. The notice is the tab's single explanation, and it is the state
 * the callout's `standing` kind exists for: it is true as the page renders and
 * has nothing to interrupt for.
 */
export const ReadOnlyBecauseArchived: Story = {
  render: () =>
    frame({ ...company, archived_at: "2026-07-15T00:00:00Z" }, WRITER),
};

/** The same notice in dark, because tone reaches the heading's ink. */
export const ReadOnlyBecauseArchivedDark: Story = {
  globals: { theme: "dark" },
  render: () =>
    frame({ ...company, archived_at: "2026-07-15T00:00:00Z" }, WRITER),
};

export const CompleteProfileContext: Story = {
  render() {
    installFetchStub({
      "GET /me": meRoute(WRITER),
      [`GET /companies/${COMPANY}/profile-fields`]: () =>
        jsonResponse({ data: [] }),
      [`GET /companies/${COMPANY}/facts`]: () => jsonResponse({ data: [] }),
      [`GET /companies/${COMPANY}/site-reads/latest`]: () =>
        new Response(null, { status: 204 }),
      [`GET /companies/${COMPANY}/technical-enrich/latest`]: () =>
        jsonResponse({ lanes: [] }),
      "GET /relationships": () =>
        jsonResponse({
          data: [],
          page: { has_more: false, next_cursor: null },
        }),
      [`GET /companies/${COMPANY}/hierarchy-rollup`]: () =>
        jsonResponse({
          root_id: COMPANY,
          scope: "tree",
          weighted_pipeline: { amount_minor: 0, currency: "EUR" },
          closed_won: { amount_minor: 0, currency: "EUR" },
          activity_count_30d: 0,
          aggregated_account_count: 1,
          restricted_excluded: [],
          computed_at: "2026-07-13T09:00:00Z",
          pipeline_state_at: "2026-07-13T09:00:00Z",
          closed_won_period: {
            grain: "quarter",
            label: "2026-Q3",
            start_at: "2026-07-01T00:00:00Z",
            end_at: "2026-10-01T00:00:00Z",
            anchor: "closed_at",
            chosen: false,
            previous: "2026-Q2",
            next: null,
          },
          activity_window: {
            start_at: "2026-07-13T09:00:00Z",
            end_at: "2026-07-13T09:00:00Z",
          },
          current_periods: {
            period_year: "2026",
            period_quarter: "2026-Q3",
            period_month: "2026-07",
          },
        }),
    });
    return (
      <StoryProviders>
        <CompanyProfileTab
          active
          company={company}
          onOpenHistory={() => undefined}
          onOpenReceipt={() => undefined}
        />
      </StoryProviders>
    );
  },
};
