/** @vitest-environment jsdom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { en } from "../i18n/en";
import { render } from "./home.testkit";
import { ScorecardPanel } from "./home.weekly.scorecard";

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

describe("the weekly scorecard", () => {
  it("draws nothing at all when the review carries no scorecard", () => {
    const { container } = render(<ScorecardPanel scorecard={undefined} />);
    expect(container.innerHTML).toBe("");
  });

  it("omits the lead block rather than drawing a rep zeros they did not earn", () => {
    render(<ScorecardPanel scorecard={{ deal: dealBlock }} />);
    expect(
      screen.queryByRole("region", {
        name: en["home.weekly.scorecard.leadBlock"],
      }),
    ).toBeNull();
    // The deal block it DID send is still drawn: this is an omission, not a
    // panel that gave up.
    expect(
      screen.getByRole("region", {
        name: en["home.weekly.scorecard.dealBlock"],
      }),
    ).toBeTruthy();
  });

  it("omits the deal block on its own terms too", () => {
    render(<ScorecardPanel scorecard={{ lead: leadBlock }} />);
    expect(
      screen.queryByRole("region", {
        name: en["home.weekly.scorecard.dealBlock"],
      }),
    ).toBeNull();
    expect(
      screen.getByRole("region", {
        name: en["home.weekly.scorecard.leadBlock"],
      }),
    ).toBeTruthy();
  });

  it("draws a present block whose counts are zero, because that is a finding", () => {
    render(
      <ScorecardPanel
        scorecard={{
          lead: { ...leadBlock, advanced: 0, answered_in_target: 0 },
        }}
      />,
    );
    // Present and zero: the rep HAD leads and moved none, which the panel must
    // say rather than hide.
    expect(screen.getByText(en["home.weekly.scorecard.advanced"])).toBeTruthy();
  });

  it("omits the median when no deal changed stage, never drawing it as zero days", () => {
    render(
      <ScorecardPanel
        scorecard={{ deal: { ...dealBlock, median_days_in_stage: null } }}
      />,
    );
    expect(
      screen.queryByText(en["home.weekly.scorecard.medianDaysInStage"]),
    ).toBeNull();
  });

  it("draws the median when there is one", () => {
    render(<ScorecardPanel scorecard={{ deal: dealBlock }} />);
    expect(
      screen.getByText(en["home.weekly.scorecard.medianDaysInStage"]),
    ).toBeTruthy();
  });

  it("names the open-deal denominator beside each coverage count", () => {
    render(<ScorecardPanel scorecard={{ deal: dealBlock }} />);
    // "3 of 5 open deals" — the count is only judgeable against its
    // denominator, which is why the server sends counts and not a rate.
    expect(
      screen.getAllByText(
        en["home.weekly.scorecard.ofOpen"].replace("{total}", "5"),
      ).length,
    ).toBeGreaterThan(0);
  });

  it("says the meeting counts are a floor only when history is actually missing", () => {
    render(<ScorecardPanel scorecard={{ lead: leadBlock }} />);
    expect(
      screen.queryByText(en["home.weekly.scorecard.partialHistory"]),
    ).toBeNull();

    cleanup();
    render(
      <ScorecardPanel
        scorecard={{ lead: { ...leadBlock, meetings_partial_history: 2 } }}
      />,
    );
    expect(
      screen.getByText(en["home.weekly.scorecard.partialHistory"]),
    ).toBeTruthy();
  });
});
