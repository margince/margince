// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { translate, translatePlural, type Words } from "../../i18n";
import { reportingStoryEvaluation } from "./reporting.fixtures";
import { writtenSummary } from "./reporting.written";

const english: Words = {
  t: (key, params) => translate("en", key, params),
  plural: (base, count, params) => translatePlural("en", base, count, params),
  locale: "en",
};

type Evaluation = typeof reportingStoryEvaluation;
type Metric = Evaluation["metrics"][number];

function withMetrics(metrics: Metric[]): Evaluation {
  return { ...reportingStoryEvaluation, metrics };
}

const won: Metric = {
  id: "bookings_won",
  version: "2",
  unit: "EUR",
  value: 42_000_000,
  target: 80_000_000,
  target_actual: 42_000_000,
  coverage: { status: "ok", withheld: false },
  evidence: { metric: "bookings_won", context_id: "interval" },
};

it("writes the won, forecast and open sentences from the figures it was given", () => {
  const sentences = writtenSummary(
    withMetrics([
      won,
      {
        id: "forecast_landing",
        version: "1",
        unit: "EUR",
        value: 76_000_000,
        coverage: { status: "ok", withheld: false },
        evidence: { metric: "forecast_landing", context_id: "state" },
      },
      {
        id: "open_pipeline",
        version: "1",
        unit: "EUR",
        value: 120_000_000,
        coverage: { status: "ok", withheld: false, eligible_count: 34 },
        evidence: { metric: "open_pipeline", context_id: "state" },
      },
    ]),
    english,
  );
  expect(sentences).toHaveLength(3);
  expect(sentences[0]).toMatch(/^You won .+ in .+, 53% of the .+ target\.$/);
  expect(sentences[1]).toMatch(
    /^The deal forecast expects this fiscal quarter to end at .+\.$/,
  );
  expect(sentences[2]).toMatch(/ is open across 34 deals\.$/);
});

it("leaves out a figure that was not measured rather than guessing it", () => {
  const sentences = writtenSummary(
    withMetrics([
      { ...won, coverage: { status: "failed", withheld: false } },
      {
        id: "open_pipeline",
        version: "1",
        unit: "EUR",
        value: 120_000_000,
        coverage: { status: "ok", withheld: false },
        evidence: { metric: "open_pipeline", context_id: "state" },
      },
    ]),
    english,
  );
  expect(sentences).toEqual([]);
});

it("says no sales were won when the period has none", () => {
  const sentences = writtenSummary(
    withMetrics([
      { ...won, value: 0, coverage: { status: "no_data", withheld: false } },
    ]),
    english,
  );
  expect(sentences).toHaveLength(1);
  expect(sentences[0]).toMatch(/^No sales were won in .+\.$/);
});
