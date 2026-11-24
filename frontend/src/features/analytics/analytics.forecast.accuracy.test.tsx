/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { jsonResponse } from "../../testing/company.fixtures";
import { ForecastAccuracy } from "./analytics.forecast.accuracy";

type Accuracy = components["schemas"]["ForecastAccuracy"];

function show(accuracy: Accuracy) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      jsonResponse({
        self_correcting: true,
        evidence_cutoff: "2026-11-22T00:00:00Z",
        prior_strength: 20,
        stages: [],
        accuracy,
      }),
    ),
  );
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <ForecastAccuracy />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("how accurate were we", () => {
  it("sets the share won per chance band and each quarter's landing against what was won", async () => {
    show({
      calibration: {
        deals: 80,
        floor: 20,
        lead_days: 14,
        bands: [
          { from_percent: 0, to_percent: 9, deals: 3 },
          {
            from_percent: 60,
            to_percent: 69,
            deals: 40,
            mean_percent: 64,
            won_percent: 55,
          },
        ],
      },
      landings: [
        {
          period_start: "2026-07-01",
          period_end: "2026-09-30",
          currency: "EUR",
          said_minor: 120000000,
          landed_minor: 90000000,
          said_on: "2026-09-02",
        },
      ],
    });
    expect(
      await screen.findByText("Chance 64%, won 55% (40 deals)"),
    ).toBeTruthy();
    expect(screen.getByText("3 deals, too few to state a share")).toBeTruthy();
    expect(screen.getByText(/Said .*1,200,000.*won .*900,000/)).toBeTruthy();
  });

  it("says there is not enough history while nothing has closed", async () => {
    show({
      calibration: { deals: 0, floor: 20, lead_days: 14, bands: [] },
      landings: [],
    });
    expect(await screen.findByText(/Not enough closed deals yet/)).toBeTruthy();
  });
});
