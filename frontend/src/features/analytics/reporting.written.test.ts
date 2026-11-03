// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { translate, translatePlural, type Words } from "../../i18n";
import { reportingStoryEvaluation } from "../../testing/reporting.fixtures";
import { writtenSummary } from "./reporting.written";

const english: Words = {
  t: (key, params) => translate("en", key, params),
  plural: (base, count, params) => translatePlural("en", base, count, params),
  locale: "en",
};

type Evaluation = typeof reportingStoryEvaluation;
type Metric = Evaluation["metrics"][number];

const context: Evaluation["context"] = {
  ...reportingStoryEvaluation.context,
  interval: {
    start_at: "2026-10-12T00:00:00Z",
    end_at: "2026-10-19T00:00:00Z",
  },
  target_interval: {
    start_at: "2026-10-01T00:00:00Z",
    end_at: "2026-11-01T00:00:00Z",
  },
  state_at: "2026-09-30T10:00:00Z",
};

function withMetrics(metrics: Metric[]): Evaluation {
  return { ...reportingStoryEvaluation, context, metrics };
}

// A week's sales against a month's target: the two actuals differ.
const won: Metric = {
  id: "bookings_won",
  version: "2",
  unit: "EUR",
  value: 30_000,
  target: 100_000,
  target_actual: 50_000,
  coverage: { status: "ok", withheld: false },
  evidence: { metric: "bookings_won", context_id: "interval" },
};

const open: Metric = {
  id: "open_pipeline",
  version: "1",
  unit: "EUR",
  value: 120_000_000,
  coverage: { status: "ok", withheld: false, eligible_count: 34 },
  evidence: { metric: "open_pipeline", context_id: "state" },
};

it("states the period's sales and the target's own actual separately", () => {
  const [sales, target] = writtenSummary(withMetrics([won]), english);
  expect(sales).toMatch(/^Won in .+: €300\.$/);
  expect(target).toMatch(/^Target for .+: €1,000, 50% reached with €500\.$/);
});

it("dates the forecast and the open pipeline to the day they were read", () => {
  const sentences = writtenSummary(
    withMetrics([
      {
        id: "forecast_landing",
        version: "1",
        unit: "EUR",
        value: 76_000_000,
        coverage: { status: "ok", withheld: false },
        evidence: { metric: "forecast_landing", context_id: "state" },
      },
      open,
    ]),
    english,
  );
  expect(sentences).toEqual([
    expect.stringMatching(
      /^Forecast for the end of the fiscal quarter, as of 30 Sept? 2026: .+\.$/,
    ),
    expect.stringMatching(/^Open on 30 Sept? 2026: .+ in 34 deals\.$/),
  ]);
});

it("leaves out a figure that was not measured or that is partly hidden", () => {
  const sentences = writtenSummary(
    withMetrics([
      { ...won, coverage: { status: "failed", withheld: false } },
      { ...open, coverage: { ...open.coverage, withheld: true } },
    ]),
    english,
  );
  expect(sentences).toEqual([]);
});

it("does not say nothing was won when the wins are hidden", () => {
  const sentences = writtenSummary(
    withMetrics([
      { ...won, value: 0, coverage: { status: "no_data", withheld: true } },
    ]),
    english,
  );
  expect(sentences).toEqual([]);
});

it("says no sales were won when the period has none", () => {
  const sentences = writtenSummary(
    withMetrics([
      {
        ...won,
        value: 0,
        target: undefined,
        coverage: { status: "no_data", withheld: false },
      },
    ]),
    english,
  );
  expect(sentences).toEqual([expect.stringMatching(/^No sales won in .+\.$/)]);
});

it("leaves the empty scope to the notice instead of saying no sales were won", () => {
  const evaluation = withMetrics([
    {
      ...won,
      value: 0,
      target: undefined,
      coverage: { status: "no_data", withheld: false },
    },
  ]);
  const emptyScope = {
    ...evaluation,
    context: { ...evaluation.context, scope_holds_deals: false },
  };
  expect(writtenSummary(emptyScope, english)).toEqual([]);
  const scopeWithDeals = {
    ...evaluation,
    context: { ...evaluation.context, scope_holds_deals: true },
  };
  expect(writtenSummary(scopeWithDeals, english)).toEqual([
    expect.stringMatching(/^No sales won in .+\.$/),
  ]);
});
