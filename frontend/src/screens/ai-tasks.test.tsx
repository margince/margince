/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { GrantSpec } from "../app/mefixture";
import { feature, status } from "./ai-admin.testkit";
import { AiRoutingCard } from "./ai-routing";
import { backendFor, jsonResponse, render } from "./ai-routing.testkit";
import { openTaskDetails } from "./ai-task-details-testing";
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
      if (url.includes("/ai/task-overrides")) return jsonResponse({});
      if (url.includes("/ai/call-stats/flow")) {
        return jsonResponse({
          task: feature.task,
          window: "7d",
          total: 0,
          unanswered: 0,
          steps: [],
        });
      }
      if (url.includes("/ai/call-stats"))
        return jsonResponse({ window: "7d", group: "task", rows: [] });
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
  it("names each task's tier and leaves the model to the tier", async () => {
    vi.stubGlobal("fetch", withDiagnostics(EVERYTHING));
    render(<AiTasksCard />);

    const row = await screen.findByTestId(`ai-task-row-${feature.task}`);
    expect(within(row).getByText("Everyday cloud")).toBeTruthy();
    expect(within(row).queryByText("example-model")).toBeNull();
    expect(screen.getByRole("columnheader", { name: "Tier" })).toBeVisible();
    expect(within(row).queryByText("Custom")).toBeNull();
  });

  it("opens a task's sheet from anywhere on its row, linking its calls", async () => {
    vi.stubGlobal("fetch", withDiagnostics(EVERYTHING));
    const user = userEvent.setup();
    render(<AiTasksCard />);

    const row = await screen.findByTestId(`ai-task-row-${feature.task}`);
    await user.click(within(row).getByText("Everyday cloud"));
    const sheet = await screen.findByRole("dialog");
    expect(
      within(sheet)
        .getByRole("link", { name: "View these calls →" })
        .getAttribute("href"),
    ).toBe(`#/settings/model-calls?task=${feature.task}`);
  });

  it("offers Open, not Edit, to a seat that may not change routing", async () => {
    vi.stubGlobal(
      "fetch",
      withDiagnostics({
        ai_routing: ["read"],
        ai_budget: ["read"],
        ai_diagnostics: ["read"],
      }),
    );
    render(<AiTasksCard />);

    const row = await screen.findByTestId(`ai-task-row-${feature.task}`);
    expect(
      within(row).getByRole("button", { name: `Open ${feature.display_name}` }),
    ).toBeTruthy();
  });

  it("says there is nothing yet, inside the panel, rather than a bare header row", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = input instanceof Request ? input.url : String(input);
        if (url.includes("/ai/status")) {
          return jsonResponse({ ...status, features: [] });
        }
        return withDiagnostics(EVERYTHING)(input, init);
      }),
    );
    render(<AiTasksCard />);

    const empty = await screen.findByText("Nothing here yet.");
    expect(empty.closest(".panel-body")).not.toBeNull();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("keeps a failed read inside the panel's padding", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = input instanceof Request ? input.url : String(input);
        if (url.includes("/ai/status")) {
          return jsonResponse({ status: 500, detail: "boom" }, 500);
        }
        return withDiagnostics(EVERYTHING)(input, init);
      }),
    );
    render(<AiTasksCard />);

    expect(
      (await screen.findByRole("alert")).closest(".panel-body"),
    ).not.toBeNull();
  });

  it("refuses the embeddings row's verb with where its settings live", async () => {
    const embeddings = {
      ...feature,
      task: "embeddings",
      display_name: "Search and retrieval",
      leading_tier: "embeddings",
      defaults: undefined,
    };
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = input instanceof Request ? input.url : String(input);
        if (url.includes("/ai/status")) {
          return jsonResponse({ ...status, features: [embeddings, feature] });
        }
        return withDiagnostics(EVERYTHING)(input, init);
      }),
    );
    const user = userEvent.setup();
    render(<AiTasksCard />);

    const rows = await screen.findAllByTestId(/^ai-task-row-/);
    expect(rows.map((row) => row.dataset.testid)).toEqual([
      `ai-task-row-${feature.task}`,
      "ai-task-row-embeddings",
    ]);
    const verb = within(rows[1]).getByRole("button", {
      name: "Edit Search and retrieval",
    });
    expect(verb).toHaveAccessibleDescription(/embedding model row/i);
    expect(rows[1]).not.toHaveClass("rowlink");
    expect(rows[0]).toHaveClass("rowlink");
    await user.click(within(rows[1]).getByText("Embedding model"));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("marks a task an admin customised", async () => {
    vi.stubGlobal("fetch", withDiagnostics(EVERYTHING));
    const custom = {
      ...status,
      features: [{ ...feature, overrides: { thinking: "high" as const } }],
    };
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = input instanceof Request ? input.url : String(input);
        if (url.includes("/ai/status")) return jsonResponse(custom);
        return withDiagnostics(EVERYTHING)(input, init);
      }),
    );
    const user = userEvent.setup({ delay: null });
    render(<AiTasksCard />);

    await screen.findByText(feature.display_name);
    expect(await openTaskDetails(user, feature.display_name)).toHaveTextContent(
      "Custom",
    );
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
        name: "Everyday cloud: Responding",
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
        name: /^Premium: no model calls in the last 1h/i,
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
      within(cheap).getByRole("button", { name: "Everyday cloud: Responding" }),
    ).toBeTruthy();
    const embed = screen.getByTestId("ai-routing-tier-embeddings");
    expect(within(embed).getByText("Embedding model")).toBeTruthy();
    expect(within(embed).getByText("embeddings")).toBeTruthy();
    expect(
      within(embed).getByText("3 calls, 3 failed · out of quota"),
    ).toBeTruthy();
    expect(screen.queryByText("embed")).toBeNull();
    expect(screen.queryByRole("button", { name: /^edit\b/i })).toBeNull();
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

  it("keeps a failed health read inside the panel's padding", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = input instanceof Request ? input.url : String(input);
        if (url.includes("/ai/health")) {
          return jsonResponse({ status: 500, detail: "boom" }, 500);
        }
        return withDiagnostics(DIAGNOSTICS)(input, init);
      }),
    );
    render(<AiRoutingCard />);

    expect(
      (await screen.findByRole("alert")).closest(".panel-body"),
    ).not.toBeNull();
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
