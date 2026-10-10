// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { armHoverIntent } from "../design-system/hoverintent-testing";
import { LocaleProvider } from "../i18n";
import { feature } from "./ai-admin.testkit";
import { AiFeatureTable } from "./ai-feature-table";
import { taskDot } from "./ai-task-details";
import { openTaskDetails } from "./ai-task-details-testing";

type Feature = components["schemas"]["AiFeatureRoute"];
type Providers = components["schemas"]["AiProviderHealth"];

afterEach(cleanup);

const lead = feature.effective_candidates[0];
// A two-rung ladder whose rungs sit on two providers, so one can be blocked
// while the other still answers.
const ladder: Feature = {
  ...feature,
  execution_mode: "background",
  effective_candidates: [
    lead,
    { ...lead, tier: "premium", provider: "anthropic", model: "fallback" },
  ],
};
const blocked = (...names: string[]): Providers => ({
  providers: names.map((provider) => ({
    provider,
    health: "out_of_credit",
    since: "2026-10-06T08:00:00Z",
    retry_after: "2026-10-06T08:15:00Z",
  })),
});
const answering = { window_hours: 1, rungs: [] };

describe("taskDot", () => {
  it("is red only once every rung's provider is blocked", () => {
    expect(taskDot(ladder, answering, blocked("gemini"))).toBe("idle");
    expect(taskDot(ladder, answering, blocked("gemini", "anthropic"))).toBe(
      "bad",
    );
  });

  it("does not count a degraded provider as blocked", () => {
    const degraded: Providers = {
      providers: [
        {
          provider: "gemini",
          health: "degraded",
          since: "2026-10-06T08:00:00Z",
        },
      ],
    };
    expect(taskDot(feature, undefined, degraded)).toBe("idle");
  });

  it("reads the lane a task starts on", () => {
    const rungs = (healthy: boolean) => ({
      window_hours: 1,
      rungs: [
        {
          tier: "cheap_cloud",
          healthy,
          calls: 3,
          failures: 0,
          median_latency_ms: 800,
        },
      ],
    });
    expect(taskDot(feature, rungs(true), undefined)).toBe("ok");
    expect(taskDot(feature, rungs(false), undefined)).toBe("bad");
  });

  it("reads the ladder's lane once the decision model's provider is blocked", () => {
    const deciding: Feature = {
      ...feature,
      decision_first: true,
      decision_candidate: { ...lead, tier: "decide", provider: "jev" },
    };
    const lanes = {
      window_hours: 1,
      rungs: [
        {
          tier: "decide",
          healthy: false,
          calls: 3,
          failures: 3,
          median_latency_ms: 0,
        },
        {
          tier: "cheap_cloud",
          healthy: true,
          calls: 3,
          failures: 0,
          median_latency_ms: 800,
        },
      ],
    };
    expect(taskDot(deciding, lanes, undefined)).toBe("bad");
    expect(taskDot(deciding, lanes, blocked("jev"))).toBe("ok");
  });

  it("is red for a task waiting on the allowance or bound to nothing", () => {
    expect(
      taskDot({ ...feature, impact: "budget_blocked" }, undefined, undefined),
    ).toBe("bad");
    expect(
      taskDot({ ...feature, impact: "unconfigured" }, undefined, undefined),
    ).toBe("bad");
  });
});

describe("a task's details", () => {
  const show = (row: Feature, providers?: Providers) =>
    render(
      <LocaleProvider initial="en">
        <AiFeatureTable rows={[row]} providers={providers} onEdit={() => {}} />
      </LocaleProvider>,
    );

  it("says a background task waits while every model it can use is blocked", async () => {
    const user = userEvent.setup({ delay: null });
    show(ladder, blocked("gemini", "anthropic"));
    const details = await openTaskDetails(user, ladder.display_name);
    expect(details).toHaveTextContent("Waiting now");
    expect(details).toHaveTextContent("Out of credit");
  });

  it("says a task that answers from its own facts still answers while every model is blocked", async () => {
    const user = userEvent.setup({ delay: null });
    const row = {
      ...ladder,
      execution_mode: "interactive",
      degrades_on_outage: true,
    };
    show(row, blocked("gemini", "anthropic"));
    const details = await openTaskDetails(user, row.display_name);
    expect(details).toHaveTextContent("Answering from its own facts now");
    expect(details).not.toHaveTextContent("Failing now");
    expect(details).not.toHaveTextContent("fails at once");
  });

  it("says an interactive task fails while every model it can use is blocked", async () => {
    const user = userEvent.setup({ delay: null });
    const row = { ...ladder, execution_mode: "interactive" };
    show(row, blocked("gemini", "anthropic"));
    expect(await openTaskDetails(user, row.display_name)).toHaveTextContent(
      "Failing now",
    );
  });

  it("says a blocked rung is skipped while another still answers", async () => {
    const user = userEvent.setup({ delay: null });
    show(ladder, blocked("gemini"));
    const details = await openTaskDetails(user, ladder.display_name);
    expect(details).toHaveTextContent("A blocked provider is skipped");
    expect(details).not.toHaveTextContent("Waiting now");
  });

  it("says a blocked decision model is skipped", async () => {
    const user = userEvent.setup({ delay: null });
    const deciding: Feature = {
      ...ladder,
      decision_first: true,
      decision_candidate: { ...lead, tier: "decide", provider: "jev" },
    };
    show(deciding, blocked("jev"));
    expect(
      await openTaskDetails(user, deciding.display_name),
    ).toHaveTextContent("A blocked provider is skipped");
  });

  it("names the decision model's provider rather than its adapter key", async () => {
    const user = userEvent.setup({ delay: null });
    const deciding: Feature = {
      ...ladder,
      decision_first: true,
      decision_candidate: {
        ...lead,
        tier: "decide",
        provider: "jev_compatible",
      },
    };
    show(deciding);
    const details = await openTaskDetails(user, deciding.display_name);
    expect(details).toHaveTextContent("Jev-compatible");
    expect(details).not.toHaveTextContent("jev_compatible");
  });

  it("says search indexing is refused rather than waiting", async () => {
    const user = userEvent.setup({ delay: null });
    const embed = { ...feature, execution_mode: "embedding" };
    show(embed, blocked("gemini"));
    const details = await openTaskDetails(user, embed.display_name);
    expect(details).toHaveTextContent("Refused now");
    expect(details).not.toHaveTextContent("Waiting now");
  });

  it("says what the task would do, with nothing blocked", async () => {
    const user = userEvent.setup({ delay: null });
    show(ladder);
    const details = await openTaskDetails(user, ladder.display_name);
    expect(details).toHaveTextContent(
      "tries again at the provider’s next check",
    );
    expect(details).not.toHaveTextContent("A blocked provider is skipped");
  });
});

it("keeps a greyed Edit on a row with nothing to tune, saying where its model is set", async () => {
  const user = userEvent.setup({ delay: null });
  armHoverIntent();
  const embed = {
    ...feature,
    display_name: "Search and retrieval",
    defaults: undefined,
  };
  render(
    <LocaleProvider initial="en">
      <AiFeatureTable rows={[embed]} onEdit={() => {}} />
    </LocaleProvider>,
  );
  const edit = screen.getByRole("button", {
    name: "Edit Search and retrieval",
  });
  expect(edit).toBeDisabled();
  expect(edit).toHaveAccessibleDescription(
    /on the Embedding model row under Model tiers/,
  );
  await user.hover(edit.parentElement ?? edit);
  expect(await screen.findByRole("tooltip")).toHaveTextContent(
    "Search and retrieval has no thinking level or timeouts to set.",
  );
});
