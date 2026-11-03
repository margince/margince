// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { translate, translatePlural, type Words } from "../../i18n";
import { reportingStoryEvaluation } from "../../testing/reporting.fixtures";
import {
  type ChecksReading,
  prospectingSummary,
  salesReviewSummary,
  writtenSummary,
} from "./reporting.written";

const english: Words = {
  t: (key, params) => translate("en", key, params),
  plural: (base, count, params) => translatePlural("en", base, count, params),
  locale: "en",
};

// The sentences alone, for the tests that read the words.
const salesText = (
  evaluation: Evaluation,
  words: Words,
  checks?: ChecksReading,
) => salesReviewSummary(evaluation, words, checks).map(({ text }) => text);

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

const landing = (comparison: Metric["target_comparison"]): Metric => ({
  id: "forecast_landing",
  version: "1",
  unit: "EUR",
  value: 76_000_000,
  coverage: { status: "ok", withheld: false },
  evidence: { metric: "forecast_landing", context_id: "state" },
  target_comparison: comparison,
});

const shortOfTarget: NonNullable<Metric["target_comparison"]> = {
  status: "compared",
  target_metric: "bookings_won",
  target: 80_000_000,
  landing: 76_000_000,
  won: 42_000_000,
  gap: 4_000_000, // ds:ignore the target comparison money gap, not a CSS gap
  remaining: 38_000_000,
  open_pipeline: 136_000_000,
  coverage: 3.58,
};

it("states the sales review's position from the server's comparison", () => {
  const sentences = salesText(
    withMetrics([landing(shortOfTarget), open]),
    english,
  );
  expect(sentences).toEqual([
    expect.stringMatching(
      /^Expected finish for .+: €760k, €40k short of the €800k target\.$/,
    ),
    "Open deals expected to close this quarter cover 3.6× of the €380k still to win.",
  ]);
});

it("says above, on and reached in the target's own words", () => {
  const ahead = salesText(
    withMetrics([landing({ ...shortOfTarget, gap: -2_000_000 })]),
    english,
  );
  expect(ahead[0]).toMatch(/€20k above the €800k target\.$/);
  const reached = salesText(
    withMetrics([
      landing({
        status: "target_reached",
        target_metric: "bookings_won",
        target: 80_000_000,
      }),
    ]),
    english,
  );
  expect(reached[0]).toMatch(
    /^Sales won in .+ have reached the €800k target\.$/,
  );
});

it("says there is no gap without a target, and concludes nothing from a failed figure", () => {
  expect(
    salesText(
      withMetrics([
        landing({ status: "no_target", target_metric: "bookings_won" }),
      ]),
      english,
    ),
  ).toEqual([
    expect.stringMatching(
      /^No sales target is set for .+, so there is no gap to show\.$/,
    ),
  ]);
  const failed = salesText(
    withMetrics([
      { ...won, value: null, coverage: { status: "failed", withheld: false } },
      {
        ...landing(shortOfTarget),
        value: null,
        coverage: { status: "failed", withheld: false },
      },
    ]),
    english,
  );
  expect(failed).toEqual([
    "Some figures could not be read just now, so this summary leaves them out.",
  ]);
});

it("gives no on-target verdict while a figure or the checks failed", () => {
  const ahead = landing({ ...shortOfTarget, gap: -2_000_000 });
  const failedWon = {
    ...won,
    value: null,
    coverage: { status: "failed" as const, withheld: false },
  };
  const withFailedFigure = salesText(withMetrics([failedWon, ahead]), english);
  expect(withFailedFigure.join(" ")).not.toMatch(/above the/);
  const withFailedChecks = salesText(withMetrics([ahead]), english, {
    state: "failed",
  });
  expect(withFailedChecks.join(" ")).not.toMatch(/above the/);
  expect(withFailedChecks).toContain(
    "The open checks could not be read, so nothing is said here about what needs attention.",
  );
  // Short of target is no healthy verdict, so a failed input leaves it standing.
  expect(
    salesText(withMetrics([failedWon, landing(shortOfTarget)]), english)[1],
  ).toMatch(/€40k short of the €800k target\.$/);
});

it("links each sentence to what it rests on, the checks to the action list", () => {
  const sentences = salesReviewSummary(
    withMetrics([won, landing(shortOfTarget), open]),
    english,
    { state: "ready", findings: 24, deals: 7 },
  );
  expect(sentences.map(({ link }) => link?.kind)).toEqual([
    "forecast",
    "evidence",
    "evidence",
    "interventions",
  ]);
  expect(sentences[1]?.link).toEqual({
    kind: "evidence",
    reference: won.evidence,
  });
  expect(sentences.at(-1)?.text).toBe(
    "24 open checks on 7 deals need an answer.",
  );
});

const count = (
  id: Metric["id"],
  value: number | null,
  status: Metric["coverage"]["status"] = "ok",
): Metric => ({
  id,
  version: "1",
  unit: "count",
  value,
  coverage: { status, withheld: false },
  evidence: { metric: id, context_id: "interval" },
});

it("states the prospecting outcome of the workflow in use, and a zero as a zero", () => {
  const evaluation = withMetrics([
    count("accepted_opportunities", 0, "no_data"),
    count("leads_promoted", 3),
    count("meetings_held", 1),
    count("leads_awaiting_response", 2),
  ]);
  expect(prospectingSummary(evaluation, "handoff", 2, english)).toEqual([
    expect.stringMatching(/^No handoffs accepted in .+\.$/),
    expect.stringMatching(/^1 meeting held in .+\.$/),
    "2 leads are still waiting for a first reply.",
    "2 handoffs are waiting for a decision from sales.",
  ]);
  expect(
    prospectingSummary(evaluation, "qualification", undefined, english)[0],
  ).toMatch(/^3 leads qualified in .+\.$/);
});

it("leaves an unread prospecting figure out and says it failed", () => {
  const evaluation = withMetrics([
    count("accepted_opportunities", null, "failed"),
    count("meetings_held", 4),
  ]);
  expect(prospectingSummary(evaluation, "handoff", undefined, english)).toEqual(
    [
      "Some figures could not be read just now, so this summary leaves them out.",
      expect.stringMatching(/^4 meetings held in .+\.$/),
    ],
  );
});
