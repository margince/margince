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
// seat makes. `health` is what `/ai/health` answers; `null` never answers, the
// wait a reader sits through before the first health read lands.
function withDiagnostics(allow: GrantSpec, health: unknown = HEALTH) {
  const backend = backendFor(allow);
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = input instanceof Request ? input.url : String(input);
      if (url.includes("/ai/status")) return jsonResponse(status);
      if (url.includes("/ai/health")) {
        return health === null
          ? new Promise<Response>(() => {})
          : jsonResponse(health);
      }
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
    expect(screen.getByText("example-model")).toBeTruthy();
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
    // Named after its lane, so seven dots are seven controls to a reader.
    await user.click(
      await within(cheap).findByRole("button", {
        name: "cheap_cloud: Responding",
      }),
    );
    expect(await screen.findByText("12 calls, 1 failed")).toBeTruthy();
    expect(screen.getByText("Median 840 ms")).toBeTruthy();
    await user.keyboard("{Escape}");

    // A tier nothing called in the window says so rather than reading healthy,
    // and a tier no task leads with says so too, so every row has the same shape.
    const premium = screen.getByTestId("ai-routing-tier-premium");
    expect(within(premium).getByText("0 tasks")).toBeTruthy();
    expect(
      within(premium).getByRole("button", {
        name: /^premium: no model calls in the last 1h/i,
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
    expect(within(cheap).queryByText(/Responding/)).toBeNull();
  });
});

// A reader holding the diagnostics read and not the routing one gets the
// health rows alone: each rung as a lane, unbound, named as the lane the
// document would call it.
describe("the Model tiers card for a diagnostics-only reader", () => {
  const DIAGNOSTICS: GrantSpec = { ai_diagnostics: ["read"] };

  it("lists every rung that took calls, unbound, under its lane's name", async () => {
    vi.stubGlobal(
      "fetch",
      withDiagnostics(DIAGNOSTICS, {
        window_hours: 24,
        rungs: [
          ...HEALTH.rungs,
          {
            tier: "embed",
            healthy: false,
            calls: 3,
            failures: 3,
            median_latency_ms: 0,
            last_sentinel: "provider_quota",
          },
        ],
      }),
    );
    render(<AiRoutingCard />);

    const cheap = await screen.findByTestId("ai-routing-tier-cheap_cloud");
    expect(within(cheap).getByText("Not bound")).toBeTruthy();
    expect(
      within(cheap).getByRole("button", { name: "cheap_cloud: Responding" }),
    ).toBeTruthy();
    const embed = screen.getByTestId("ai-routing-tier-embeddings");
    expect(within(embed).getByText("embeddings")).toBeTruthy();
    expect(within(embed).getByText(/provider_quota/)).toBeTruthy();
    expect(screen.queryByText("embed")).toBeNull();
    expect(screen.queryByRole("button", { name: /^edit$/i })).toBeNull();
  });

  it("says no lane took a call rather than drawing an empty list", async () => {
    vi.stubGlobal(
      "fetch",
      withDiagnostics(DIAGNOSTICS, { window_hours: 24, rungs: [] }),
    );
    render(<AiRoutingCard />);

    expect(
      await screen.findByText("No model calls in the last 24h."),
    ).toBeTruthy();
    expect(screen.queryByTestId(/^ai-routing-tier-/)).toBeNull();
  });

  it("waits on the health read rather than showing nothing", async () => {
    vi.stubGlobal("fetch", withDiagnostics(DIAGNOSTICS, null));
    render(<AiRoutingCard />);

    expect(await screen.findByRole("status")).toHaveAttribute(
      "aria-busy",
      "true",
    );
    expect(screen.queryByText(/No model calls/)).toBeNull();
  });
});
