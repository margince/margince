// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  openingView,
  overviewViews,
  recommendedView,
} from "./analytics.overview.choice";

const both = ["sales", "prospecting"] as const;

describe("which Overview view opens", () => {
  it("offers a view only where the catalog publishes its preset", () => {
    expect(
      overviewViews({
        presets: [{ id: "sdr", metrics: ["meetings_held"], blocks: [] }],
      }),
    ).toEqual(["prospecting"]);
    expect(overviewViews({ presets: [] })).toEqual([]);
  });

  it("offers Team prospecting only to a reader whose lens covers colleagues", () => {
    const catalog = {
      presets: [
        { id: "sdr" as const, metrics: ["meetings_held" as const], blocks: [] },
        {
          id: "team" as const,
          metrics: ["meetings_held" as const],
          blocks: [],
        },
      ],
    };
    expect(overviewViews(catalog, true)).toEqual(["prospecting", "team"]);
    expect(overviewViews(catalog, false)).toEqual(["prospecting"]);
  });

  it("recommends prospecting to a reader measuring their own work in an SDR team", () => {
    expect(
      recommendedView(both, {
        sdrFramework: true,
        defaultScope: "owner",
        submitsForecast: false,
      }),
    ).toBe("prospecting");
  });

  // The same role can do either job, so the recommendation reads the work.
  it("recommends the sales review to a reader whose lens covers a team", () => {
    expect(
      recommendedView(both, {
        sdrFramework: true,
        defaultScope: "managed_teams",
        submitsForecast: false,
      }),
    ).toBe("sales");
    expect(
      recommendedView(both, {
        sdrFramework: false,
        defaultScope: "owner",
        submitsForecast: false,
      }),
    ).toBe("sales");
    expect(
      recommendedView(both, {
        sdrFramework: true,
        defaultScope: "owner",
        submitsForecast: true,
      }),
    ).toBe("sales");
  });

  it("recommends the one view a catalog offers", () => {
    expect(
      recommendedView(["prospecting"], {
        sdrFramework: false,
        defaultScope: "workspace",
        submitsForecast: true,
      }),
    ).toBe("prospecting");
  });

  it("opens the address's view, then the remembered one, then the recommended one", () => {
    expect(openingView(both, "prospecting", "sales", "sales")).toBe(
      "prospecting",
    );
    expect(openingView(both, undefined, "prospecting", "sales")).toBe(
      "prospecting",
    );
    expect(openingView(both, null, null, "sales")).toBe("sales");
  });

  it("passes over a named view the catalog does not offer", () => {
    expect(openingView(["sales"], "prospecting", "prospecting", "sales")).toBe(
      "sales",
    );
    expect(openingView(both, "constructor", null, "sales")).toBe("sales");
  });
});
