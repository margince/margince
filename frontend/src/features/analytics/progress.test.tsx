// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { CONTEXTS } from "./analytics.overview.fixtures";
import { render } from "./analytics.testkit";
import { GoalEditor } from "./goals.editor";
import { ProgressScreen } from "./progress";
import { draftPlan, sellerMarch, teamMarch } from "./progress.fixtures";
import { equalMonths, monthsTotal } from "./progress.queries";
import { ProgressReadings, ProgressResult } from "./progress.result";
import { ProgressWork } from "./progress.work";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

describe("equalMonths", () => {
  it("gives the first months the minor units a division leaves over", () => {
    const months = equalMonths(1000001);
    expect(months.slice(0, 6)).toEqual(Array(5).fill(83334).concat(83333));
    expect(monthsTotal(months)).toBe(1000001);
  });

  it("spreads an amount below twelve units over the first months only", () => {
    expect(equalMonths(3)).toEqual([1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0]);
  });
});

describe("monthsTotal", () => {
  it("has no total while a month is not set", () => {
    const months: (number | null)[] = Array(12).fill(100);
    months[11] = null;
    expect(monthsTotal(months)).toBeNull();
  });
});

describe("ProgressResult", () => {
  it("shows a shortfall in the danger tone with the completion", () => {
    render(<ProgressResult progress={sellerMarch} onEvidence={vi.fn()} />);
    expect(screen.getByText("€60,000")).toBeTruthy();
    expect(screen.getByText("€40,000 below goal")).toBeTruthy();
    expect(screen.getAllByText("60% of the goal").length).toBeGreaterThan(0);
  });

  it("says a team goal is incomplete instead of comparing", () => {
    render(<ProgressResult progress={teamMarch} onEvidence={vi.fn()} />);
    expect(screen.queryByText(/below goal/)).toBeNull();
    expect(
      screen.getByText(
        "Goals incomplete: at least one seller has no monthly goal yet, €100,000 so far",
      ),
    ).toBeTruthy();
  });

  it("opens the closed deals as evidence", async () => {
    const onEvidence = vi.fn();
    const user = userEvent.setup();
    render(<ProgressResult progress={sellerMarch} onEvidence={onEvidence} />);
    await user.click(screen.getByRole("button", { name: "View closed deals" }));
    expect(onEvidence).toHaveBeenCalledWith({
      metric: "bookings_won",
      context_id: "interval",
    });
  });
});

describe("ProgressReadings", () => {
  it("names what sold and counts the deals no line could place", () => {
    render(<ProgressReadings progress={sellerMarch} onEvidence={vi.fn()} />);
    expect(
      screen.getByText("Fleet Suite, Telematics, Driver App and 1 more"),
    ).toBeTruthy();
    expect(screen.getByText("Unclassified")).toBeTruthy();
  });

  it("opens the sales behind a category, and shows no split while it is hidden", async () => {
    const onEvidence = vi.fn();
    const user = userEvent.setup();
    render(<ProgressReadings progress={sellerMarch} onEvidence={onEvidence} />);
    // The product card is the first of the readings.
    await user.click(screen.getAllByRole("button")[0] as HTMLElement);
    expect(onEvidence).toHaveBeenCalledWith(
      expect.objectContaining({ group_key: "kind:software" }),
    );
    cleanup();
    render(
      <ProgressReadings
        progress={{
          ...sellerMarch,
          categories: { ...sellerMarch.categories, status: "withheld" },
          planned: { status: "ok" },
        }}
        onEvidence={onEvidence}
      />,
    );
    expect(screen.queryByText("Unclassified")).toBeNull();
    expect(screen.getAllByText("Hidden").length).toBeGreaterThan(0);
  });
});

describe("GoalEditor", () => {
  it("lets a manager publish a saved draft that adds up", () => {
    render(<GoalEditor plan={draftPlan} />);
    expect(
      screen.getByRole("button", { name: "Publish" }).hasAttribute("disabled"),
    ).toBe(false);
  });

  it("asks to save first once a month changes", async () => {
    const user = userEvent.setup();
    render(<GoalEditor plan={draftPlan} />);
    const january = screen.getByLabelText("January");
    await user.clear(january);
    await user.type(january, "5");
    expect(screen.getByText("Save the draft first.")).toBeTruthy();
  });
});

// Routes the work lists by path and by page size: a one-row read is a count.
function stubWork(answer: (url: URL) => unknown) {
  vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
    const url = new URL(input instanceof Request ? input.url : String(input));
    return jsonResponse(answer(url));
  });
}

const lead = (id: string, name: string) => ({
  id,
  full_name: name,
  status: "new",
  score: 0,
  source: "manual",
  captured_by: "human:u1",
  version: 1,
  created_at: "2026-08-20T09:00:00Z",
  updated_at: "2026-08-20T09:00:00Z",
});

const openDeal = (id: string, name: string, close: string | null) => ({
  id,
  name,
  status: "open",
  stage_id: null,
  owner_id: "00000000-0000-4000-8000-0000000000a1",
  amount_minor: 4_200_000,
  currency: "EUR",
  expected_close_date: close,
  version: 1,
  created_at: "2026-01-10T09:00:00Z",
  updated_at: "2026-01-10T09:00:00Z",
});

