// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { jsonResponse, render } from "./ai-routing.testkit";
import { TaskOutcome } from "./ai-task-outcome";

type Step = components["schemas"]["AiFlowStep"];

const step = (extra: Partial<Step> = {}): Step => ({
  decision: false,
  tier: "cheap_cloud",
  provider: "openrouter",
  model: "bound-model",
  attempts: 10,
  answered: 10,
  p50_ms: 800,
  gave_up: {},
  switched: 0,
  ...extra,
});

const flowOf = (steps: Step[]): components["schemas"]["AiTaskFlow"] => ({
  task: "summarize",
  window: "7d",
  total: 10,
  unanswered: 0,
  steps,
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const ROUTING_READER: GrantSpec = {
  ai_diagnostics: ["read"],
  ai_routing: ["read"],
};

function signedIn(allow: GrantSpec = ROUTING_READER) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse(meFixture({ allow }))),
  );
}

const SWITCHED_NOT_CERTIFIED = step({
  switched: 4,
  switched_to: "other-model",
  switched_to_certification: { verdict: "not_supported", current: true },
});

// A step a provider answered with another model says what the records hold for
// that model, as the task's bound models do.
describe("TaskOutcome", () => {
  it("says a switched model is not certified when its record says so", async () => {
    signedIn();
    render(<TaskOutcome flow={flowOf([SWITCHED_NOT_CERTIFIED])} />);
    expect(
      await screen.findByText(
        "other-model is not certified for this task. It is still used, so check what it produces.",
      ),
    ).toBeInTheDocument();
  });

  // The server sends no grade to this seat, so its absence says nothing.
  it("says nothing of certification to a seat that may not read routing", async () => {
    signedIn({ ai_diagnostics: ["read"] });
    render(<TaskOutcome flow={flowOf([SWITCHED_NOT_CERTIFIED])} />);
    await screen.findByRole("status");
    expect(screen.queryByText(/for this task/)).not.toBeInTheDocument();
  });

  it("says a switched model no record measured has not been measured", async () => {
    signedIn();
    render(
      <TaskOutcome
        flow={flowOf([step({ switched: 4, switched_to: "other-model" })])}
      />,
    );
    expect(
      await screen.findByText(
        "other-model has not been measured for this task with these settings.",
      ),
    ).toBeInTheDocument();
  });

  it("grades nothing on a step that was not switched", () => {
    signedIn();
    render(<TaskOutcome flow={flowOf([step()])} />);
    expect(screen.queryByText(/for this task/)).not.toBeInTheDocument();
  });
});
