/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { GrantSpec } from "../app/mefixture";
import { feature, status } from "./ai-admin.testkit";
import { AiRoutingCard } from "./ai-routing";
import { backendFor, jsonResponse, render } from "./ai-routing.testkit";
import { AiTasksCard } from "./ai-tasks";

// The AI tasks card and the per-tier facts the Model tiers rows join from the
// same status and health reads.

const EVERYTHING: GrantSpec = {
  ai_routing: ["read", "update"],
  ai_budget: ["read"],
  ai_diagnostics: ["read"],
};

const HEALTH = {
  window_hours: 1,
  rungs: [
    {
      tier: "cheap_cloud",
      healthy: true,
      calls: 12,
      failures: 1,
      median_latency_ms: 840,
    },
  ],
};

// The routing card's own stub, widened by the two reads only a diagnostics
// seat makes.
function withDiagnostics(allow: GrantSpec) {
  const backend = backendFor(allow);
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = input instanceof Request ? input.url : String(input);
      if (url.includes("/ai/status")) return jsonResponse(status);
      if (url.includes("/ai/health")) return jsonResponse(HEALTH);
      return backend.fetchMock(input, init);
    },
  );
  return fetchMock;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("AiTasksCard", () => {
  it("leads each task with the model it runs on now", async () => {
    vi.stubGlobal("fetch", withDiagnostics(EVERYTHING));
    render(<AiTasksCard />);

    expect(await screen.findByText(feature.display_name)).toBeTruthy();
    expect(screen.getByText("gemini · example-model")).toBeTruthy();
    // The hedge that said this was only the policy's pick is gone: the row is
    // the resolved chain.
    expect(screen.queryByText(/does not show provider status/i)).toBeNull();
  });

  // `/ai/status` answers an empty task list to a seat without routing read,
  // and nothing to one without diagnostics or budget read. Each gets the
  // withheld panel rather than a table that reads as "no tasks".
  it.each([
    { ai_budget: ["read"], ai_diagnostics: ["read"] } satisfies GrantSpec,
    { ai_routing: ["read"], ai_budget: ["read"] } satisfies GrantSpec,
    { ai_routing: ["read"], ai_diagnostics: ["read"] } satisfies GrantSpec,
  ])("withholds the section from a partial grant: %j", async (allow) => {
    vi.stubGlobal("fetch", withDiagnostics(allow));
    render(<AiTasksCard />);

    expect(
      await screen.findByText(/only a user with both ai diagnostics read/i),
    ).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
  });
});

describe("a Model tiers row's facts", () => {
  it("says how many tasks lead with a tier and whether it answered", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", withDiagnostics(EVERYTHING));
    render(<AiRoutingCard />);

    const cheap = await screen.findByTestId("ai-routing-tier-cheap_cloud");
    expect(await within(cheap).findByText("1 task")).toBeTruthy();
    // The dot names the state and opens the numbers behind it.
    await user.click(
      await within(cheap).findByRole("button", { name: "Responding" }),
    );
    expect(await screen.findByText("12 calls, 1 failed")).toBeTruthy();
    expect(screen.getByText("Median 840 ms")).toBeTruthy();
    await user.keyboard("{Escape}");

    // A tier nothing called in the window says so rather than reading healthy,
    // and a tier no task leads with claims no count.
    const premium = screen.getByTestId("ai-routing-tier-premium");
    expect(within(premium).queryByText(/task/)).toBeNull();
    expect(
      within(premium).getByRole("button", {
        name: /no model calls in the last 1h/i,
      }),
    ).toBeTruthy();
  });

  // Without the diagnostics grant neither read is made, and the rows claim
  // nothing: an absent count is not "unused".
  it("claims nothing a reader's grants do not cover", async () => {
    vi.stubGlobal(
      "fetch",
      withDiagnostics({ ai_routing: ["read", "update"], ai_budget: ["read"] }),
    );
    render(<AiRoutingCard />);

    const cheap = await screen.findByTestId("ai-routing-tier-cheap_cloud");
    expect(within(cheap).queryByText(/task/)).toBeNull();
    expect(within(cheap).queryByText("Responding")).toBeNull();
  });
});
