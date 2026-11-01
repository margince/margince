/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { formatMoneyCompact } from "../../format/format";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import {
  LandingCard,
  ModelRangeCard,
  RecordsLandingCard,
  rangeOf,
  SufficiencyCard,
} from "./analytics.forecast.landing";

type Readings = components["schemas"]["ForecastReadings"];
type Landing = NonNullable<Readings["landing"]>;
type Sufficiency = NonNullable<Readings["sufficiency"]>;

afterEach(cleanup);

function draw(node: ReactNode) {
  return render(<LocaleProvider initial="en">{node}</LocaleProvider>);
}

const landing = (over: Partial<Landing> = {}): Landing => ({
  amount_minor: 900_00,
  measure: "commit_evidence",
  won_minor: 400_00,
  remaining_minor: 500_00,
  ...over,
});

const sufficiency = (over: Partial<Sufficiency> = {}): Sufficiency => ({
  basis: "historical_median",
  reference_landing_minor: 1_000_00,
  remaining_to_support_minor: 600_00,
  needed_open_minor: 2_400_00,
  current_open_minor: 1_200_00,
  coverage_bp: 5_000,
  ...over,
});

describe("the projected landing", () => {
  const money = (minor: number) => formatMoneyCompact(minor, "EUR", "en");

  it("names its basis, then the sum it made, so a reader can check it", () => {
    draw(<LandingCard landing={landing()} currency="EUR" locale="en" />);

    // One label over every measure, with the basis leading the detail.
    expect(screen.getByText("Landing")).toBeTruthy();
    expect(screen.getByText("Won + confirmed commit")).toBeTruthy();
    expect(
      screen.getByText(`${money(400_00)} won + ${money(500_00)} to come`),
    ).toBeTruthy();
  });

  // A call is the whole period's landing, so a reader must not be told it is
  // the remainder; the records' own projection stands beside it.
  it("says a manager's call replaces the projection rather than adding to it", () => {
    draw(
      <LandingCard
        landing={landing({
          measure: "manager_call",
          amount_minor: 900_00,
          remaining_minor: 0,
          system_minor: 650_00,
        })}
        currency="EUR"
        locale="en"
      />,
    );

    expect(screen.getByText("Manager’s call")).toBeTruthy();
    expect(
      screen.getByText(`Records alone project ${money(650_00)}`),
    ).toBeTruthy();
    expect(screen.queryByText(/ to come$/)).toBeNull();
  });

  it("names a standing call beside a projection without adding it", () => {
    draw(
      <LandingCard
        landing={landing()}
        callMinor={1_900_00}
        currency="EUR"
        locale="en"
      />,
    );

    expect(screen.getByText(money(900_00))).toBeTruthy();
    expect(
      screen.getByText(`Manager’s call ${money(1_900_00)}, not added`),
    ).toBeTruthy();
  });

  it("names the fallback when nobody has called the period", () => {
    draw(
      <LandingCard
        landing={landing({ caveat: "call_absent" })}
        currency="EUR"
        locale="en"
      />,
    );

    expect(
      screen.getByText(en["forecast.landing.caveat.call_absent"]),
    ).toBeTruthy();
  });

  it("reports a call below the money already won rather than correcting it", () => {
    draw(
      <LandingCard
        landing={landing({
          measure: "manager_call",
          amount_minor: 100_00,
          caveat: "call_below_actual",
        })}
        currency="EUR"
        locale="en"
      />,
    );

    expect(
      screen.getByText(en["forecast.landing.caveat.call_below_actual"]),
    ).toBeTruthy();
  });
});

