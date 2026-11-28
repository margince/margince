// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { seedWriter } from "../../testing/mefixture";
import { CompanyLeads } from "./companyleads";
import { COMPANY_LEADS_VIEW } from "./companyleads.fixtures";

afterEach(cleanup);
function show(
  view: components["schemas"]["Company360"] | undefined,
  loading = false,
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  seedWriter(client);
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <CompanyLeads companyId="company-1" view={view} loading={loading} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

it("opens linked leads and carries the company filter to the full list", () => {
  show(COMPANY_LEADS_VIEW);
  expect(
    screen.getByRole("link", { name: "Dana Buyer" }).getAttribute("href"),
  ).toBe("/#/leads/lead-1");
  const href = screen
    .getByRole("link", { name: en["co.leads.list"] })
    .getAttribute("href");
  expect(href).toContain("company_id=company-1");
  expect(href).toContain("#/leads");
});

it("names a partial page and links to more leads", () => {
  show({
    ...COMPANY_LEADS_VIEW,
    leads: {
      data: COMPANY_LEADS_VIEW.leads?.data ?? [],
      page: { has_more: true, next_cursor: "next-page" },
    },
  });
  expect(screen.getByText(en["state.partial"])).toBeInTheDocument();
  expect(
    screen.getByRole("link", { name: en["co.leads.more"] }),
  ).toBeInTheDocument();
});

it("distinguishes no linked leads from a withheld section", () => {
  const shown = show({
    ...COMPANY_LEADS_VIEW,
    leads: { data: [], page: { has_more: false, next_cursor: null } },
  });
  expect(screen.getByText(en["co.leads.empty"])).toBeInTheDocument();
  shown.unmount();
  show({
    ...COMPANY_LEADS_VIEW,
    leads: undefined,
    sections_omitted: ["leads"],
  });
  expect(screen.queryByText(en["co.leads.empty"])).not.toBeInTheDocument();
  expect(screen.getByText(en["state.withheld"])).toBeInTheDocument();
  expect(screen.queryByRole("link")).not.toBeInTheDocument();
});

it("keeps a pending or missing section from claiming there are no leads", () => {
  const shown = show(undefined, true);
  expect(screen.queryByText(en["co.leads.empty"])).not.toBeInTheDocument();
  shown.unmount();
  show({ ...COMPANY_LEADS_VIEW, leads: undefined });
  expect(screen.getByText(en["state.unavailable"])).toBeInTheDocument();
});
