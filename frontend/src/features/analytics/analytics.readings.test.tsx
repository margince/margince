// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { formatAnalyticsMoney } from "../../format/format";
import { en } from "../../i18n/en";
import { AnalyticsScreen, ForecastTile } from "./analytics";
import { ForecastStrip } from "./analytics.pipeline";
import { ownLensContext, render, reportsStub } from "./analytics.testkit";

// What a reading says when it has no figure to say.
//
// Its own file because `analytics.test.tsx` is already past the thousand-line
// ceiling, and these are the states that surface exists for: a slot compared
// across a row must answer in words, and the words have to say WHICH absence
// this is — a read in flight, a read that failed and a lens that came back with
// nothing are three facts, and one of them resolves by waiting.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The real server for every read this screen makes, with ONE report answered
// differently. Composed rather than rewritten: a hand-built stub here would be
// a second model of the report endpoint, and the two would drift.
function pipelineAnswers(answer: () => Promise<Response>) {
  const rest = reportsStub({ context: ownLensContext });
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input instanceof Request ? input.url : input);
    return url.includes("/reports/pipeline-current")
      ? answer()
      : rest(input, init);
  });
}

async function openOutcomes() {
  render(<AnalyticsScreen />);
  await userEvent
    .setup()
    .click(await screen.findByRole("button", { name: "My outcomes" }));
}

// The catalog's own words with the count filled in as the renderer fills it: a
// literal here would go on passing while the catalog said something else.
const deals = (count: string) =>
  en["analytics.forecastDeals_other"].replace("{count}", count);

describe("a forecast tile with no money to state", () => {
  it("answers with the deals it holds rather than with a glyph", () => {
    render(
      <ForecastTile
        label="Commit"
        amountMinor={null}
        dealCount={3}
        currency="EUR"
        locale="en"
      />,
    );

    // The category HAS deals; what it has no figure for is their value, and
    // that is the caption's job rather than the reading's.
    expect(screen.getByText(deals("3"))).toBeTruthy();
    expect(screen.getByText(en["analytics.forecastNoAmount"])).toBeTruthy();
  });

  // One deal is not "1 deals". The count picks the arm and the reader's own
  // plural rule decides which, so no call site can spell it wrong.
  it("counts a single deal in the singular", () => {
    render(
      <ForecastTile
        label="Commit"
        amountMinor={null}
        dealCount={1}
        currency="EUR"
        locale="en"
      />,
    );

    expect(screen.getByText(en["analytics.forecastDeals_one"])).toBeTruthy();
    expect(screen.queryByText(deals("1"))).toBeNull();
  });

  it("says no deals when the report returned no row at all", () => {
    render(
      <ForecastTile
        label="Commit"
        amountMinor={null}
        dealCount={null}
        currency="EUR"
        locale="en"
      />,
    );

    // Nothing was measured, so there is nothing to be zero of.
    expect(screen.getByText(en["analytics.forecastNoFigure"])).toBeTruthy();
    expect(screen.queryByText(en["analytics.forecastNoAmount"])).toBeNull();
  });

  it("counts the deals plainly where the money covers all of them", () => {
    render(
      <ForecastTile
        label="Commit"
        amountMinor={250_000}
        weightedMinor={125_000}
        dealCount={4}
        pricedDeals={4}
        currency="EUR"
        locale="en"
      />,
    );

    const detail = screen.getByText(/weighted/);
    expect(detail.textContent).toBe(
      `${formatAnalyticsMoney(125_000, "EUR", "en")} weighted · ${deals("4")}`,
    );
  });
});

