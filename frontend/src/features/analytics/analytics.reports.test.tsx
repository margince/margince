/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import {
  formatMoney,
  formatMoneyCompact,
  MONEY_ABSENT,
} from "../../format/format";
import { LocaleProvider } from "../../i18n";
import { vi as viCatalog } from "../../i18n/vi";
import { AnalyticsScreen, buildStageAggregates } from "./analytics";
import { SECTION_REPORTS } from "./analytics.reports";
import {
  openPipeline,
  render,
  reportsStub,
  stageTable,
} from "./analytics.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type Stage = components["schemas"]["Stage"];

const SECTION_REPORT_COUNT_PIPELINE = SECTION_REPORTS.pipeline.length;

// Money never sums across currencies (AC-DS-FX1). Every plan this screen runs
// groups by currency, so a stage or a category holding two currencies has two
// totals and the screen shows both. It must never show one figure that added
// them, or a currency it invented for a figure the server sent without one.
describe("reports never sum money across currencies", () => {
  // Out of pipeline order, so the ordering assertions below prove a sort
  // rather than an echo of the fixture.
  const stage = (id: string, name: string, position: number): Stage => ({
    id,
    pipeline_id: "pl",
    name,
    position,
    semantic: "open",
    win_probability: 20,
  });
  const STAGES: Stage[] = [
    stage("pl-s2", "Propose", 2),
    stage("pl-s1", "Qualify", 1),
  ];

  it("asks the server for the CONVERTED measures, grouped by stage alone", async () => {
    const bodies: { key: string; body: Record<string, unknown> }[] = [];
    vi.stubGlobal(
      "fetch",
      reportsStub({ onRun: (key, body) => bodies.push({ key, body }) }),
    );
    render(<AnalyticsScreen />);
    await openPipeline();
    await waitFor(() => expect(screen.getByText("Qualify")).toBeTruthy());
    const stagePlan = bodies.find((sent) => sent.key === "pipeline-current");
    // No currency in the grouping. The server converted each deal before
    // summing, so a stage is one row rather than one per currency it trades in.
    expect(stagePlan?.body).toMatchObject({ group_by: ["stage_id"] });
    expect(stagePlan?.body.aggregates).toEqual([
      { fn: "sum", field: "amount_base_minor", as: "raw_minor" },
      { fn: "sum", field: "weighted_base_minor", as: "weighted_minor" },
      { fn: "count", as: "deal_count" },
      { fn: "count", field: "amount_base_minor", as: "priced_deals" },
    ]);
  });

  // The money plans that are still native keep their currency grouping. The
  // company table sums amount_minor, and a total spanning currencies there
  // would be a number with no unit.
  //
  // Every plan is checked, not a named list. A list would pass after a plan
  // was converted and its entry removed. It would also pass after a new plan
  // summed native money nobody thought to list. The rule is about what a plan
  // asks for: sum a native minor-unit column and currency must be grouped.
  //
  // The scan reads the field name, so it is as strong as the convention behind
  // it. Every money measure the report engine offers ends in `_minor`
  // (`amount_minor`, `weighted_amount_minor`) or `_base_minor` (converted).
  // Nothing on the server enforces that: a measure named `amount_cents` would
  // sum across currencies with this test still green. That needs a new money
  // measure spelled outside the convention. The fix then is the server
  // refusing the combination (#4131), not a longer suffix list here.
  it("groups every native money plan by currency", async () => {
    const bodies: { key: string; body: Record<string, unknown> }[] = [];
    vi.stubGlobal(
      "fetch",
      reportsStub({ onRun: (key, body) => bodies.push({ key, body }) }),
    );
    render(<AnalyticsScreen />);
    // Every section, so a report added to a new section enters this census by
    // construction rather than by somebody remembering to widen the walk.
    await openPipeline();
    await waitFor(() => expect(screen.getByText("Qualify")).toBeTruthy());

    expect(bodies.length).toBeGreaterThan(0);
    let nativePlans = 0;
    for (const { key, body } of bodies) {
      const aggregates = (body.aggregates ?? []) as { field?: string }[];
      const sumsNative = aggregates.some(
        (aggregate) =>
          aggregate.field?.endsWith("_minor") &&
          !aggregate.field.endsWith("_base_minor"),
      );
      if (!sumsNative) {
        continue;
      }
      nativePlans += 1;
      expect(
        body.group_by,
        `${key} sums a native minor-unit column without grouping by currency`,
      ).toContain("currency");
    }
    // A scan that matched nothing would pass over a screen where every plan
    // had gone native.
    expect(nativePlans).toBeGreaterThan(0);
  });

  it("draws one row per stage, in the installation's base currency", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        // What the converted report returns: one row for the stage, its money
        // already added up across the currencies the deals were written in.
        stageRows: [
          {
            stage_id: "pl-s1",
            raw_minor: 250_000,
            weighted_minor: 50_000,
            deal_count: 5,
          },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await openPipeline();

    expect(
      (await stageTable()).getByText(formatMoney(250_000, "EUR", "en")),
    ).toBeTruthy();
    // One stage, one row: the count cell appears once.
    expect(screen.getAllByText("Qualify")).toHaveLength(1);
  });

  it("renders an unpriced stage as absent rather than as zero euros", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        stageRows: [
          {
            stage_id: "pl-s1",
            raw_minor: null,
            weighted_minor: null,
            deal_count: 4,
          },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await openPipeline();
    await waitFor(() => expect(screen.getByText("Qualify")).toBeTruthy());
    // The count is real and stays; only the money is unknown.
    expect((await stageTable()).getByText("4")).toBeTruthy();
    expect(screen.getAllByText(MONEY_ABSENT).length).toBeGreaterThan(0);
    expect(screen.queryByText(formatMoney(0, "EUR", "en"))).toBeNull();
  });

  it("renders a forecast category with no deals as absent rather than as zero euros", async () => {
    vi.stubGlobal("fetch", reportsStub({ forecastRows: [] }));
    render(<AnalyticsScreen />);
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "Pipeline" }));
    await waitFor(() =>
      expect(screen.getAllByText(MONEY_ABSENT).length).toBeGreaterThan(0),
    );
    expect(screen.queryByText(formatMoney(0, "EUR", "en"))).toBeNull();
  });

  // One stage is one row: the server converts each deal before summing, so
  // there is no per-currency split left to order within a stage.
  it("orders stage rows down the pipeline", () => {
    const rows = [
      { stage_id: "pl-s2", raw_minor: 1, deal_count: 1 },
      { stage_id: "pl-s1", raw_minor: 1, deal_count: 1 },
    ];
    expect(
      buildStageAggregates(rows, STAGES).map((row) => row.stageName),
    ).toEqual(["Qualify", "Propose"]);
  });

  it("sorts a row whose stage the pipeline no longer carries to the end", () => {
    const rows = [
      { stage_id: "gone", raw_minor: 1, deal_count: 1 },
      { stage_id: "pl-s1", raw_minor: 1, deal_count: 1 },
    ];
    expect(
      buildStageAggregates(rows, STAGES).map((row) => row.stageId),
    ).toEqual(["pl-s1", "gone"]);
  });

  it("keeps an absent measure absent rather than reading it as zero", () => {
    const [row] = buildStageAggregates(
      [
        {
          stage_id: "pl-s1",
          raw_minor: null,
          weighted_minor: null,
          deal_count: 3,
        },
      ],
      STAGES,
    );
    expect(row.rawMinor).toBeNull();
    expect(row.weightedMinor).toBeNull();
    // A count of zero would be a claim; the server did send this one.
    expect(row.count).toBe(3);
  });

  // priced_deals answers how many of `deal_count` the sums above cover. A deal
  // in a currency the rate sheet cannot price is counted but priced nowhere in
  // either total (margince#4201).
  it("carries how many of a stage's deals the money actually covers", () => {
    const [row] = buildStageAggregates(
      [
        {
          stage_id: "pl-s1",
          raw_minor: 100,
          weighted_minor: 90,
          deal_count: 2,
          priced_deals: 1,
        },
      ],
      STAGES,
    );
    expect(row.pricedDeals).toBe(1);
  });

  // A row that omits priced_deals answers "the server did not say", not "zero
  // deals were priced". Folding the two together would print a claim nobody
  // made.
  it("answers null rather than zero when a stage row omits priced_deals", () => {
    const [row] = buildStageAggregates(
      [
        {
          stage_id: "pl-s1",
          raw_minor: 100,
          weighted_minor: 90,
          deal_count: 2,
        },
      ],
      STAGES,
    );
    expect(row.pricedDeals).toBeNull();
  });

  it("shows a stage row's money is short a deal the rate sheet cannot price", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        stageRows: [
          {
            stage_id: "pl-s1",
            raw_minor: 100,
            weighted_minor: 90,
            deal_count: 2,
            priced_deals: 1,
          },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await openPipeline();
    expect(
      (await stageTable()).getByText((text) => text.includes("1 of 2 priced")),
    ).toBeTruthy();
  });

  it("shows no priced footnote when the row omits priced_deals entirely", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        stageRows: [
          {
            stage_id: "pl-s1",
            raw_minor: 100,
            weighted_minor: 90,
            deal_count: 2,
          },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await openPipeline();
    await waitFor(() => expect(screen.getByText("Qualify")).toBeTruthy());
    expect(screen.queryByText((text) => text.includes("priced"))).toBeNull();
  });

  it("shows no priced footnote once every counted deal was priced", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        stageRows: [
          {
            stage_id: "pl-s1",
            raw_minor: 100,
            weighted_minor: 90,
            deal_count: 2,
            priced_deals: 2,
          },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await openPipeline();
    await waitFor(() => expect(screen.getByText("Qualify")).toBeTruthy());
    expect(screen.queryByText((text) => text.includes("priced"))).toBeNull();
  });

  // The wire allows a deal to carry no forecast category: nobody has said which
  // way it is going. The five named categories match none of it. A tile set
  // built from the enum alone drops those deals off the screen: the money is
  // not moved to another tile, it leaves. On the demo dataset that is 22 of 27
  // open deals.
  it("shows the deals that carry no forecast category at all", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        // Converted rows: one per category, the server having already summed
        // each deal in the base currency. This test is about the uncategorised
        // row: on the demo dataset it is 22 of 27 open deals. A slot set built
        // from the named enum alone does not move that money to another tile;
        // it drops it off the screen.
        forecastRows: [
          {
            forecast_category: "omitted",
            raw_minor: 58_860_000,
            weighted_minor: 31_502_500,
            deal_count: 5,
          },
          {
            forecast_category: null,
            raw_minor: 202_720_000,
            weighted_minor: 76_460_000,
            deal_count: 17,
          },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "Pipeline" }));

    expect(
      await screen.findByText(formatMoneyCompact(202_720_000, "EUR", "en")),
    ).toBeTruthy();
    // And never folded into a named category: the two totals added together
    // is the number that must not appear anywhere.
    expect(
      screen.queryByText(formatMoneyCompact(261_580_000, "EUR", "en")),
    ).toBeNull();
  });

  // The forecast is one comparison, not one per currency.
  //
  // A band per currency would be correct for native sums. But a manager would
  // then compare commit against best case inside each currency, never across
  // the business. The server converts, so there is one denomination. The proof is
  // that no currency code appears as a heading over the tiles.
  it("draws the categories as one comparison, in the base currency", async () => {
    const bodies: { key: string; body: Record<string, unknown> }[] = [];
    vi.stubGlobal(
      "fetch",
      reportsStub({
        onRun: (key, body) => bodies.push({ key, body }),
        forecastRows: [
          {
            forecast_category: "commit",
            raw_minor: 2_500_000,
            weighted_minor: 250_000,
            deal_count: 1,
          },
          {
            forecast_category: "best_case",
            raw_minor: 920_000,
            weighted_minor: 460_000,
            deal_count: 1,
          },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "Pipeline" }));

    // Both categories, both figures, in the one base currency and compact.
    const compact = (minor: number) => formatMoneyCompact(minor, "EUR", "en");
    expect(await screen.findByText(compact(2_500_000))).toBeTruthy();
    expect(screen.getByText(compact(920_000))).toBeTruthy();

    // The plan asks for converted money and does not group by currency. Those
    // two go together: a request that changed one without the other would
    // either band a single denomination or sum minor units across several.
    const plan = bodies.find((sent) => sent.key === "forecast");
    expect(plan?.body.group_by).toEqual(["forecast_category"]);
    expect(plan?.body.aggregates).toContainEqual({
      fn: "sum",
      field: "amount_base_minor",
      as: "raw_minor",
    });
  });

  // A converted total skips a deal the rate sheet cannot price. So the tile
  // says how many deals it is about, as well as what they are worth.
  //
  // The sum has no sound alternative: guessing a rate would invent money. But
  // a tile printing the total alone would give €25,000 as the category's
  // worth. It holds two deals, and one was left out. The count makes that
  // visible: two deals, one figure.
  it("says how many deals a category holds, not only what it is worth", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        forecastRows: [
          {
            forecast_category: "commit",
            // Two deals; one is in a currency the sheet cannot price, so it
            // contributes nothing to the sum and is still counted.
            raw_minor: 2_500_000,
            weighted_minor: 250_000,
            deal_count: 2,
            priced_deals: 1,
          },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "Pipeline" }));

    expect(
      await screen.findByText(formatMoneyCompact(2_500_000, "EUR", "en")),
    ).toBeTruthy();
    // Fragments on the tile's second line, never "Label: value" pairs: a
    // caption is read as one sentence about the figure above it. Where the
    // money covers every deal the count says so; here it does not. So the count
    // states the gap. Without it the total reads as covering both deals when
    // it covers one.
    const [detail] = screen.getAllByText((text) =>
      text.includes("weighted · "),
    );
    expect(detail.textContent).toBe(
      `${formatMoneyCompact(250_000, "EUR", "en")} weighted · 1 of 2 priced`,
    );
  });

  // An installation whose deals are all categorised should not be shown an empty
  // sixth tile asking about a state it never reaches.
  it("draws no uncategorised tile when every deal has a category", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        forecastRows: [
          {
            forecast_category: "commit",
            currency: "EUR",
            raw_minor: 1000,
            weighted_minor: 900,
            deal_count: 1,
          },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "Pipeline" }));
    await waitFor(() =>
      expect(
        screen.getByText(formatMoneyCompact(1000, "EUR", "en")),
      ).toBeTruthy(),
    );
    expect(screen.queryByText("No category yet")).toBeNull();
  });
});

