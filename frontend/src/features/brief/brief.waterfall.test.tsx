/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { OutlookPanel } from "./brief.waterfall";

type Review = components["schemas"]["WeeklyReview"];
type Outlook = NonNullable<Review["outlook"]>[number];

afterEach(cleanup);

function draw(node: ReactNode) {
  return render(<LocaleProvider initial="en">{node}</LocaleProvider>);
}

const horizon = (over: Partial<Outlook> = {}): Outlook => ({
  period_kind: "quarter",
  period_start: "2026-04-01",
  period_end: "2026-06-30",
  base_currency: "EUR",
  won_minor: 180_00,
  commit_minor: 120_00,
  best_case_minor: 390_00,
  weighted_minor: 240_00,
  opening_landing_minor: 260_00,
  closing_landing_minor: 300_00,
  forward_measure: "commit_evidence",
  movement: [
    { bar: "created", delta_minor: 90_00 },
    { bar: "slipped", delta_minor: -50_00 },
  ],
  ...over,
});

function show(outlook: readonly Outlook[], onOpenForecast = vi.fn()) {
  draw(
    <OutlookPanel
      outlook={outlook}
      locale="en"
      onOpenForecast={onOpenForecast}
    />,
  );
  return onOpenForecast;
}

describe("the frozen outlook", () => {
  // A week nobody forecast and a week that landed on nothing are different
  // facts, and a strip of zeros would claim the second.
  it("says no forecast was composed rather than drawing zeros", () => {
    show([]);

    expect(screen.getByText(en["brief.weekly.outlook.none"])).toBeTruthy();
    expect(screen.queryByText(en["brief.weekly.outlook.landing"])).toBeNull();
  });

  // The figure includes commit, so a reader adding the two would double-count
  // the overlap. The basis line is what stops that, and it sits under the
  // reading rather than inside its name: the label names what is measured.
  it("says under best case that the figure already contains commit", () => {
    show([horizon()]);

    expect(screen.getByText(en["brief.weekly.outlook.bestCase"])).toBeTruthy();
    expect(
      screen.getByText(en["brief.weekly.outlook.bestCaseDetail"]),
    ).toBeTruthy();
  });

  // A period frozen with no base currency cannot be said as money. Its cells
  // name that absence in words rather than drawing a bare dash.
  it("names an unforecast period in words rather than an em dash", () => {
    show([horizon({ base_currency: "" })]);

    const table = screen.getByRole("table", {
      name: en["brief.weekly.outlook"],
    });
    const figures = [...table.querySelectorAll("tbody tr")].map(
      (row) => row.lastElementChild?.textContent,
    );
    expect(figures).toHaveLength(5);
    expect(new Set(figures)).toEqual(new Set([en["format.notForecast"]]));
  });

  // One column per period the review froze, in the order it froze them.
  it("sets each frozen period beside the others as a column", () => {
    show([
      horizon({ period_kind: "week", won_minor: 10_00 }),
      horizon({ period_kind: "quarter" }),
    ]);

    const headers = screen
      .getAllByRole("columnheader")
      .map((cell) => cell.textContent);
    expect(headers.slice(1)).toEqual([
      en["brief.weekly.outlook.week"],
      en["brief.weekly.outlook.quarter"],
    ]);
  });

  it("says which forward measure the landing was read under", () => {
    show([horizon()]);

    expect(
      screen.getByText(en["brief.weekly.outlook.measure.commit_evidence"]),
    ).toBeTruthy();
  });

  // A past week keeps saying what it was read under. A review frozen under the
  // weighted measure must not re-label itself when the setting changes.
  it("keeps the measure the week was frozen under", () => {
    show([horizon({ forward_measure: "weighted" })]);

    expect(
      screen.getByText(en["brief.weekly.outlook.measure.weighted"]),
    ).toBeTruthy();
    expect(
      screen.queryByText(en["brief.weekly.outlook.measure.commit_evidence"]),
    ).toBeNull();
  });
});

describe("the movement bridge", () => {
  it("draws the bars in the order the server froze them", () => {
    show([horizon()]);

    // Read off the sr-only TABLE rather than the bars: it carries the same
    // steps for a reader who cannot see them, so an order that held only in
    // the drawing would be an order half the readers never get.
    const rows = screen.getAllByRole("row").map((row) => row.textContent ?? "");
    const created = rows.findIndex((row) =>
      row.includes(en["brief.weekly.bar.created"]),
    );
    const slipped = rows.findIndex((row) =>
      row.includes(en["brief.weekly.bar.slipped"]),
    );
    expect(created).toBeGreaterThanOrEqual(0);
    expect(slipped).toBeGreaterThan(created);
  });

  // The figures are the forward measure's output, so the one door opens the
  // section that draws it.
  it("opens the forecast the figures were read under", async () => {
    const opened = vi.fn();
    show([horizon()], opened);

    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", { name: en["brief.weekly.outlook.open"] }),
    );
    expect(opened).toHaveBeenCalledTimes(1);
  });

  // A week nobody forecast has no figure to open anything from.
  it("offers no door on a week that froze no outlook", () => {
    show([]);

    expect(
      screen.queryByRole("button", { name: en["brief.weekly.outlook.open"] }),
    ).toBeNull();
  });

  // No Monday snapshot means there is no opening to have moved from. A zero
  // opening would draw a week that started from nothing and made everything.
  it("shows 'no Monday snapshot' rather than a zero opening", () => {
    show([horizon({ opening_landing_minor: undefined, movement: [] })]);

    expect(screen.getByText(en["brief.weekly.bridge.noOpening"])).toBeTruthy();
    expect(screen.queryByText(en["brief.weekly.bridge.opening"])).toBeNull();
  });

  // The bars are a CLAIM that these causes account for the whole difference.
  // The primitive checks that arithmetic in production, and a reader must be
  // told when it does not hold rather than shown bars that quietly do not add.
  it("warns when the bars do not reconcile to the closing figure", () => {
    show([
      horizon({
        opening_landing_minor: 260_00,
        closing_landing_minor: 300_00,
        movement: [{ bar: "created", delta_minor: 10_00 }],
      }),
    ]);

    expect(screen.getByText(en["brief.weekly.bridge.reconcile"])).toBeTruthy();
  });

  it("draws no warning when the bars do add up", () => {
    show([horizon()]);

    expect(screen.queryByText(en["brief.weekly.bridge.reconcile"])).toBeNull();
  });
});
