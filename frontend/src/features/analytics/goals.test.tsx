// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { render } from "./analytics.testkit";
import { SalesGoalsScreen } from "./goals";
import { GoalEditor } from "./goals.editor";
import {
  draftPlan,
  goalRoster,
  publishedPlan,
  sellerMarch,
} from "./progress.fixtures";
import type { SalesGoalPlan } from "./progress.queries";

afterEach(() => {
  cleanup();
  window.location.hash = "";
});

const ALEX = draftPlan.owner_id;
const SAM = "0194f8a0-0000-7000-8000-000000000002";
const PLAN = `/analytics/sales-goals/${ALEX}/2026`;

// A plan a manager published and may still change.
const editablePublished: SalesGoalPlan = {
  ...publishedPlan,
  capabilities: { edit: true },
  published: draftPlan.draft,
};

type Sent = { method: string; path: string; body: unknown };

// Routes every write of one plan and records what each one sent.
function stubWrites(plan: SalesGoalPlan): Sent[] {
  const sent: Sent[] = [];
  const record = (method: string, path: string) => (body: unknown) => {
    sent.push({ method, path, body });
    return jsonResponse(plan);
  };
  installFetchStub({
    [`PUT ${PLAN}/draft`]: record("PUT", "draft"),
    [`DELETE ${PLAN}/draft`]: record("DELETE", "draft"),
    [`POST ${PLAN}/publish`]: record("POST", "publish"),
    [`POST ${PLAN}/withdraw`]: record("POST", "withdraw"),
    [`GET ${PLAN}`]: () => jsonResponse(plan),
  });
  return sent;
}