describe("whether the open deals support the reference", () => {
  it("keeps the basis in the receipt and out of the two-line detail", async () => {
    const user = userEvent.setup();
    draw(
      <SufficiencyCard
        sufficiency={sufficiency()}
        currency="EUR"
        locale="en"
      />,
    );

    // The detail holds two lines and both are spoken for, so the measure lives
    // ONLY in the receipt: as a fragment on the second line it cost that line a
    // third row, which the clamp then took away in German.
    expect(screen.queryByText(/4-period median/)).toBeNull();

    await user.click(await screen.findByRole("button", { name: "Evidence" }));

    expect(
      await screen.findByText(/median of the last 4 comparable periods/i),
    ).toBeTruthy();
  });

  // The need is the VALUE, so a detail that repeated it said one number twice.
  it("says what is open and what it is reaching for, never the need again", () => {
    draw(
      <SufficiencyCard
        sufficiency={sufficiency()}
        currency="EUR"
        locale="en"
      />,
    );

    // Compact, because a slot is not as wide as an amount.
    expect(screen.getByText("€2,400")).toBeTruthy();
    const detail = screen.getByText(/open ·/);
    expect(detail.textContent).toBe("€1,200 open · to reach €1,000");
    expect(detail.textContent).not.toContain("€2,400");
  });

  it("renders coverage as a whole percent", () => {
    draw(
      <SufficiencyCard
        sufficiency={sufficiency({ coverage_bp: 5_000 })}
        currency="EUR"
        locale="en"
      />,
    );

    expect(screen.getByText(/50% of required deal value/i)).toBeTruthy();
  });

  // The case a zeroed figure would get exactly backwards: no basis must not
  // draw "0 needed, 0 covered", which reads as a fully covered pipeline.
  it("draws no figure at all when there is no basis to measure against", () => {
    draw(
      <SufficiencyCard
        sufficiency={{ absent: "insufficient_basis" }}
        currency="EUR"
        locale="en"
      />,
    );

    expect(
      screen.getByText(en["forecast.pipelineAbsent.insufficient_basis"]),
    ).toBeTruthy();
    expect(screen.queryByText(/of the deal value this needs/i)).toBeNull();
  });

  it("says when the history is too thin for a conversion rate", () => {
    draw(
      <SufficiencyCard
        sufficiency={{ absent: "insufficient_history" }}
        currency="EUR"
        locale="en"
      />,
    );

    expect(
      screen.getByText(en["forecast.pipelineAbsent.insufficient_history"]),
    ).toBeTruthy();
    expect(screen.queryByText(/of the deal value this needs/i)).toBeNull();
  });
});

describe("a manager's call beside the records", () => {
  it("shows the deal records' projection as its own card", () => {
    draw(
      <RecordsLandingCard systemMinor={650_00} currency="EUR" locale="en" />,
    );
    expect(screen.getByText(en["forecast.landingRecords"])).toBeTruthy();
    expect(
      screen.getByText(formatMoneyCompact(650_00, "EUR", "en")),
    ).toBeTruthy();
  });
});

describe("the modelled finish beside the landing", () => {
  const range = { lowMinor: 610_00, highMinor: 880_00 };
  const money = (minor: number) => formatMoneyCompact(minor, "EUR", "en");

  function model(over: Partial<Parameters<typeof ModelRangeCard>[0]> = {}) {
    return (
      <ModelRangeCard
        label="Modelled quarter finish"
        range={range}
        wonMinor={400_00}
        unpriced={0}
        fxMissing={0}
        money={money}
        {...over}
      />
    );
  }

  it("is its own reading, never a line under the landing", () => {
    const { container } = draw(
      <LandingCard landing={landing()} currency="EUR" locale="en" />,
    );
    expect(container.textContent).not.toMatch(/80%|8 in 10| to €/);
  });

  it("states the central 80% of modelled outcomes and the won money inside it", () => {
    draw(model());
    expect(
      screen.getByText(`${money(610_00)} to ${money(880_00)}`),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "Central 80% of modelled outcomes under current chances",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(`Includes ${money(400_00)} already won`),
    ).toBeTruthy();
  });

  it("says which deals it leaves out", () => {
    draw(model({ unpriced: 2, fxMissing: 1 }));
    expect(screen.getByText("2 unpriced deals left out")).toBeTruthy();
  });

  it("says it assumes the chances are right, in the receipt", async () => {
    const user = userEvent.setup();
    draw(model({ unpriced: 2, fxMissing: 1 }));

    await user.click(await screen.findByRole("button", { name: "Evidence" }));

    expect(
      await screen.findByText(/it is not a measured forecast accuracy\.$/),
    ).toBeTruthy();
    expect(
      screen.getByText("1 deal without an exchange rate left out"),
    ).toBeTruthy();
  });

  it("leads to the deals it was drawn from", async () => {
    const user = userEvent.setup();
    const onOpen = vi.fn();
    draw(model({ onOpen }));

    await user.click(screen.getByRole("button", { name: "Open" }));

    expect(onOpen).toHaveBeenCalledOnce();
  });

  it("says when no range was drawn", () => {
    draw(model({ range: undefined, onOpen: vi.fn() }));
    expect(screen.getByText("No range")).toBeTruthy();
    expect(
      screen.getByText("No modelled range was drawn for this period."),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Open" })).toBeNull();
  });

  it("reads equal ends as one amount", () => {
    draw(model({ range: { lowMinor: 420_00, highMinor: 420_00 } }));
    expect(screen.getByText(money(420_00))).toBeTruthy();
  });
});

describe("rangeOf", () => {
  it("keeps a range only when both ends arrived", () => {
    expect(rangeOf(610_00, 880_00)).toEqual({
      lowMinor: 610_00,
      highMinor: 880_00,
    });
    expect(rangeOf(610_00, undefined)).toBeUndefined();
    expect(rangeOf(undefined, 880_00)).toBeUndefined();
  });

  it("keeps equal ends, so the reading can say nothing is left to model", () => {
    expect(rangeOf(420_00, 420_00)).toEqual({
      lowMinor: 420_00,
      highMinor: 420_00,
    });
  });
});
