// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { type Locale, LocaleProvider } from "../../i18n";
import { DealsCard } from "./company360";

type Company360Deal = components["schemas"]["Company360Deal"];

afterEach(cleanup);

const DEALS: Company360Deal[] = [
  {
    deal_id: "d-1",
    name: "Fleet retrofit",
    status: "open",
    stage_name: "Solution Discovery",
    amount: { amount_minor: 4_800_000, currency: "EUR" },
    expected_close_date: "2026-09-30",
    stalled: true,
  },
  {
    deal_id: "d-2",
    name: "Phase one licence",
    status: "won",
    stalled: false,
  },
];

function renderCard(deals: Company360Deal[], locale: Locale = "en") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>
        <DealsCard
          view={{
            as_of: "2026-08-13T09:00:00Z",
            company: {
              id: "o-1",
              display_name: "Brandt Automotive GmbH",
              source: "manual",
              captured_by: "human:u-1",
              created_at: "2026-06-01T08:00:00Z",
              updated_at: "2026-06-01T08:00:00Z",
            },
            sections_omitted: [],
            deals: {
              data: deals,
              page: { has_more: false, next_cursor: null },
              won_lifetime: { amount_minor: 450_000, currency: "EUR" },
              lost_count: 1,
            },
          }}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

function rowOf(name: string): HTMLElement {
  const row = screen.getByRole("button", { name }).closest("tr");
  if (!row) {
    throw new Error(`no table row holds ${name}`);
  }
  return row;
}

describe("DealsCard: the account's deals as one table", () => {
  it("reads each deal's stage, value, close day and status off its 360 row", () => {
    renderCard(DEALS);

    const open = within(rowOf("Fleet retrofit"));
    expect(open.getByText("Solution Discovery")).toBeTruthy();
    expect(open.getByText("€48,000.00")).toBeTruthy();
    expect(open.getByText("30 Sept")).toBeTruthy();
    expect(open.getByText("Open")).toBeTruthy();
    expect(open.getByText("Stalled")).toBeTruthy();
  });

  it("leaves a cell empty where the row carries no amount or close day", () => {
    renderCard(DEALS);

    const won = within(rowOf("Phase one licence"));
    expect(won.getByText("Won")).toBeTruthy();
    expect(won.getByText("No stage")).toBeTruthy();
    expect(won.queryByText(/€/)).toBeNull();
  });

  it("names each status in the reader's language", () => {
    renderCard(DEALS, "vi");

    expect(within(rowOf("Fleet retrofit")).getByText("Đang mở")).toBeTruthy();
    expect(within(rowOf("Phase one licence")).getByText("Thắng")).toBeTruthy();
  });

  it("draws no table for an account with no deals", () => {
    renderCard([]);

    expect(screen.queryByRole("table")).toBeNull();
    expect(screen.getByText("No open deals for this company.")).toBeTruthy();
  });
});
