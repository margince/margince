// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import type { Locale } from "../../i18n";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { LeadScreen } from "./leads";
import { SIGNAL_CATALOG } from "./leadsignals.fixture";

// The linked company's facts sit in a group named for that company. While the
// company holds no industry, the lead's own industry stands in.

type Lead = components["schemas"]["Lead"];

const meta: Meta = {
  title: "Records/Leads/Details",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const base: Lead = {
  id: "l-1",
  full_name: "Jonas Petersen",
  email: "jonas@nordwind.example",
  company_name: "Nordwind Logistik",
  status: "contacted",
  score: 72,
  source: "manual",
  captured_by: "human:u1",
  writable: true,
  version: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const linked: Lead = {
  ...base,
  company_id: "c-7",
  company_facts: {
    industry: "logistics_transport",
    country: "DE",
    size_band: "51-200",
  },
};

function page(lead: Lead, locale: Locale = "en", companyName?: string) {
  return () => {
    installFetchStub({
      "GET /lead-signal-catalog": () => jsonResponse(SIGNAL_CATALOG),
      "GET /leads/l-1": () => jsonResponse(lead),
      "GET /companies/c-7": () =>
        jsonResponse({
          id: "c-7",
          display_name: companyName ?? "Nordwind Holding SE",
        }),
      "GET /me": meRoute(
        { lead: ["read", "update"], company: ["read"] },
        { roles: ["rep"] },
      ),
    });
    return (
      <StoryProviders locale={locale}>
        <LeadScreen id="l-1" />
      </StoryProviders>
    );
  };
}

/** A linked company that holds all three facts. */
export const CompanyFacts: Story = { render: page(linked) };

/** The company holds only its size, so the lead's own industry stays editable. */
export const SomeFactsMissing: Story = {
  render: page({
    ...linked,
    industry: "software",
    company_facts: { industry: null, country: null, size_band: "1-10" },
  }),
};

/** No linked company: the industry a rep typed. */
export const TypedIndustry: Story = {
  render: page({ ...base, industry: "manufacturing" }),
};

/** A reader who may not open the company gets no facts from it. */
export const NoCompanyRead: Story = {
  render: page({ ...base, company_id: "c-7", industry: "software" }),
};

/** A company name long enough to wrap the group's heading. */
export const LongCompanyName: Story = {
  render: page(
    linked,
    "de",
    "Sindelfingen Werke für Fahrzeugtechnik und Systemintegration GmbH & Co. KG",
  ),
};

export const CompanyFactsGerman: Story = { render: page(linked, "de") };

export const CompanyFactsVietnamese: Story = { render: page(linked, "vi") };

export const CompanyFactsDark: Story = {
  globals: { theme: "dark" },
  render: page(linked),
};

export const CompanyFactsPhone: Story = {
  tags: ["uat-phone"],
  render: page(linked),
};
