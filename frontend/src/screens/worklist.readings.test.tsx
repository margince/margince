/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import type { WorklistFilter } from "./worklist.queries";
import { WorklistReadings } from "./worklist.readings";

// What the strip above the queue claims, and what it refuses to claim.
//
// The readings are the server's figures; this file is about the two places a
// client can still get them wrong — formatting a number whose units nobody
// knows, and drawing an absent figure as a zero.

type Worklist = components["schemas"]["Worklist"];
type WorklistReadingsData = components["schemas"]["WorklistReadings"];

function day(readings: Partial<WorklistReadingsData> = {}): Worklist {
  return {
    as_of: "2026-08-31T09:00:00Z",
    scope: "mine",
    scope_options: ["mine"],
    queue: [],
    summary: { urgent: 0, due: 0, lower_priority: 0, total: 0 },
    sources_unavailable: [],
    reach: [],
    counts: [],
    readings: {
      changed_since_brief: 0,
      revenue_at_risk_minor: null,
      buyer_replies: 0,
      prospecting: 0,
      review: 0,
      more_available: false,
      ...readings,
    },
  };
}

function draw(
  readings: Partial<WorklistReadingsData> = {},
  onLane: (filter: WorklistFilter) => void = () => {},
) {
  return render(
    <LocaleProvider initial="en">
      <WorklistReadings day={day(readings)} onLane={onLane} />
    </LocaleProvider>,
  );
}

afterEach(cleanup);

describe("the worklist readings strip", () => {
  it("states the four readings the server sent", () => {
    draw({
      revenue_at_risk_minor: 384_500_00,
      revenue_currency: "EUR",
      buyer_replies: 14,
      prospecting: 3,
      review: 27,
    });

    expect(screen.getByText("14")).toBeTruthy();
    expect(screen.getByText("3")).toBeTruthy();
    expect(screen.getByText("27")).toBeTruthy();
  });

  // A figure whose units nobody knows is not money. Drawing the raw minor units
  // as a euro amount is the error the conversion seam exists to prevent, and it
  // reaches the reader as a number they have no reason to doubt.
  it("refuses to draw an amount whose currency the server would not name", () => {
    draw({ revenue_at_risk_minor: 384_500_00, revenue_currency: null });

    expect(
      screen.getByText(en["worklist.readings.revenue.noFigure"]),
    ).toBeTruthy();
    // The reading names what it could not price rather than leaving the slot
    // to state an absence twice.
    expect(
      screen.getByText(en["worklist.readings.revenue.unpriced"]),
    ).toBeTruthy();
    // The raw minor units must not appear in any formatting.
    expect(screen.queryByText(/384/)).toBeNull();
    expect(screen.queryByText(/38.450.000/)).toBeNull();
  });

  // Null is not zero. Zero says nothing is drifting; absence says nobody can
  // tell, and a reader who cannot tell those apart is worse off than one shown
  // nothing.
  it("tells an unpriced day apart from a day with nothing at risk", () => {
    const absent = draw({ revenue_at_risk_minor: null });
    expect(
      screen.getByText(en["worklist.readings.revenue.noFigure"]),
    ).toBeTruthy();
    absent.unmount();

    draw({ revenue_at_risk_minor: 0, revenue_currency: "EUR" });
    expect(
      screen.queryByText(en["worklist.readings.revenue.noFigure"]),
    ).toBeNull();
    expect(screen.getByText("€0")).toBeTruthy();
    expect(
      screen.getByText(en["worklist.readings.revenue.detail"]),
    ).toBeTruthy();
  });

  // The unpriced arm used to be the one reading on this strip with no way out,
  // and it is the one a reader most needs: the lane holds the drifting deals
  // whether or not anybody priced them, and pricing them is the work.
  it("opens the deals-at-risk lane whether or not the money could be priced", async () => {
    const user = userEvent.setup();
    const priced = vi.fn();
    const unpriced = vi.fn();

    const shown = draw(
      { revenue_at_risk_minor: 384_500_00, revenue_currency: "EUR" },
      priced,
    );
    await user.click(
      screen.getByRole("button", {
        name: "Open",
        description: "Revenue at risk",
      }),
    );
    expect(priced).toHaveBeenCalledWith("deals_at_risk");
    shown.unmount();

    draw({ revenue_at_risk_minor: null }, unpriced);
    await user.click(
      screen.getByRole("button", {
        name: "Open",
        description: "Revenue at risk",
      }),
    );
    expect(unpriced).toHaveBeenCalledWith("deals_at_risk");
  });

  // A source read to its bound makes every figure a floor. The caveat is under
  // the whole strip rather than in one slot: the four are read across as one
  // statement, and marking one invites the reading where the others are exact.
  it("says so when the figures are floors rather than totals", () => {
    const exact = draw({ buyer_replies: 4, more_available: false });
    expect(screen.queryByText(/Each figure is a minimum/)).toBeNull();
    exact.unmount();

    draw({ buyer_replies: 4, more_available: true });
    expect(screen.getByText(/Each figure is a minimum/)).toBeTruthy();
  });

  // The strip is one comparison, so it always draws its four slots — a reading
  // that vanished at zero would make the row fold at a different width from one
  // read to the next.
  it("draws all four readings on a day with no work at all", () => {
    draw();

    expect(screen.getByText(en["worklist.readings.revenue"])).toBeTruthy();
    expect(screen.getByText(en["worklist.readings.replies"])).toBeTruthy();
    expect(screen.getByText(en["worklist.readings.prospecting"])).toBeTruthy();
    expect(screen.getByText(en["worklist.readings.review"])).toBeTruthy();
  });

  // ZERO IS A READING. A count of none is what the server counted, so the slot
  // spells the number rather than reaching for an empty term — and the line
  // under it still says what the figure was taken over, because a bare "0" with
  // nothing beside it reads as a slot that failed to fill.
  it("draws a count of none as the number it is", () => {
    draw({ buyer_replies: 0 });

    const card = screen
      .getByText(en["worklist.readings.replies"])
      .closest(".stat-card");
    expect(card?.querySelector(".stat-card-value")?.textContent).toBe("0");
    expect(
      screen.getByText(en["worklist.readings.replies.detail"]),
    ).toBeTruthy();
  });
});
