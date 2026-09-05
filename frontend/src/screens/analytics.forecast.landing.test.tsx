/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { LandingCard, SufficiencyCard } from "./analytics.forecast.landing";

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
  it("says the sum it made, so a reader can check the arithmetic", () => {
    draw(<LandingCard landing={landing()} currency="EUR" locale="en" />);

    // The two halves, both named. A figure alone leaves a reader to guess
    // which readings were added, and the wrong guess is Won plus Best case.
    const detail = screen.getByText(/already won plus/i);
    expect(detail.textContent).toContain("400");
    expect(detail.textContent).toContain("500");
  });

  // The misreading this shape exists to prevent: a call is the whole period's
  // landing, so a reader must not be told it is the remainder.
  it("says a manager's call replaces the projection rather than adding to it", () => {
    draw(
      <LandingCard
        landing={landing({
          measure: "manager_call",
          amount_minor: 900_00,
          remaining_minor: 0,
        })}
        currency="EUR"
        locale="en"
      />,
    );

    expect(
      screen.getByText(/replaces the projection rather than adding/i),
    ).toBeTruthy();
    expect(screen.queryByText(/already won plus/i)).toBeNull();
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

describe("whether the pipeline supports the reference", () => {
  it("names the basis, so a reader can disagree with it rather than the sum", () => {
    draw(
      <SufficiencyCard
        sufficiency={sufficiency()}
        currency="EUR"
        locale="en"
      />,
    );

    expect(
      screen.getByText(/median of the last four comparable periods/i),
    ).toBeTruthy();
  });

  it("renders coverage as a whole percent", () => {
    draw(
      <SufficiencyCard
        sufficiency={sufficiency({ coverage_bp: 5_000 })}
        currency="EUR"
        locale="en"
      />,
    );

    expect(screen.getByText(/50% of the pipeline this needs/i)).toBeTruthy();
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
    expect(screen.queryByText(/of the pipeline this needs/i)).toBeNull();
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
    expect(screen.queryByText(/of the pipeline this needs/i)).toBeNull();
  });
});