describe("ProgressWork", () => {
  it("offers the next page of open leads while the list has more", async () => {
    let pages = 0;
    stubWork((url) => {
      if (
        !url.pathname.endsWith("/leads") ||
        url.searchParams.get("limit") === "1"
      )
        return { data: [], page: { has_more: false, total: 2 } };
      pages += 1;
      return {
        data: [lead(`l-${pages}`, `Lead ${pages}`)],
        page: { next_cursor: "next", has_more: pages === 1 },
      };
    });
    const user = userEvent.setup();
    render(<ProgressWork progress={sellerMarch} />);
    await user.click(await screen.findByRole("button", { name: "Show more" }));
    expect(await screen.findByText("Lead 2")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Show more" })).toBeNull();
  });

  it("counts the open leads in each status, and all of them on the tab", async () => {
    const totals: Record<string, number> = { new: 3, contacted: 2, engaged: 1 };
    stubWork((url) => ({
      data: [],
      page: {
        has_more: false,
        total: totals[url.searchParams.get("status") ?? ""],
      },
    }));
    render(<ProgressWork progress={sellerMarch} />);
    expect(
      await screen.findByRole("button", { name: /^Leads\W*6$/ }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: /^Contacted\W*2$/ }),
    ).toBeTruthy();
  });

  it("flags each open deal by what it sells, its close date and its next task", async () => {
    stubWork((url) => {
      if (url.pathname.endsWith("/analytics/progress/deals"))
        return {
          data: [
            {
              deal: openDeal("d1", "Fleet rollout", "2020-01-15"),
              sale_type: "product_and_service",
              next_task: { subject: "Send the offer", due_at: null },
            },
            {
              deal: openDeal("d2", "Depot pilot", "2099-06-01"),
              sale_type: "unclassified",
            },
            { deal: openDeal("d3", "Yard sensors", null) },
          ],
          page: { has_more: false },
        };
      return { data: [], page: { has_more: false } };
    });
    const user = userEvent.setup();
    render(<ProgressWork progress={sellerMarch} />);
    await user.click(
      await screen.findByRole("button", { name: /^Deals\W*3$/ }),
    );
    expect(await screen.findByText("Fleet rollout")).toBeTruthy();
    expect(screen.getByText("Product and service")).toBeTruthy();
    expect(screen.getByText("Overdue")).toBeTruthy();
    expect(screen.getByText("Send the offer")).toBeTruthy();
    expect(screen.getByText("Unclassified")).toBeTruthy();
    expect(screen.getByText("Outside the period")).toBeTruthy();
    expect(screen.getByText("No date")).toBeTruthy();
    expect(screen.getAllByText("No next task")).toHaveLength(2);
  });

  it("names each deal's owner on a team and hides an amount the reader may not see", async () => {
    const owner = "00000000-0000-4000-8000-0000000000a1";
    stubWork((url) => {
      if (url.pathname.endsWith("/analytics/progress/deals"))
        return {
          data: [
            {
              deal: {
                ...openDeal("d1", "Fleet rollout", "2026-03-20"),
                amount_minor: null,
                masked_fields: ["amount_minor"],
              },
              next_task: {
                subject: "Send the offer",
                due_at: "2026-03-24T09:00:00Z",
                assignee_id: "00000000-0000-4000-8000-0000000000a2",
              },
            },
            {
              deal: {
                ...openDeal("d2", "Depot pilot", "2026-03-25"),
                amount_minor: null,
              },
            },
          ],
          page: { has_more: false },
        };
      return { data: [], page: { has_more: false } };
    });
    const user = userEvent.setup();
    render(
      <ProgressWork
        progress={{ ...teamMarch, scope: { ...teamMarch.scope, id: owner } }}
      />,
    );
    await user.click(
      await screen.findByRole("button", { name: /^Deals\W*2$/ }),
    );
    expect(await screen.findByText("Fleet rollout")).toBeTruthy();
    expect(screen.getByRole("columnheader", { name: "Owner" })).toBeTruthy();
    expect(screen.getByText("Hidden")).toBeTruthy();
    expect(screen.getByText("No amount")).toBeTruthy();
    expect(screen.getByText("Send the offer")).toBeTruthy();
  });
});

describe("ProgressScreen", () => {
  it("reads the seller a team row opened, though the picker never listed them", async () => {
    const seller = "00000000-0000-4000-8000-0000000000b2";
    window.location.hash = `#/progress?scope=owner:${seller}`;
    installFetchStub({
      "GET /analytics/context": () => jsonResponse(CONTEXTS.leader),
      "GET /analytics/progress": () =>
        jsonResponse({
          ...sellerMarch,
          scope: { kind: "owner", id: seller, label: "Alex Brandt" },
        }),
    });
    const stubbed = globalThis.fetch;
    const asked: string[] = [];
    vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
      asked.push(input instanceof Request ? input.url : String(input));
      return stubbed(input, init);
    });
    render(<ProgressScreen />);
    await waitFor(() =>
      expect(
        asked.find((url) => url.includes("/analytics/progress")),
      ).toContain(`scope_id=${seller}`),
    );
    expect((await screen.findAllByText("Alex Brandt")).length).toBeGreaterThan(
      0,
    );
  });

  it("opens the seller's sales goals for the year being read", async () => {
    const seller = "00000000-0000-4000-8000-0000000000b2";
    window.location.hash = `#/progress?period=year&year=2026&scope=owner:${seller}`;
    installFetchStub({
      "GET /analytics/context": () => jsonResponse(CONTEXTS.leader),
      "GET /analytics/progress": () =>
        jsonResponse({
          ...sellerMarch,
          period: { kind: "year", year: 2026 },
          scope: { kind: "owner", id: seller, label: "Alex Brandt" },
        }),
    });
    const user = userEvent.setup();
    render(<ProgressScreen />);
    const links = await screen.findAllByRole("button", {
      name: "View sales goals",
    });
    await user.click(links[links.length - 1] as HTMLElement);
    expect(window.location.hash).toMatch(/^#\/goals\?/);
    expect(window.location.hash).toContain("year=2026");
    expect(window.location.hash).toContain(`seller=${seller}`);
  });
});
