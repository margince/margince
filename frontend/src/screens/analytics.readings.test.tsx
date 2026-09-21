/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { formatMoneyCompact } from "../format/format";
import { en } from "../i18n/en";
import { AnalyticsScreen, ForecastTile } from "./analytics";
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
      `${formatMoneyCompact(125_000, "EUR", "en")} weighted · ${deals("4")}`,
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

    expect(
      await screen.findAllByText(en["analytics.readingLoading"]),
    ).toHaveLength(2);
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
});