describe("SalesGoalsScreen", () => {
  it("lists the sellers beside the plan of the first, with its history", async () => {
    window.location.hash = "#/goals?year=2026";
    installFetchStub({
      "GET /analytics/sales-goals": () => jsonResponse(goalRoster),
      [`GET ${PLAN}`]: () => jsonResponse(draftPlan),
    });
    render(<SalesGoalsScreen />);
    const roster = await screen.findByRole("table", { name: "Team members" });
    expect(within(roster).getByText("Sam Okafor")).toBeTruthy();
    expect(within(roster).getByText("€1,140,000")).toBeTruthy();
    expect(within(roster).getAllByText("Not set")).toHaveLength(2);
    expect(within(roster).getByText("No plan")).toBeTruthy();
    expect(
      await screen.findByText("Sales goals of Alex Brandt for 2026"),
    ).toBeTruthy();
    expect(screen.getByText("Dana Manager saved a draft")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Save draft" })).toBeTruthy();
  });

  it("opens the plan of the seller a manager picks from the list", async () => {
    window.location.hash = "#/goals?year=2026";
    installFetchStub({
      "GET /analytics/sales-goals": () => jsonResponse(goalRoster),
      [`GET ${PLAN}`]: () => jsonResponse(draftPlan),
      [`GET /analytics/sales-goals/${SAM}/2026`]: () =>
        jsonResponse({
          ...publishedPlan,
          owner_id: SAM,
          owner_name: "Sam Okafor",
        }),
    });
    const user = userEvent.setup();
    render(<SalesGoalsScreen />);
    await user.click(await screen.findByText("Sam Okafor"));
    expect(
      await screen.findByText("Sales goals of Sam Okafor for 2026"),
    ).toBeTruthy();
    expect(window.location.hash).toContain(`seller=${SAM}`);
  });

  it("names a drafted year in the list instead of calling it not set", async () => {
    window.location.hash = "#/goals?year=2026";
    const [alex, ...rest] = goalRoster.data;
    installFetchStub({
      "GET /analytics/sales-goals": () =>
        jsonResponse({
          ...goalRoster,
          data: [{ ...alex, draft_total_minor: 120000000 }, ...rest],
        }),
      [`GET ${PLAN}`]: () => jsonResponse(draftPlan),
    });
    render(<SalesGoalsScreen />);
    const roster = await screen.findByRole("table", { name: "Team members" });
    expect(within(roster).getByText("€1,200,000 draft")).toBeTruthy();
  });

  it("reads a published plan as text until the manager asks to change it", async () => {
    window.location.hash = "#/goals?year=2026";
    installFetchStub({
      "GET /analytics/sales-goals": () => jsonResponse(goalRoster),
      [`GET ${PLAN}`]: () => jsonResponse(editablePublished),
      "GET /analytics/progress": () => jsonResponse(sellerMarch),
    });
    const user = userEvent.setup();
    render(<SalesGoalsScreen />);
    expect(
      await screen.findByText("2026 so far: €60,000 of €100,000 (60%)"),
    ).toBeTruthy();
    await user.click(
      await screen.findByRole("button", { name: "Change goals" }),
    );
    expect(screen.getByRole("button", { name: "Publish" })).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("button", { name: "Publish" })).toBeNull();
  });

  it("shows a seller their own year as text they cannot change", async () => {
    window.location.hash = "#/goals?year=2026";
    installFetchStub({
      "GET /analytics/sales-goals": () =>
        jsonResponse({ ...goalRoster, data: goalRoster.data.slice(0, 1) }),
      [`GET ${PLAN}`]: () => jsonResponse(publishedPlan),
    });
    render(<SalesGoalsScreen />);
    expect(await screen.findByText(/^Read-only\./)).toBeTruthy();
    expect(screen.queryByRole("table", { name: "Team members" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Save draft" })).toBeNull();
    const months = screen.getByLabelText("Monthly goals");
    expect(within(months).getByText("January")).toBeTruthy();
    expect(within(months).getByText("Year total")).toBeTruthy();
    expect(screen.getByText("Published")).toBeTruthy();
  });
});

describe("GoalEditor", () => {
  it("spreads a yearly goal equally over the twelve months", async () => {
    stubWrites(draftPlan);
    const user = userEvent.setup();
    render(<GoalEditor plan={{ ...draftPlan, draft: undefined }} />);
    expect(screen.getByText("12 months are not set.")).toBeTruthy();
    const yearly = screen.getByLabelText("Yearly goal in EUR");
    await user.clear(yearly);
    await user.type(yearly, "120000");
    await user.click(screen.getByRole("button", { name: "Spread equally" }));
    expect(
      await screen.findByText("The months add up to €120,000."),
    ).toBeTruthy();
  });

  it("warns while the months do not add up to the yearly goal", async () => {
    stubWrites(draftPlan);
    const user = userEvent.setup();
    render(<GoalEditor plan={draftPlan} />);
    const january = screen.getByLabelText("January");
    await user.clear(january);
    await user.type(january, "1");
    expect(
      await screen.findByText(/not the yearly goal of €1,200,000\.$/),
    ).toBeTruthy();
  });

  it("saves the months as a draft with the reason given", async () => {
    const sent = stubWrites(draftPlan);
    const user = userEvent.setup();
    render(<GoalEditor plan={draftPlan} />);
    await user.click(screen.getByRole("button", { name: "Save draft" }));
    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]).toMatchObject({
      method: "PUT",
      body: {
        months: draftPlan.draft?.months,
        declared_total_minor: 120000000,
        reason: "Agreed in the yearly review",
        origin: "manual",
      },
    });
  });

  it("discards the open draft", async () => {
    const sent = stubWrites(draftPlan);
    const user = userEvent.setup();
    render(<GoalEditor plan={draftPlan} />);
    await user.click(screen.getByRole("button", { name: "Discard draft" }));
    await waitFor(() =>
      expect(sent).toEqual([expect.objectContaining({ method: "DELETE" })]),
    );
  });

  it("publishes only once the manager confirms the goals were agreed", async () => {
    const sent = stubWrites(draftPlan);
    const user = userEvent.setup();
    render(<GoalEditor plan={draftPlan} />);
    await user.click(screen.getByRole("button", { name: "Publish" }));
    const dialog = await screen.findByRole("dialog");
    const confirm = within(dialog).getByRole("button", { name: "Publish" });
    expect(
      confirm.getAttribute("aria-disabled") ?? confirm.hasAttribute("disabled"),
    ).toBeTruthy();
    await user.click(
      within(dialog).getByLabelText("These goals were agreed together."),
    );
    await user.click(within(dialog).getByRole("button", { name: "Publish" }));
    await waitFor(() =>
      expect(sent).toEqual([
        expect.objectContaining({
          path: "publish",
          body: { reason: "Agreed in the yearly review", confirm_agreed: true },
        }),
      ]),
    );
  });

  it("withdraws published goals with the reason written", async () => {
    const sent = stubWrites(editablePublished);
    const user = userEvent.setup();
    render(<GoalEditor plan={editablePublished} />);
    await user.click(screen.getByRole("button", { name: "Withdraw" }));
    const dialog = await screen.findByRole("dialog");
    await user.type(within(dialog).getByLabelText("Reason"), "Left the team");
    await user.click(within(dialog).getByRole("button", { name: "Withdraw" }));
    await waitFor(() =>
      expect(sent).toEqual([
        expect.objectContaining({
          path: "withdraw",
          body: { reason: "Left the team" },
        }),
      ]),
    );
  });

  it("refuses to publish months that are not all set", () => {
    stubWrites(draftPlan);
    const months = [...(draftPlan.draft?.months ?? [])];
    months[3] = null;
    render(
      <GoalEditor
        plan={{
          ...draftPlan,
          draft: draftPlan.draft && { ...draftPlan.draft, months },
        }}
      />,
    );
    expect(screen.getByText("1 month is not set.")).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Publish" }).hasAttribute("disabled"),
    ).toBe(true);
  });
});
