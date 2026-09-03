/** @vitest-environment jsdom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { PlanSection } from "./brief.plan";
import { jsonResponse, render, stubApi, writes } from "./home.testkit";
import type { WeeklyPlan, WeeklyPlanCommitment } from "./weeklyplan.queries";

// The week ahead. Everything this panel writes is staged first, so the tests
// that matter most are the ones asserting nothing was sent.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function commitment(
  over: Partial<WeeklyPlanCommitment> = {},
): WeeklyPlanCommitment {
  return {
    id: "c1",
    label: "Call the Aster buyer back",
    state: "open",
    position: 1,
    due_on: null,
    help_requested: null,
    manager_response: null,
    manager_user_id: null,
    responded_at: null,
    completed_at: null,
    ...over,
  } as WeeklyPlanCommitment;
}

function plan(over: Partial<WeeklyPlan> = {}): WeeklyPlan {
  return {
    id: "p1",
    local_week_start: "2026-06-08",
    status: "open",
    commitments: [commitment()],
    ...over,
  } as WeeklyPlan;
}

describe("the week ahead", () => {
  // The rep has not planned yet. A read that created a plan would put an empty
  // week in everyone's history the first time they opened the page, so the
  // server answers 404 and the panel offers rather than assumes.
  it("offers to open a plan when there is none, and opening one is the only write", async () => {
    const calls = stubApi({
      "GET /weekly-plans/current": () =>
        jsonResponse({ title: "Not Found" }, 404),
      "POST /weekly-plans/current": () =>
        jsonResponse(plan({ commitments: [] })),
    });
    render(<PlanSection />);

    const start = await screen.findByRole("button", { name: en["plan.start"] });
    expect(writes(calls)).toHaveLength(0);

    await userEvent.click(start);

    await waitFor(() => expect(writes(calls)).toHaveLength(1));
    expect(writes(calls)[0].path).toBe("/weekly-plans/current");
    expect(writes(calls)[0].method).toBe("POST");
  });

  // The rule the design system states and this panel exists to honour: a
  // control that IS the write is a Switch, and one that stages a choice is a
  // Checkbox. Ticking sends nothing.
  it("stages a tick and sends nothing until Save", async () => {
    const calls = stubApi({
      "GET /weekly-plans/current": () => jsonResponse(plan()),
    });
    render(<PlanSection />);

    const box = await screen.findByRole("checkbox", {
      name: "Call the Aster buyer back",
    });
    // Save is not in the resting layout. A save bar standing there would say
    // there is something to save on a week nobody has touched.
    expect(screen.queryByRole("button", { name: /Save/ })).toBeNull();

    await userEvent.click(box);

    expect(writes(calls)).toHaveLength(0);
    expect(
      screen.getByRole("button", {
        name: en["plan.save_one"].replace("{count}", "1"),
      }),
    ).toBeTruthy();
  });

  it("settles the staged commitment on Save, and only that one", async () => {
    const calls = stubApi({
      "GET /weekly-plans/current": () =>
        jsonResponse(
          plan({
            commitments: [
              commitment(),
              commitment({ id: "c2", label: "Send the Weber quote" }),
            ],
          }),
        ),
      "PUT /weekly-plans/commitments/c1/state": () =>
        new Response(null, { status: 204 }),
    });
    render(<PlanSection />);

    await userEvent.click(
      await screen.findByRole("checkbox", {
        name: "Call the Aster buyer back",
      }),
    );
    await userEvent.click(
      screen.getByRole("button", {
        name: en["plan.save_one"].replace("{count}", "1"),
      }),
    );

    await waitFor(() => expect(writes(calls)).toHaveLength(1));
    expect(writes(calls)[0].path).toBe("/weekly-plans/commitments/c1/state");
    expect(writes(calls)[0].body).toEqual({ state: "done" });
  });

  // A save that fails halfway must not leave the panel lying about it.
  //
  // Save settles each staged row in its own request, so a refusal on the second
  // one leaves the first written and the rest not. What the reader must not be
  // told is that nothing happened, or that everything did: the rows that landed
  // have to stop being staged, and the one that failed has to still say so.
  it("keeps the unsaved rows staged when one write is refused", async () => {
    const calls = stubApi({
      "GET /weekly-plans/current": () =>
        jsonResponse(
          plan({
            commitments: [
              commitment(),
              commitment({ id: "c2", label: "Send the Weber quote" }),
            ],
          }),
        ),
      "PUT /weekly-plans/commitments/c1/state": () =>
        new Response(null, { status: 204 }),
      "PUT /weekly-plans/commitments/c2/state": () =>
        jsonResponse({ title: "Conflict" }, 409),
    });
    render(<PlanSection />);

    await userEvent.click(
      await screen.findByRole("checkbox", {
        name: "Call the Aster buyer back",
      }),
    );
    await userEvent.click(
      screen.getByRole("checkbox", { name: "Send the Weber quote" }),
    );
    await userEvent.click(
      screen.getByRole("button", {
        name: en["plan.save_other"].replace("{count}", "2"),
      }),
    );

    // BOTH are attempted. A loop that threw on the first refusal would leave
    // the second row unwritten with nothing having asked it to stop.
    await waitFor(() => expect(writes(calls)).toHaveLength(2));
    // The refusal is SAID. A panel that swallows it tells a rep their week is
    // recorded when half of it is not.
    expect(await screen.findByRole("alert")).toBeTruthy();
    // And exactly ONE row is still staged: the refused one. Leaving both staged
    // invites a second Save that re-sends a write which already succeeded;
    // clearing both loses the reader's own unsaved intent. The Save button
    // counts the staged set, so its label is where that count is readable.
    await waitFor(() =>
      expect(
        screen.getByRole("button", {
          name: en["plan.save_one"].replace("{count}", "1"),
        }),
      ).toBeTruthy(),
    );
  });

  // `missed` is what the week's close writes over a commitment left open. A box
  // that reopened it would let one click undo the close, and the review's counts
  // would stop agreeing with the rows they were counted from.
  it("offers no checkbox on a commitment the close already settled", async () => {
    stubApi({
      "GET /weekly-plans/current": () =>
        jsonResponse(plan({ commitments: [commitment({ state: "missed" })] })),
    });
    render(<PlanSection />);

    expect(await screen.findByText(en["plan.state.missed"])).toBeTruthy();
    expect(screen.queryByRole("checkbox")).toBeNull();
  });

  // A closed week is history: the weekly job froze its outcome into the review.
  it("draws no controls at all on a closed week", async () => {
    stubApi({
      "GET /weekly-plans/current": () =>
        jsonResponse(plan({ status: "closed" })),
    });
    render(<PlanSection />);

    expect(await screen.findByText("Call the Aster buyer back")).toBeTruthy();
    expect(screen.queryByRole("checkbox")).toBeNull();
    expect(screen.queryByRole("button", { name: en["plan.add"] })).toBeNull();
    expect(
      screen.queryByRole("button", { name: en["plan.help.ask"] }),
    ).toBeNull();
  });

  // The editor is behind a press. An always-open textarea would put an empty box
  // on every row of a week where nobody needs anything, which is most weeks.
  it("keeps the help editor closed until asked, then sends what was typed", async () => {
    const calls = stubApi({
      "GET /weekly-plans/current": () => jsonResponse(plan()),
      "PUT /weekly-plans/commitments/c1/help": () =>
        new Response(null, { status: 204 }),
    });
    render(<PlanSection />);

    const ask = await screen.findByRole("button", {
      name: en["plan.help.ask"],
    });
    expect(screen.queryByLabelText(en["plan.help.label"])).toBeNull();

    await userEvent.click(ask);
    await userEvent.type(
      screen.getByLabelText(en["plan.help.label"]),
      "Need you on the pricing call",
    );
    await userEvent.click(
      screen.getByRole("button", { name: en["plan.help.send"] }),
    );

    await waitFor(() => expect(writes(calls)).toHaveLength(1));
    expect(writes(calls)[0].body).toEqual({
      help_requested: "Need you on the pricing call",
    });
  });

  // A standing ask with no answer yet says so. Silence where an answer will go
  // reads as an answer of "nothing", which is not what happened.
  it("shows a standing request as waiting until the lead answers", async () => {
    stubApi({
      "GET /weekly-plans/current": () =>
        jsonResponse(
          plan({
            commitments: [
              commitment({ help_requested: "Need you on pricing" }),
            ],
          }),
        ),
    });
    render(<PlanSection />);

    expect(await screen.findByText(en["plan.help.waiting"])).toBeTruthy();
  });

  // An empty date is an absence, not an empty string: the contract types
  // due_on as nullable and "" is neither a date nor a null.
  it("adds a commitment with no date as a null date", async () => {
    const calls = stubApi({
      "GET /weekly-plans/current": () =>
        jsonResponse(plan({ commitments: [] })),
      "POST /weekly-plans/commitments": () => jsonResponse(commitment(), 201),
    });
    render(<PlanSection />);

    await userEvent.click(
      await screen.findByRole("button", { name: en["plan.add"] }),
    );
    await userEvent.type(
      screen.getByLabelText(`${en["plan.new.label"]} *`),
      "Book the Weber demo",
    );
    await userEvent.click(
      screen.getByRole("button", { name: en["plan.new.save"] }),
    );

    await waitFor(() => expect(writes(calls)).toHaveLength(1));
    expect(writes(calls)[0].body).toEqual({
      label: "Book the Weber demo",
      due_on: null,
    });
  });
});
