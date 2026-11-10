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
        "Goals incomplete: some months have no goal for every seller, €100,000 so far",
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

describe("ProgressWork", () => {
  it("offers the next page of open leads while the list has more", async () => {
    const asked: string[] = [];
    installFetchStub({
      "GET /leads": () => {
        asked.push("leads");
        return jsonResponse({
          data: [
            {
              id: `l-${asked.length}`,
              full_name: `Lead ${asked.length}`,
              status: "new",
              score: 0,
              source: "manual",
              captured_by: "human:u1",
              version: 1,
              created_at: "2026-08-20T09:00:00Z",
              updated_at: "2026-08-20T09:00:00Z",
            },
          ],
          page: { next_cursor: "next", has_more: asked.length === 1 },
        });
      },
    });
    const user = userEvent.setup();
    render(<ProgressWork progress={sellerMarch} />);
    await user.click(await screen.findByRole("button", { name: "Show more" }));
    expect(await screen.findByText("Lead 2")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Show more" })).toBeNull();
  });
});

describe("ProgressScreen", () => {
  it("reads the seller a team row opened, though the picker never listed them", async () => {
    const seller = "00000000-0000-4000-8000-0000000000b2";
    window.location.hash = `#/progress?scope=owner:${seller}`;
    installFetchStub({
      "GET /analytics/context": () => jsonResponse(CONTEXTS.leader),
      "GET /analytics/progress": () => jsonResponse(sellerMarch),
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
  });
});
