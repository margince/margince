/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { readingsDay } from "./brief.fixtures";
import { BriefReadingsStrip } from "./brief.readings";
import type { Worklist } from "./worklist.queries";

// Urgent counts rows from every lane, so a lane that did not answer makes it a
// floor. A side read that failed is named beside the lanes but puts no row on
// the queue, and Urgent stays the exact count it was.

type Unavailable = Worklist["sources_unavailable"][number];

const failedCalendar: Unavailable = {
  source: "calendar",
  category: "meetings",
  reason: "failed",
  contributes_rows: false,
};

const withheldMeetings: Unavailable = {
  source: "meeting",
  category: "meetings",
  reason: "withheld",
  contributes_rows: true,
};

function drawUrgent(urgent: number, missing: Unavailable[]): string {
  const day = readingsDay({}, undefined, undefined, { urgent });
  day.sources_unavailable = missing;
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <BriefReadingsStrip day={day} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  const value = screen
    .getByText(en["brief.readings.urgent"])
    .closest(".stat-card")
    ?.querySelector(".stat-card-value");
  if (!(value instanceof HTMLElement)) {
    throw new Error("the urgent reading has no figure on the page");
  }
  return value.textContent ?? "";
}

afterEach(cleanup);

describe("the urgent reading over a source that did not answer", () => {
  it("stays exact when only the calendar read failed", () => {
    expect(drawUrgent(4, [failedCalendar])).toBe("4");
  });

  it("still states a zero when only the calendar read failed", () => {
    expect(drawUrgent(0, [failedCalendar])).toBe("0");
  });

  it("is a floor when the meetings lane did not answer", () => {
    expect(drawUrgent(4, [failedCalendar, withheldMeetings])).toBe("4+");
  });

  // An older server sends no flag, and the safe reading of silence is a lane.
  it("is a floor when the server does not say whether the source holds rows", () => {
    expect(drawUrgent(4, [{ source: "meeting", reason: "failed" }])).toBe(
      "4+",
    );
  });
});
