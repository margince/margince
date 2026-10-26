/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { en } from "../../i18n/en";
import { render } from "../../testing/brief.testkit";
import { ScorecardPanel } from "./brief.weekly.scorecard";

// The scorecard's whole design is that ABSENT is not ZERO, and every case here
// is a way of asking whether the panel still honours that. A block of zeros
// drawn for a rep who carried no leads reads as failure at something nobody
// asked of them, and it is the one mistake this surface must not make.

afterEach(cleanup);

const leadBlock = {
  advanced: 4,
  disqualified: 1,
  promoted: 2,
  answered_in_target: 9,
  breached: 3,
  meetings_booked: 6,
  meetings_held: 5,
  meetings_no_show: 1,
  meetings_partial_history: 0,
};

const dealBlock = {
  advances: 7,
  regressions: 2,
  median_days_in_stage: 11,
  with_next_step: 3,
  open: 5,
  multi_threaded: 2,
  close_date_sound: 4,
  forecast_up: 3,
  forecast_down: 1,
};

/** The cells of the row a measure is named in, as the reader reads them. */
function rowOf(label: string): string[] {
  const row = screen.getByText(label).closest("tr");
  if (!row) throw new Error(`no row named ${label}`);
  return [...row.querySelectorAll("td, th")].map(
    (cell) => cell.textContent ?? "",
  );
}

