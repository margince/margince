/** @vitest-environment jsdom */
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { en } from "../i18n/en";
import { PlanContract } from "./brief.plan.contract";
import { jsonResponse, render, stubApi, writes } from "./home.testkit";
import type { WeeklyPlan } from "./weeklyplan.queries";

// The contract's three states are the whole subject. `null` is a rep who has
// written nothing, `""` is one who looked and says there is nothing to name,
// and text is text. A surface that folds the first two reports an unconsidered
// week as a safe one, which is the mistake this panel exists not to make.

afterEach(cleanup);

const basePlan = {
  id: "11111111-1111-1111-1111-111111111111",
  local_week_start: "2026-06-15",
  status: "open",
  commitments: [],
} as unknown as WeeklyPlan;

const planWith = (extra: Partial<WeeklyPlan>): WeeklyPlan =>
  ({ ...basePlan, ...extra }) as WeeklyPlan;

describe("the plan's contract", () => {
  it("draws nothing at all when there is no plan", () => {
    const { container } = render(<PlanContract plan={null} editable />);
    expect(container.innerHTML).toBe("");
  });

  it("says a field is unwritten rather than showing it blank", () => {
    render(<PlanContract plan={planWith({ risks: null })} editable />);
    expect(
      screen.getAllByText(en["plan.contract.unwritten"]).length,
    ).toBeGreaterThan(0);
  });

  it("tells an emptied field from one never written", () => {
    render(<PlanContract plan={planWith({ risks: "" })} editable />);
    // The rep looked and said there is nothing. That is a statement about the
    // week, and it must not read as silence.
    expect(screen.getByText(en["plan.contract.nothingToName"])).toBeTruthy();
  });

  it("draws no capacity line when the server sent no capacity", () => {
    render(<PlanContract plan={planWith({ capacity: undefined })} editable />);
    // Absent means no calendar reader is composed. A "0 meetings" line here
    // would tell a rep their week is free because an integration is missing.
    expect(screen.queryByText(/meetings and/, { exact: false })).toBeNull();
  });

  it("draws a real zero when the server did count the week", () => {
    render(
      <PlanContract
        plan={planWith({ capacity: { meetings: 0, tasks: 0 } })}
        editable
      />,
    );
    expect(
      screen.getByText(
        en["plan.contract.capacityLine"]
          .replace("{meetings}", "0")
          .replace("{tasks}", "0"),
      ),
    ).toBeTruthy();
  });

  it("warns when next week's calendar leaves no room", () => {
    render(
      <PlanContract
        plan={planWith({
          capacity: { meetings: 7, tasks: 3 },
          commitments: [],
        })}
        editable
      />,
    );
    expect(screen.getByText(en["plan.contract.crowded"])).toBeTruthy();
  });

  it("does not cry crowded on an ordinary week", () => {
    render(
      <PlanContract
        plan={planWith({ capacity: { meetings: 2, tasks: 1 } })}
        editable
      />,
    );
    expect(screen.queryByText(en["plan.contract.crowded"])).toBeNull();
  });

  it("offers no edit control on a week this seat may not write", () => {
    render(<PlanContract plan={planWith({ risks: null })} editable={false} />);
    // A control that exists in order to fail is worse than no control.
    expect(screen.queryByText(en["plan.contract.edit"])).toBeNull();
  });
});

describe("what the contract form sends", () => {
  it("saves only the half being edited, so the other is left alone", async () => {
    const calls = stubApi({
      "PUT /weekly-plans/current/contract": () => jsonResponse({}, 200),
    });
    render(
      <PlanContract
        plan={planWith({ risks: null, capacity_note: "Conference Thursday" })}
        editable
      />,
    );
    await userEvent.click(screen.getAllByText(en["plan.contract.edit"])[0]);
    await userEvent.type(screen.getByRole("textbox"), "Sponsor risk");
    await userEvent.click(screen.getByText(en["plan.contract.save"]));

    const sent = writes(calls).find((c) =>
      c.path.endsWith("/weekly-plans/current/contract"),
    );
    expect(sent).toBeTruthy();
    // The untouched half must be ABSENT from the body, not sent as the value
    // this component happened to be holding: the server resolves an omitted
    // key under its own row lock, and a stale value sent here would undo a
    // save somebody else made in the meantime.
    expect(Object.keys(sent?.body ?? {})).toEqual(["risks"]);
  });
});
