// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import {
  company360,
  companyBackstop,
  emptySection,
} from "../../testing/company.fixtures";
import { stubFetch } from "../../testing/company.testkit";
import { CompanyScreen } from "./companies";

type Company360 = components["schemas"]["Company360"];
type Company360Deal = components["schemas"]["Company360Deal"];
type Company360Project = components["schemas"]["Company360Project"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const DEAL: Company360Deal = {
  deal_id: "d-1",
  name: "Fleet retrofit",
  status: "open",
  stalled: false,
};

const PROJECT: Company360Project = {
  project_id: "p-1",
  name: "Depot rollout",
  phase: "delivering",
  quiet: false,
};

const ONE_DEAL: Company360["deals"] = {
  data: [DEAL],
  page: emptySection.page,
  won_lifetime: { amount_minor: 0, currency: "EUR" },
  lost_count: 0,
};

function renderWith(view: Company360) {
  stubFetch(async (url) => companyBackstop(url), { company360: view });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <CompanyScreen id="o-1" />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("CompanyScreen: what the tab strip counts", () => {
  it("counts the history from the server's total, past the page it sent", async () => {
    renderWith({
      ...company360,
      activities: {
        data: [],
        page: { has_more: true, next_cursor: "c1", total: 42 },
      },
    });

    expect(
      await screen.findByRole("button", { name: /^History\W+42$/ }),
    ).toBeTruthy();
  });

  it("counts deals and projects together while both lists are whole", async () => {
    renderWith({
      ...company360,
      deals: ONE_DEAL,
      projects: [PROJECT, { ...PROJECT, project_id: "p-2" }],
      projects_page: { has_more: false, next_cursor: null },
    });

    expect(
      await screen.findByRole("button", { name: /^Deals and projects\W+3$/ }),
    ).toBeTruthy();
  });

  it("draws no figure when the project list was cut short", async () => {
    renderWith({
      ...company360,
      deals: ONE_DEAL,
      projects: [PROJECT],
      projects_page: { has_more: true, next_cursor: "c1" },
    });

    expect(
      await screen.findByRole("button", { name: "Deals and projects" }),
    ).toBeTruthy();
  });
});