describe("the report frame", () => {
  // A total with no zone beside it is a number the reader places by
  // assumption, and the assumption is their own zone.
  it("names the instant and the zone the figures were cut in", async () => {
    vi.stubGlobal("fetch", reportsStub());
    render(<AnalyticsScreen />);
    await openPipeline();
    // One per report, not one per screen: each result carries its own frame.
    // A section holding two reports could be showing two results computed
    // moments apart. A single caption over both would claim they share an
    // instant they do not.
    const captions = await screen.findAllByText(
      /Central European .* Time · UTC\+[12]/,
    );
    expect(captions).toHaveLength(SECTION_REPORT_COUNT_PIPELINE);
  });

  it("writes the zone as its long name and an offset, never the IANA id", async () => {
    vi.stubGlobal("fetch", reportsStub());
    render(
      <LocaleProvider initial="vi">
        <AnalyticsScreen />
      </LocaleProvider>,
    );
    await userEvent.click(
      await screen.findByRole("button", {
        name: viCatalog["reporting.pipeline"],
      }),
    );
    expect(
      (await screen.findAllByText(/Giờ .* Trung Âu · UTC\+[12]/)).length,
    ).toBeGreaterThan(0);
    expect(screen.queryByText(/Europe\/Berlin/)).toBeNull();
  });

  // And it names no currency, because the blocks under the frame do not share
  // one. A code in the frame would read as the denomination of all of them.
  // The figures on screen here are EUR and USD at once. A reader taking such
  // a caption at its word reads the USD total as euros.
  it("claims no currency in the frame, because the blocks under it differ", async () => {
    vi.stubGlobal("fetch", reportsStub());
    render(<AnalyticsScreen />);
    await openPipeline();

    // The frame sits under all three blocks, and they are not in one currency.
    // The stage table is converted into the base currency, while the forecast
    // strip and the company table are still per-currency native sums. A code in
    // the frame would be the denomination of only one of the three.
    const captions = await screen.findAllByText(
      /Central European .* Time · UTC\+[12]/,
    );
    for (const caption of captions) {
      expect(caption.textContent).not.toMatch(/\b(EUR|USD|VND)\b/);
    }
  });

  // The converted table says its own currency, in the line under its title,
  // where the statement is true of every figure beneath it.
  it("names the base currency on the converted stage table", async () => {
    vi.stubGlobal("fetch", reportsStub());
    render(<AnalyticsScreen />);
    await openPipeline();

    expect(await screen.findByText(/converted to EUR/)).toBeTruthy();
  });

  // A server mid-upgrade sends a partial frame. Naming one of the two would be
  // worse than naming none, so the caption is drawn or it is not.
  it("draws no caption at all when the server sent only part of the frame", async () => {
    vi.stubGlobal("fetch", reportsStub({ partialFrame: true }));
    render(<AnalyticsScreen />);
    await openPipeline();
    // The report still renders. That is the half that makes this test mean
    // something. A guard checking only `as_of` also produces no caption here.
    // It does so by throwing inside formatDateTime, which takes the whole card
    // down and leaves the reader with no figures. Asserting the absence of the
    // caption alone would pass against that.
    await waitFor(() => expect(screen.getByText("Qualify")).toBeTruthy());
    expect(screen.queryByText(/Europe\/Berlin/)).toBeNull();
    expect(screen.queryByText(/As of/)).toBeNull();
  });
});
