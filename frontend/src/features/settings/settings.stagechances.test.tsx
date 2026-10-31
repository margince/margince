/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { StageChanceLine, type StageChances } from "./settings.stages";

type Stage = components["schemas"]["Stage"];

afterEach(cleanup);

const STAGE_ID = "018f0000-0000-7000-8000-0000000000aa";
const stage = { id: STAGE_ID, semantic: "open", win_probability: 90 } as Stage;

function chances(
  selfCorrecting: boolean,
  over: Partial<components["schemas"]["ForecastStageChance"]> = {},
): StageChances {
  return {
    selfCorrecting,
    byStage: new Map([
      [
        STAGE_ID,
        {
          stage_id: STAGE_ID,
          typed_probability: 90,
          evidence_n: 40,
          won_percent: 55,
          chance_in_use: 62,
          ...over,
        },
      ],
    ]),
  };
}

function line(of: StageChances) {
  render(
    <LocaleProvider initial="en">
      <StageChanceLine stage={stage} chances={of} />
    </LocaleProvider>,
  );
}

describe("StageChanceLine", () => {
  it("names the typed chance, the recorded deals and the chance in use", () => {
    line(chances(true));
    expect(
      screen.getByText(
        "Typed 90% · 40 recorded deals from here closed, 55% won · Forecast uses 62%",
      ),
    ).toBeTruthy();
  });

  it("says no recorded deal has closed yet", () => {
    line(
      chances(true, {
        evidence_n: 0,
        won_percent: undefined,
        chance_in_use: 90,
      }),
    );
    expect(
      screen.getByText(
        "Typed 90% · No recorded deal from here has closed yet · Forecast uses 90%",
      ),
    ).toBeTruthy();
  });

  it("says the typed chance is used while self-correction is off", () => {
    line(chances(false));
    expect(
      screen.getByText("Forecast uses the typed 90%: self-correction is off"),
    ).toBeTruthy();
  });
});
