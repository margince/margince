/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
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
      revenue_at_risk_minor: null,
      buyer_replies: 0,
      prospecting: 0,
      review: 0,
      more_available: false,
      ...readings,
    },
  };
}

function draw(readings: Partial<WorklistReadingsData> = {}) {
  return render(
    <LocaleProvider initial="en">
      <WorklistReadings day={day(readings)} onLane={() => {}} />
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

    expect(screen.getByText("No deal at risk could be priced")).toBeTruthy();
    // The raw minor units must not appear in any formatting.
    expect(screen.queryByText(/384/)).toBeNull();
    expect(screen.queryByText(/38.450.000/)).toBeNull();
  });

  // Null is not zero. Zero says the pipeline is safe; absence says nobody can
  // tell, and a reader who cannot tell those apart is worse off than one shown
  // nothing.
  it("tells an unpriced day apart from a day with nothing at risk", () => {
    const absent = draw({ revenue_at_risk_minor: null });
    expect(screen.getByText("No deal at risk could be priced")).toBeTruthy();
    absent.unmount();

    draw({ revenue_at_risk_minor: 0, revenue_currency: "EUR" });
    expect(screen.queryByText("No deal at risk could be priced")).toBeNull();
    expect(screen.getByText("€0")).toBeTruthy();
  });

  // A source read to its bound makes every figure a floor. The caveat is under
  // the whole strip rather than in one slot: the four are read across as one
  // statement, and marking one invites the reading where the others are exact.
  it("says so when the figures are floors rather than totals", () => {
    const exact = draw({ buyer_replies: 4, more_available: false });
    expect(screen.queryByText(/floors, not totals/)).toBeNull();
    exact.unmount();

    draw({ buyer_replies: 4, more_available: true });
    expect(screen.getByText(/floors, not totals/)).toBeTruthy();
  });

  // The strip is one comparison, so it always draws its four slots — a reading
  // that vanished at zero would make the row fold at a different width from one
  // read to the next.
  it("draws all four readings on a day with no work at all", () => {
    draw();

    expect(screen.getByText("Revenue at risk")).toBeTruthy();
    expect(screen.getByText("Buyer replies")).toBeTruthy();
    expect(screen.getByText("Prospecting")).toBeTruthy();
    expect(screen.getByText("Review")).toBeTruthy();
  });
});