describe("the seat's own readings when the report answers with nothing", () => {
  it("says a read is still in flight rather than that there is nothing", async () => {
    // A request that never settles IS the in-flight state, with no clock and
    // nothing to wait out.
    vi.stubGlobal(
      "fetch",
      pipelineAnswers(() => new Promise<Response>(() => {})),
    );
    await openOutcomes();

    await waitFor(() =>
      expect(screen.getAllByText(en["analytics.readingLoading"])).toHaveLength(
        2,
      ),
    );
    expect(screen.queryByText(en["analytics.readingNone"])).toBeNull();
  });

  it("says a read failed rather than that there is nothing", async () => {
    vi.stubGlobal(
      "fetch",
      pipelineAnswers(async () => new Response("", { status: 500 })),
    );
    await openOutcomes();

    // Waiting will not fix this one, and a card that said "None" would send a
    // rep looking for deals that are there.
    await waitFor(() =>
      expect(
        screen.getAllByText(en["analytics.readingUnavailable"]),
      ).toHaveLength(2),
    );
  });

  it("says none when the lens answered and the seat holds nothing", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({ context: ownLensContext, stageRows: [] }),
    );
    await openOutcomes();

    expect(
      await screen.findAllByText(en["analytics.readingNone"]),
    ).toHaveLength(2);
    expect(screen.queryByText(en["analytics.readingUnavailable"])).toBeNull();
  });

  it("names the missing currency rather than drawing a dash for the value", async () => {
    // An installation mid-upgrade: rows, and a frame with no base currency on
    // it. Money with no currency is not money, and a euro sign on a figure
    // that might be dong is worse than no figure.
    vi.stubGlobal(
      "fetch",
      reportsStub({
        context: ownLensContext,
        partialFrame: true,
        stageRows: [{ deal_count: 4, raw_minor: 250_000 }],
      }),
    );
    await openOutcomes();

    expect(
      await screen.findByText(en["analytics.baseValueUnnamed"]),
    ).toBeTruthy();
    expect(screen.getByText(en["analytics.noBaseCurrency"])).toBeTruthy();
    expect(screen.getByText(en["analytics.noBaseCurrencyWhy"])).toBeTruthy();
  });

  // The OTHER absence, and the reason it is not the one above: the currency is
  // there and the sum is not. "Currency not set" here would send an admin to a
  // setting that is already filled.
  it("says the amount is missing when the currency is not", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        context: ownLensContext,
        stageRows: [{ deal_count: 4, raw_minor: null }],
      }),
    );
    await openOutcomes();

    expect(
      await screen.findByText(
        en["analytics.baseValue"].replace("{currency}", "EUR"),
      ),
    ).toBeTruthy();
    expect(screen.getByText(en["analytics.forecastNoAmount"])).toBeTruthy();
    expect(screen.queryByText(en["analytics.noBaseCurrencyWhy"])).toBeNull();
  });

  it("states the figure when the row carries both halves", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        context: ownLensContext,
        stageRows: [{ deal_count: 4, raw_minor: 250_000 }],
      }),
    );
    await openOutcomes();

    expect(
      await screen.findByText(formatAnalyticsMoney(250_000, "EUR", "en")),
    ).toBeTruthy();
    expect(screen.queryByText(en["analytics.noBaseCurrency"])).toBeNull();
    expect(screen.queryByText(en["analytics.forecastNoAmount"])).toBeNull();
  });
});

// Five tiles partition the open pipeline into probability bands, and "which
// deals are in this band" is the first question anyone asks of one. The door is
// what makes the figure a way in rather than a dead end.
describe("a forecast tile's door", () => {
  it("opens the deals the band holds", () => {
    render(
      <ForecastTile
        label="60 to 89%"
        amountMinor={120000}
        currency="EUR"
        locale="en"
        dealsUrl="#/deals?win_probability=60..89"
      />,
    );

    const door = screen.getByRole("link", { name: en["stat.open"] });
    expect(door.getAttribute("href")).toBe("/#/deals?win_probability=60..89");
  });

  // Through the strip, because the decision that matters is the strip's. It
  // picks which bands get an address at all. A tile handed no url draws no
  // door whatever the reason, so testing the tile alone would pass against a
  // strip that addressed every band.
  it("is drawn for every known band and withheld from one the server added", () => {
    render(
      <ForecastStrip
        baseCurrency="EUR"
        locale="en"
        rows={[
          { win_probability_band: "60_89", raw_minor: 120000, deal_count: 2 },
          { win_probability_band: "95_plus", raw_minor: 90000, deal_count: 1 },
        ]}
      />,
    );

    const doors = screen
      .getAllByRole("link", { name: en["stat.open"] })
      .map((door) => door.getAttribute("href"));

    // A slot per known band whether a row came back for it or not. The
    // unknown band keeps its money on screen with no door.
    expect(doors).toEqual([
      "/#/deals?win_probability=90..100",
      "/#/deals?win_probability=60..89",
      "/#/deals?win_probability=30..59",
      "/#/deals?win_probability=1..29",
      "/#/deals?win_probability=0..0",
    ]);
    expect(screen.getByText("95_plus")).toBeTruthy();
  });

  // A tile whose set no query parameter names draws none. A door onto a wider
  // set would promise more than it delivers, which the deals address module
  // refuses.
  it("is absent on a tile with nowhere to go", () => {
    render(
      <ForecastTile
        label="95_plus"
        amountMinor={120000}
        currency="EUR"
        locale="en"
      />,
    );

    expect(screen.queryByRole("link", { name: en["stat.open"] })).toBeNull();
  });
});
