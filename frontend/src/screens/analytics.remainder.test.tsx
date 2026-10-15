/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AnalyticsScreen } from "./analytics";
import { render, reportsStub } from "./analytics.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const companyRows = [
  { company_id: "o1", raw_minor: 250000, deal_count: 4, currency: "EUR" },
  { company_id: "o2", raw_minor: 90000, deal_count: 2, currency: "EUR" },
];

async function reportCard(title: string) {
  await userEvent
    .setup()
    .click(await screen.findByRole("button", { name: "Pipeline" }));
  return within(await screen.findByRole("region", { name: title }));
}

describe("a report the server capped", () => {
  it("keeps its table and names how many groups it left out", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        companyRows,
        totalRows: { "open-deals-per-company": 1250 },
      }),
    );
    render(<AnalyticsScreen />);
    const card = await reportCard("Open deals per company");
    expect(await card.findByText("o1")).toBeTruthy();
    expect(card.getByText("o2")).toBeTruthy();
    expect(card.getByText("1,248 more not shown")).toBeTruthy();
  });

  it("opens no company's deals, since a currency row may lie past the cap", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        companyRows,
        totalRows: { "open-deals-per-company": 3 },
      }),
    );
    render(<AnalyticsScreen />);
    const card = await reportCard("Open deals per company");
    expect(await card.findByText("4")).toBeTruthy();
    expect(card.getByText("2")).toBeTruthy();
    expect(card.queryByRole("link", { name: "4" })).toBeNull();
    expect(card.queryByRole("link", { name: "2" })).toBeNull();
  });
});

describe("a report the server answered in full", () => {
  it("states no remainder and keeps each company's door", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        companyRows,
        totalRows: { "open-deals-per-company": 2 },
      }),
    );
    render(<AnalyticsScreen />);
    const card = await reportCard("Open deals per company");
    expect(await card.findByRole("link", { name: "4" })).toBeTruthy();
    expect(card.queryByText(/more not shown/)).toBeNull();
    expect(card.queryByText("Showing part of the list")).toBeNull();
  });

  it("states no remainder when the server sends no total", async () => {
    vi.stubGlobal("fetch", reportsStub({ companyRows }));
    render(<AnalyticsScreen />);
    const card = await reportCard("Open deals per company");
    expect(await card.findByText("4")).toBeTruthy();
    expect(card.queryByText(/more not shown/)).toBeNull();
  });

  it("draws an empty forecast as its category slots, not as a none", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({ forecastRows: [], totalRows: { forecast: 0 } }),
    );
    render(<AnalyticsScreen />);
    const card = await reportCard("Forecast categories");
    expect(await card.findByText("Commit")).toBeTruthy();
    expect(card.queryByText(/more not shown/)).toBeNull();
  });
});
