/** @vitest-environment happy-dom */
import { focusManager } from "@tanstack/react-query";
import { act, cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { company, company360 } from "../../testing/company.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  permissionDenied,
} from "../../testing/story-utils";
import { CompanyScreen } from "./companies";
import { render } from "./companies.testkit";

// The account warning on the real company page: what the finance summary
// says is overdue, worded for everyone who works the account.

afterEach(() => {
  cleanup();
  window.location.hash = "";
});

type FinanceSummary = components["schemas"]["CompanyFinanceSummary"];

const OVERDUE: FinanceSummary = {
  company_id: "o-1",
  state: "connected",
  provider: "offline_demo",
  open_balance: { amount_minor: 2000000, currency: "EUR" },
  overdue: { amount_minor: 1240000, currency: "EUR" },
  overdue_count: 3,
  oldest_overdue_days: 41,
  last_synced_at: "2026-11-02T09:00:00Z",
};

// Both reads the warning needs answer before the page settles: the account
// and its finance summary, or the refusal a role without finance read gets.
function mountCompany(
  finance: () => Response,
  grants: Parameters<typeof meRoute>[0] = {
    company: ["read"],
    finance: ["read"],
  },
): { asked: string[] } {
  const asked: string[] = [];
  installFetchStub({
    "GET /me": meRoute(grants),
    "GET /companies/o-1": () =>
      jsonResponse({ ...company, lifecycle: "customer" }),
    "GET /companies/o-1/360": () => jsonResponse(company360),
    "GET /companies/o-1/finance-summary": () => {
      asked.push("finance-summary");
      return finance();
    },
  });
  window.location.hash = "#/companies/o-1/overview";
  render(<CompanyScreen id="o-1" />);
  return { asked };
}

// No warning drawn, asserted only once the summary was read and the page
// stands: before that, an absent banner proves nothing.
async function expectNoWarning(asked: string[]) {
  await waitFor(() => expect(asked).toContain("finance-summary"));
  expect(await screen.findByRole("button", { name: "Overview" })).toBeTruthy();
  expect(screen.queryByText(/overdue on/)).toBeNull();
  expect(screen.queryByText(/€12,400/)).toBeNull();
}

describe("the company page's overdue warning", () => {
  it("states the overdue amount, the invoice count and the oldest age", async () => {
    mountCompany(() => jsonResponse(OVERDUE));

    expect(
      await screen.findByText("€12,400.00 overdue on 3 invoices"),
    ).toBeTruthy();
    expect(screen.getByText("The oldest is 41 days past due.")).toBeTruthy();
    expect(screen.queryByText(/reminder/)).toBeNull();
  });

  it("adds the reminders the accounting source says it sent", async () => {
    mountCompany(() => jsonResponse({ ...OVERDUE, dunning_level: 3 }));

    expect(
      await screen.findByText(
        "The oldest is 41 days past due. Accounting has sent 3 reminders.",
      ),
    ).toBeTruthy();
  });

  it.each(["stale", "error"] as const)(
    "dates a figure from a %s sync and says it in the past",
    async (state) => {
      mountCompany(() => jsonResponse({ ...OVERDUE, state }));

      expect(
        await screen.findByText("€12,400.00 was overdue on 3 invoices"),
      ).toBeTruthy();
      expect(
        screen.getByText(
          /^As of the last accounting sync on .*2026.*\. The oldest is 41 days past due\.$/,
        ),
      ).toBeTruthy();
      expect(screen.queryByText("€12,400.00 overdue on 3 invoices")).toBeNull();
    },
  );

  it("draws nothing for a stale figure with no sync to date it", async () => {
    const { asked } = mountCompany(() =>
      jsonResponse({ ...OVERDUE, state: "stale", last_synced_at: null }),
    );

    await expectNoWarning(asked);
  });

  it("names the amount alone when the server sends no count", async () => {
    mountCompany(() =>
      jsonResponse({
        ...OVERDUE,
        overdue_count: undefined,
        oldest_overdue_days: undefined,
      }),
    );

    expect(await screen.findByText("€12,400.00 overdue")).toBeTruthy();
  });

  it("goes once the next sync finds the invoices paid", async () => {
    let summary: FinanceSummary = { ...OVERDUE, state: "stale" };
    const { asked } = mountCompany(() => jsonResponse(summary));
    expect(
      await screen.findByText("€12,400.00 was overdue on 3 invoices"),
    ).toBeTruthy();

    summary = {
      ...OVERDUE,
      overdue: { amount_minor: 0, currency: "EUR" },
      overdue_count: 0,
      oldest_overdue_days: undefined,
      last_synced_at: "2026-11-03T09:00:00Z",
    };
    const reads = asked.length;
    act(() => {
      focusManager.setFocused(false);
      focusManager.setFocused(true);
    });
    await waitFor(() => expect(asked.length).toBeGreaterThan(reads));
    await waitFor(() => expect(screen.queryByText(/was overdue/)).toBeNull());
    focusManager.setFocused(undefined);
  });

  it("draws nothing when nothing is overdue", async () => {
    const { asked } = mountCompany(() =>
      jsonResponse({
        ...OVERDUE,
        overdue: { amount_minor: 0, currency: "EUR" },
        overdue_count: 0,
        oldest_overdue_days: undefined,
      }),
    );

    await expectNoWarning(asked);
  });

  it("draws nothing for a role without finance read", async () => {
    const { asked } = mountCompany(permissionDenied, { company: ["read"] });

    await expectNoWarning(asked);
  });
});
