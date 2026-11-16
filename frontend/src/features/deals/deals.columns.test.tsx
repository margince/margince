// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ListTable } from "../../design-system/listtable";
import { LocaleProvider, type Translator, useT } from "../../i18n";
import { companyNamesAnswer } from "../../testing/companynames.testkit";
import { phoneRowOf, stubWindowWidth } from "../../testing/phonerow.testkit";
import type { Deal } from "./deals";
import { dealColumns } from "./deals.columns";

function closeColumn() {
  const t: Translator = (key: string) => key;
  const column = dealColumns(t, "en", "UTC", new Map()).find(
    (candidate) => candidate.key === "close",
  );
  if (!column) {
    throw new Error("the deals table has no close column");
  }
  return column;
}

describe("the deals table close column", () => {
  const dates = {
    closed_at: "2026-08-20T12:00:00Z",
    expected_close_date: "2026-03-02",
  };
  const deal = (status: Deal["status"]): Deal => ({
    id: "d1",
    name: "Renewal",
    pipeline_id: "p1",
    stage_id: "s1",
    source: "manual",
    captured_by: "user:u1",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...dates,
    status,
  });

  it("sorts by the close day the cell shows", () => {
    expect(closeColumn().sort).toBe("close_day");
  });

  it("shows a closed deal's closed day, not its forecast", () => {
    const cell = closeColumn().cell(deal("won"));
    expect(cell).toBe("20/08/2026");
  });

  it("shows an open deal's forecast", () => {
    const cell = closeColumn().cell(deal("open"));
    expect(cell).toBe("02/03/2026");
  });
});

describe("a deal's row on a phone", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  const deal: Deal = {
    id: "d1",
    name: "Fleet retrofit",
    pipeline_id: "p1",
    stage_id: "s1",
    company_id: "c-1",
    amount_minor: 4_800_000,
    currency: "EUR",
    expected_close_date: "2026-03-02",
    status: "open",
    source: "manual",
    captured_by: "user:u1",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };

  function DealTable({ rows }: Readonly<{ rows: readonly Deal[] }>) {
    const t = useT();
    return (
      <ListTable
        rows={rows}
        columns={dealColumns(t, "en", "UTC", new Map([["s1", "Proposal"]]))}
        rowKey={(row) => row.id}
        unit="deals"
      />
    );
  }

  function renderTable(rows: readonly Deal[]) {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async (input: RequestInfo | URL) =>
          companyNamesAnswer(
            input instanceof Request ? input.url : String(input),
            { "c-1": "Acme GmbH" },
          ) ?? new Response("{}", { status: 404 }),
      ),
    );
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <DealTable rows={rows} />
        </LocaleProvider>
      </QueryClientProvider>,
    );
  }

  it("ends line one in the amount, then company, stage and close date", async () => {
    stubWindowWidth(390);
    renderTable([deal]);
    await screen.findByText("Acme GmbH");
    const row = await phoneRowOf("Fleet retrofit");
    expect(row.end).toEqual(["€48,000.00"]);
    expect(row.caption).toEqual(["Acme GmbH", "Proposal", "02/03/2026"]);
  });

  it("leaves out an amount, a company and a stage name it does not have", async () => {
    stubWindowWidth(390);
    renderTable([
      {
        ...deal,
        amount_minor: null,
        company_id: null,
        stage_id: "s-unnamed",
      },
    ]);
    const row = await phoneRowOf("Fleet retrofit");
    expect(row.end).toEqual([]);
    expect(row.caption).toEqual(["02/03/2026"]);
  });

  it("keeps a withheld amount and company as withheld, not as missing", async () => {
    stubWindowWidth(390);
    renderTable([
      {
        ...deal,
        amount_minor: null,
        company_id: null,
        masked_fields: ["amount_minor", "company_id"],
      },
    ]);
    const row = await phoneRowOf("Fleet retrofit");
    expect(row.end).toEqual(["••••"]);
    expect(row.caption).toEqual(["••••", "Proposal", "02/03/2026"]);
  });
});
