// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { LocaleProvider } from "../../i18n";
import { reportingStoryEvaluation } from "../../testing/reporting.fixtures";
import { QuarterTrajectory } from "./analytics.overview.trajectory";
import type { ReportingEvaluation } from "./reporting.model";

afterEach(cleanup);

const september = {
  start_at: "2026-08-31T22:00:00Z",
  end_at: "2026-09-30T22:00:00Z",
};

// A running period is read up to now; a closed one up to its own end.
function readAt(evaluatedAt: string): ReportingEvaluation {
  return {
    ...reportingStoryEvaluation,
    context: {
      ...reportingStoryEvaluation.context,
      interval: {
        ...september,
        end_at: evaluatedAt < september.end_at ? evaluatedAt : september.end_at,
      },
      horizon: september,
      evaluated_at: evaluatedAt,
    },
  };
}

function show(evaluation: ReportingEvaluation) {
  render(
    <LocaleProvider initial="en">
      <QuarterTrajectory
        evaluation={evaluation}
        charts={evaluation}
        onEvidence={() => {}}
      />
    </LocaleProvider>,
  );
}

it("names a closed period's last day, not the instant after it ends", async () => {
  show(readAt("2026-10-05T08:00:00Z"));
  expect(await screen.findByText(/^Through /)).toHaveProperty(
    "textContent",
    "Through 30 Sept",
  );
});

it("names today while the period runs", async () => {
  show(readAt("2026-09-22T12:00:00Z"));
  expect(await screen.findByText(/^Today · /)).toHaveProperty(
    "textContent",
    "Today · 22 Sept",
  );
});
