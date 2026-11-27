/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { meFixture, READS_RECORDS } from "../../testing/mefixture";
import { CompanyContractsCard } from "./companycontracts";

// What a contract row says about a recorded cancellation and a renewal that
// replaced it, read off the card a reader sees.

type Contract = components["schemas"]["Contract"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// Starts 01/01/2027, ends 31/12/2028, renews 01/01/2029; read before it starts.
const NOT_STARTED: Contract = {
  id: "c-1",
  company_id: "o-1",
  title: "Framework agreement",
  contract_number: "FA-1",
  source: "manual",
  captured_by: "human:u-1",
  status: "active",
  under_contract: false,
  term_state: "not_started",
  starts_on: "2027-01-01",
  ends_on: "2028-12-31",
  renewal_on: "2029-01-01",
  auto_renew: false,
  value_basis: "total",
  version: 3,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

function showCard(contracts: Contract[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = input instanceof Request ? input.url : String(input);
      const body = url.includes("/me")
        ? meFixture({ allow: { ...READS_RECORDS, contract: ["read"] } })
        : { data: url.includes("/contracts") ? contracts : [] };
      return Response.json(body);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <CompanyContractsCard companyId="o-1" />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("a contract row", () => {
  it("shows a cancellation recorded before the term starts, and no longer says it renews", async () => {
    showCard([
      {
        ...NOT_STARTED,
        cancellation_notice_on: "2027-06-01",
        cancellation_effective_on: "2028-12-31",
      },
    ]);

    expect(
      await screen.findByText(/^Canceled, ends .*2028, notice given .*2027$/),
    ).toBeTruthy();
    expect(screen.queryByText(/^Renews/)).toBeNull();
  });

  it("names the contract that replaced it, and no longer says it renews", async () => {
    const successor: Contract = {
      ...NOT_STARTED,
      id: "c-2",
      contract_number: "FA-2",
      starts_on: "2029-01-01",
      ends_on: null,
      renewal_on: null,
    };
    showCard([
      successor,
      { ...NOT_STARTED, status: "superseded", superseded_by_id: "c-2" },
    ]);

    expect(
      await screen.findByText("Replaced by Framework agreement, FA-2"),
    ).toBeTruthy();
    expect(screen.queryByText(/^Renews/)).toBeNull();
  });

  it("still says a successor replaced it when the card does not hold that row", async () => {
    showCard([
      { ...NOT_STARTED, status: "superseded", superseded_by_id: "c-9" },
    ]);

    expect(
      await screen.findByText(en["contracts.replacedByUnnamed"]),
    ).toBeTruthy();
  });

  it("says when an agreement with nothing recorded against it renews", async () => {
    showCard([NOT_STARTED]);

    expect(await screen.findByText(/^Renews .*2029$/)).toBeTruthy();
  });
});