describe("the weekly scorecard", () => {
  it("draws nothing at all when the review carries no scorecard", () => {
    const { container } = render(
      <ScorecardPanel scorecard={undefined} weekStart="2026-06-01" />,
    );
    expect(container.innerHTML).toBe("");
  });

  it("names the reviewed week by its number, not as this week", () => {
    render(
      <ScorecardPanel scorecard={{ deal: dealBlock }} weekStart="2026-06-01" />,
    );
    expect(
      screen.getByRole("columnheader", {
        name: en["brief.report.col.week"].replace("{week}", "23"),
      }),
    ).toBeTruthy();
  });

  it("omits the lead block rather than drawing a rep zeros they did not earn", () => {
    render(
      <ScorecardPanel scorecard={{ deal: dealBlock }} weekStart="2026-06-01" />,
    );
    expect(
      screen.queryByText(en["brief.weekly.scorecard.advanced"]),
    ).toBeNull();
    // The deal block it did send is still drawn: an omission, not a give-up.
    expect(
      screen.getByText(en["brief.weekly.scorecard.advances"]),
    ).toBeTruthy();
  });

  it("omits the deal block on its own terms too", () => {
    render(
      <ScorecardPanel scorecard={{ lead: leadBlock }} weekStart="2026-06-01" />,
    );
    expect(
      screen.queryByText(en["brief.weekly.scorecard.advances"]),
    ).toBeNull();
    expect(
      screen.getByText(en["brief.weekly.scorecard.advanced"]),
    ).toBeTruthy();
  });

  it("draws a present block whose counts are zero, because that is a finding", () => {
    render(
      <ScorecardPanel
        scorecard={{
          lead: { ...leadBlock, advanced: 0, answered_in_target: 0 },
        }}
        weekStart="2026-06-01"
      />,
    );
    // Present and zero: the rep HAD leads and moved none, which the panel must
    // say rather than hide.
    expect(
      screen.getByText(en["brief.weekly.scorecard.advanced"]),
    ).toBeTruthy();
  });

  it("omits the median when no deal changed stage, never drawing it as zero days", () => {
    render(
      <ScorecardPanel
        scorecard={{ deal: { ...dealBlock, median_days_in_stage: null } }}
        weekStart="2026-06-01"
      />,
    );
    expect(
      screen.queryByText(en["brief.weekly.scorecard.medianDaysInStage"]),
    ).toBeNull();
  });

  it("draws the median when there is one", () => {
    render(
      <ScorecardPanel scorecard={{ deal: dealBlock }} weekStart="2026-06-01" />,
    );
    expect(
      screen.getByText(en["brief.weekly.scorecard.medianDaysInStage"]),
    ).toBeTruthy();
  });

  it("names the open-deal denominator beside each coverage count", () => {
    render(
      <ScorecardPanel scorecard={{ deal: dealBlock }} weekStart="2026-06-01" />,
    );
    // "3 of 5 open deals" — the count is only judgeable against its
    // denominator, which is why the server sends counts and not a rate.
    expect(
      screen.getAllByText(
        en["brief.weekly.scorecard.ofOpen"].replace("{total}", "5"),
      ).length,
    ).toBeGreaterThan(0);
  });

  // A week with no open deal has no denominator, so the three coverage rows
  // have nothing to be a share of. "0 of 0 open deals" states a rate nobody
  // could have scored.
  it("says there were no open deals rather than scoring a share of none", () => {
    render(
      <ScorecardPanel
        scorecard={{
          deal: {
            ...dealBlock,
            open: 0,
            with_next_step: 0,
            multi_threaded: 0,
            close_date_sound: 0,
          },
        }}
        weekStart="2026-06-01"
      />,
    );

    // One arm for all three: three spellings of one absence would read as
    // three different weeks.
    expect(
      screen.getAllByText(en["brief.weekly.scorecard.noOpen"]),
    ).toHaveLength(3);
    expect(
      screen.queryByText(
        en["brief.weekly.scorecard.ofOpen"].replace("{total}", "0"),
      ),
    ).toBeNull();
  });

  // The breach is the reading. Leading with the leads answered in target put
  // the reassurance in the figure and the actionable number in a footnote.
  it("leads the target row with the breaches, not with the answers", () => {
    render(
      <ScorecardPanel
        scorecard={{ lead: { ...leadBlock, arrived: 12 } }}
        weekStart="2026-06-01"
      />,
    );

    expect(rowOf(en["brief.weekly.scorecard.answeredInTarget"])).toEqual([
      en["brief.weekly.scorecard.answeredInTarget"],
      "3",
      en["brief.weekly.scorecard.unanswered"]
        .replace("{count}", "3")
        .replace("{total}", "12"),
    ]);
  });

  // Zero breaches is not every lead answered. Three leads arrived, one got a
  // first response, none was stamped as breached: two are still unanswered.
  // The panel used to say "All answered in target" here.
  it("names the unanswered leads when none breached but not all were answered", () => {
    render(
      <ScorecardPanel
        scorecard={{
          lead: {
            ...leadBlock,
            arrived: 3,
            answered_in_target: 1,
            breached: 0,
          },
        }}
        weekStart="2026-06-01"
      />,
    );
    expect(rowOf(en["brief.weekly.scorecard.answeredInTarget"])).toEqual([
      en["brief.weekly.scorecard.answeredInTarget"],
      "0",
      en["brief.weekly.scorecard.unanswered"]
        .replace("{count}", "2")
        .replace("{total}", "3"),
    ]);
  });

  // The plan's case: one lead answered late (so also breached), one never
  // answered. One has no recorded response, not zero. Breaches are not
  // subtracted, because the late answer is in both counts.
  it("counts one unanswered lead of two when the other was answered late", () => {
    render(
      <ScorecardPanel
        scorecard={{
          lead: {
            ...leadBlock,
            arrived: 2,
            answered_in_target: 1,
            breached: 2,
          },
        }}
        weekStart="2026-06-01"
      />,
    );
    expect(rowOf(en["brief.weekly.scorecard.answeredInTarget"])[2]).toBe(
      en["brief.weekly.scorecard.unanswered"]
        .replace("{count}", "1")
        .replace("{total}", "2"),
    );
  });

  it("says every lead got a recorded first response, without claiming a target was met", () => {
    render(
      <ScorecardPanel
        scorecard={{
          lead: {
            ...leadBlock,
            arrived: 2,
            answered_in_target: 2,
            breached: 0,
          },
        }}
        weekStart="2026-06-01"
      />,
    );
    const basis = rowOf(en["brief.weekly.scorecard.answeredInTarget"])[2];
    expect(basis).toBe(
      en["brief.weekly.scorecard.allResponded"].replace("{total}", "2"),
    );
    expect(basis).not.toMatch(/in target/i);
  });

  it("says no lead arrived when the cohort is empty", () => {
    render(
      <ScorecardPanel
        scorecard={{
          lead: {
            ...leadBlock,
            arrived: 0,
            answered_in_target: 0,
            breached: 0,
          },
        }}
        weekStart="2026-06-01"
      />,
    );
    expect(rowOf(en["brief.weekly.scorecard.answeredInTarget"])[2]).toBe(
      en["brief.weekly.scorecard.noArrivals"],
    );
  });

  // A week frozen before the cohort was recorded, or served by an older
  // server, cannot say how many went unanswered, and must not read as none.
  it.each([undefined, null])(
    "says the arrivals were not recorded when the cohort is %s, even with no breach",
    (arrived) => {
      render(
        <ScorecardPanel
          scorecard={{ lead: { ...leadBlock, arrived, breached: 0 } }}
          weekStart="2026-06-01"
        />,
      );
      const basis = rowOf(en["brief.weekly.scorecard.answeredInTarget"])[2];
      expect(basis).toBe(
        en["brief.weekly.scorecard.respondedNoCohort"].replace("{count}", "9"),
      );
    },
  );

  it("says the meeting counts are a floor only when history is actually missing", () => {
    render(
      <ScorecardPanel scorecard={{ lead: leadBlock }} weekStart="2026-06-01" />,
    );
    expect(
      screen.queryByText(en["brief.weekly.scorecard.partialHistory"]),
    ).toBeNull();

    cleanup();
    render(
      <ScorecardPanel
        scorecard={{ lead: { ...leadBlock, meetings_partial_history: 2 } }}
        weekStart="2026-06-01"
      />,
    );
    expect(
      screen.getByText(en["brief.weekly.scorecard.partialHistory"]),
    ).toBeTruthy();
  });

  // Three states, not two. Zero and absent both draw nothing, but they are
  // different facts: zero means the week was rebuilt completely, absent means
  // it was scored before the rebuild existed. Only a real shortfall is drawn,
  // and drawing it is what stops a reader taking a floor for a total.
  it("reports deals it could not rebuild, and only when there are some", () => {
    render(
      <ScorecardPanel
        scorecard={{ deal: { ...dealBlock, unreconstructible: 0 } }}
        weekStart="2026-06-01"
      />,
    );
    expect(
      screen.queryByText(en["brief.weekly.scorecard.unreconstructible"]),
    ).toBeNull();

    cleanup();
    // A week scored before the reconstruction existed sends no field at all.
    // It must read like the zero case rather than throwing or drawing "0".
    render(
      <ScorecardPanel scorecard={{ deal: dealBlock }} weekStart="2026-06-01" />,
    );
    expect(
      screen.queryByText(en["brief.weekly.scorecard.unreconstructible"]),
    ).toBeNull();

    cleanup();
    render(
      <ScorecardPanel
        scorecard={{ deal: { ...dealBlock, unreconstructible: 2 } }}
        weekStart="2026-06-01"
      />,
    );
    expect(
      screen.getByText(en["brief.weekly.scorecard.unreconstructible"]),
    ).toBeTruthy();
  });
});
