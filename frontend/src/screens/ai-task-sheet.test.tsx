// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { feature } from "./ai-admin.testkit";
import { jsonResponse, render } from "./ai-routing.testkit";
import { overrideOf, TaskSheet, timeoutOptions } from "./ai-task-sheet";

// A task's thinking level and timeouts: which selects a task gets, what a save
// sends, and what a colleague's newer save does to it.

type Feature = components["schemas"]["AiFeatureRoute"];

const DECIDING: Feature = {
  ...feature,
  task: "capture_confidentiality_verdict",
  display_name: "Thread confidentiality check",
  decides: true,
  decision_first: true,
};

const FLOW = {
  task: DECIDING.task,
  window: "7d",
  total: 10,
  unanswered: 1,
  steps: [
    {
      decision: true,
      tier: "decide",
      provider: "jev_compatible",
      model: "typesafe/jev-1.13",
      attempts: 10,
      answered: 8,
      p50_ms: 500,
      gave_up: { timeout: 2 },
    },
    {
      decision: false,
      tier: "local_small",
      provider: "openai_compatible",
      model: "openai/gpt-oss-120b",
      attempts: 2,
      answered: 1,
      p50_ms: 1100,
      gave_up: { provider_error: 1 },
    },
  ],
};

function server({
  overrides = {},
  conflict = false,
}: {
  overrides?: unknown;
  conflict?: boolean;
} = {}) {
  const puts: { body: unknown; ifMatch: string | null }[] = [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.url.endsWith("/v1/me"))
        return jsonResponse(
          meFixture({
            allow: { ai_routing: ["read", "update"], ai_diagnostics: ["read"] },
          }),
        );
      if (req.url.includes("/ai/task-overrides")) {
        if (req.method === "PUT") {
          puts.push({
            body: await req.json(),
            ifMatch: req.headers.get("If-Match"),
          });
          if (conflict)
            return jsonResponse(
              { title: "Conflict", status: 409, code: "version_skew" },
              409,
            );
        }
        const response = jsonResponse(overrides);
        response.headers.set("ETag", '"overrides-v1"');
        return response;
      }
      if (req.url.includes("/ai/call-stats/flow")) return jsonResponse(FLOW);
      if (req.url.includes("/ai/call-stats")) {
        return jsonResponse({
          window: "7d",
          group: "tier",
          rows: [
            {
              key: "decide",
              calls: 10,
              failed: 2,
              timeouts: 2,
              p50_ms: 500,
              p95_ms: 14000,
              tokens_in: 0,
              tokens_out: 0,
              cost_microusd: 0,
              unpriced: 0,
            },
          ],
        });
      }
      throw new Error(`unexpected request: ${req.method} ${req.url}`);
    },
  );
  vi.stubGlobal("fetch", fetchMock);
  return { puts };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("TaskSheet", () => {
  it("offers a deciding task three settings and marks each default", async () => {
    server();
    const user = userEvent.setup();
    render(
      <TaskSheet route={DECIDING} canManage canSeeCalls onClose={() => {}} />,
    );

    const sheet = await screen.findByRole("dialog");
    expect(
      within(sheet).getByRole("combobox", { name: /Thinking level/ }),
    ).toBeTruthy();
    const decision = within(sheet).getByRole("combobox", {
      name: /Decision model timeout/,
    });
    expect(decision).toHaveTextContent("15 s (default)");
    expect(
      within(sheet).getByRole("combobox", { name: /Model call timeout/ }),
    ).toHaveTextContent("300 s (default)");
    await user.click(decision);
    const options = screen.getAllByRole("option").map((o) => o.textContent);
    expect(options).toContain("60 s");
    expect(options).not.toContain("90 s");
  });

  it("offers any other task no decision timeout", async () => {
    server();
    render(
      <TaskSheet
        route={feature}
        canManage
        canSeeCalls={false}
        onClose={() => {}}
      />,
    );

    const sheet = await screen.findByRole("dialog");
    expect(
      within(sheet).queryByRole("combobox", { name: /Decision model timeout/ }),
    ).toBeNull();
    expect(
      within(sheet).getByRole("combobox", { name: /Model call timeout/ }),
    ).toBeTruthy();
  });

  it("saves only what departs from the defaults, under the ETag it read", async () => {
    const { puts } = server();
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(
      <TaskSheet
        route={DECIDING}
        canManage
        canSeeCalls={false}
        onClose={onClose}
      />,
    );

    const sheet = await screen.findByRole("dialog");
    const save = within(sheet).getByRole("button", { name: "Save settings" });
    expect(save).toBeDisabled();
    await user.click(
      within(sheet).getByRole("combobox", { name: /Decision model timeout/ }),
    );
    await user.click(screen.getByRole("option", { name: "30 s" }));
    await user.click(save);

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(puts).toEqual([
      {
        body: { [DECIDING.task]: { decision_timeout_ms: 30000 } },
        ifMatch: '"overrides-v1"',
      },
    ]);
  });

  it("resets a customised task to its defaults", async () => {
    server({
      overrides: {
        [DECIDING.task]: { thinking: "high", attempt_timeout_ms: 60000 },
      },
    });
    const user = userEvent.setup();
    render(
      <TaskSheet
        route={DECIDING}
        canManage
        canSeeCalls={false}
        onClose={() => {}}
      />,
    );

    const sheet = await screen.findByRole("dialog");
    await waitFor(() =>
      expect(
        within(sheet).getByRole("combobox", { name: /Model call timeout/ }),
      ).toHaveTextContent("60 s"),
    );
    await user.click(
      within(sheet).getByRole("button", { name: "Reset to defaults" }),
    );
    expect(
      within(sheet).getByRole("combobox", { name: /Model call timeout/ }),
    ).toHaveTextContent("300 s (default)");
    expect(within(sheet).getByText("Unsaved changes")).toBeTruthy();
  });

  it("says a colleague saved first rather than overwriting them", async () => {
    server({ conflict: true });
    const user = userEvent.setup();
    render(
      <TaskSheet
        route={feature}
        canManage
        canSeeCalls={false}
        onClose={() => {}}
      />,
    );

    const sheet = await screen.findByRole("dialog");
    await user.click(
      within(sheet).getByRole("combobox", { name: /Thinking level/ }),
    );
    await user.click(screen.getByRole("option", { name: "low" }));
    await user.click(
      within(sheet).getByRole("button", { name: "Save settings" }),
    );

    expect(
      await within(sheet).findByText(
        "Someone saved these settings while you were editing",
      ),
    ).toBeTruthy();
  });

  it("shows which step answered and how the slow calls sit against the timeout", async () => {
    server();
    render(
      <TaskSheet route={DECIDING} canManage canSeeCalls onClose={() => {}} />,
    );

    const sheet = await screen.findByRole("dialog");
    expect(
      await within(sheet).findByText(
        "90% of 10 calls got an answer. 1 did not.",
      ),
    ).toBeTruthy();
    expect(within(sheet).getByText("8 of 10 answered")).toBeTruthy();
    expect(within(sheet).getByText("2 timed out → passed on")).toBeTruthy();
    expect(within(sheet).getByText("1 failed → no answer")).toBeTruthy();
    expect(await within(sheet).findByText(/p95 is close to it/)).toBeTruthy();
  });

  it("disables every control for a reader who may look but not change", async () => {
    server();
    render(
      <TaskSheet
        route={DECIDING}
        canManage={false}
        canSeeCalls={false}
        onClose={() => {}}
      />,
    );

    const sheet = await screen.findByRole("dialog");
    expect(
      within(sheet).getByRole("combobox", { name: /Thinking level/ }),
    ).toBeDisabled();
    expect(
      within(sheet).getByRole("button", { name: "Save settings" }),
    ).toBeDisabled();
  });
});

describe("timeoutOptions and overrideOf", () => {
  it("keeps every option inside the bounds a save accepts", () => {
    expect(timeoutOptions([10, 300], 300)).toEqual([
      10, 15, 20, 30, 45, 60, 90, 120, 180, 240, 300,
    ]);
    expect(timeoutOptions([5, 60], 15)).toEqual([5, 10, 15, 20, 30, 45, 60]);
  });

  it("writes nothing for a draft at its defaults, and no decision timeout off a deciding task", () => {
    const defaults = { thinking: "" as const, decision: 15, attempt: 300 };
    expect(overrideOf(defaults, defaults, true)).toEqual({});
    expect(
      overrideOf(
        { thinking: "low", decision: 30, attempt: 60 },
        defaults,
        false,
      ),
    ).toEqual({ thinking: "low", attempt_timeout_ms: 60000 });
  });
});
