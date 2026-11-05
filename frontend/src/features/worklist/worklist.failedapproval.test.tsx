// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { WorklistScreen } from "./worklist";

// A failed approval's way off the list: acknowledge posts the failure
// acknowledgement, and retry approves the row once more where the server
// offers it.

type Worklist = components["schemas"]["Worklist"];
type WorklistItem = components["schemas"]["WorklistItem"];

const failedTitle = "this was approved, but the work it released did not run";
const failedAt = "2026-08-31T08:12:03.123456Z";

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}

function failedRow(actions: WorklistItem["actions"]): WorklistItem {
  return {
    occurred_precision: "instant",
    id: "a1",
    source: "failed_approval",
    category: "tasks",
    level: 4,
    consequence: "task_slips",
    title: failedTitle,
    occurred_at: failedAt,
    because: [],
    actions,
  };
}

function day(queue: WorklistItem[]): Worklist {
  return {
    as_of: "2026-08-31T09:00:00Z",
    scope: "mine",
    scope_options: ["mine"],
    queue,
    summary: {
      urgent: 0,
      due: queue.length,
      lower_priority: 0,
      total: queue.length,
    },
    sources_unavailable: [],
    readings: {
      changed_since_brief: 0,
      revenue_at_risk_minor: null,
      buyer_replies: 0,
      prospecting: 0,
      review: 0,
      more_available: false,
    },
    reach: [],
    counts: [],
  };
}

// The server takes the row off the list once the named endpoint answers 2xx.
// It records every POST, so a test can say which endpoint was reached.
function stubServer(
  actions: WorklistItem["actions"],
  answers: Record<string, number>,
  held?: Promise<void>,
) {
  const posted: string[] = [];
  const bodies: unknown[] = [];
  let settled = false;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const target =
        input instanceof Request ? input : new Request(input, init);
      const path = new URL(target.url).pathname;
      if (target.method === "POST") {
        posted.push(path);
        const text = await target.text();
        bodies.push(text ? JSON.parse(text) : undefined);
        await held;
        const status = answers[path] ?? 404;
        if (status < 300) {
          settled = true;
          return new Response(status === 204 ? null : "{}", { status });
        }
        return new Response(JSON.stringify({ title: "refused" }), {
          status,
          headers: { "content-type": "application/json" },
        });
      }
      if (target.url.includes("/worklist")) {
        return jsonResponse(settled ? day([]) : day([failedRow(actions)]));
      }
      return jsonResponse({ data: [] });
    }),
  );
  return { posted, bodies };
}

function renderWorklist() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <WorklistScreen />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("a failed approval on the Worklist", () => {
  it("leaves the list once the reader acknowledges it", async () => {
    const user = userEvent.setup();
    const { posted, bodies } = stubServer(["acknowledge"], {
      "/v1/approvals/a1/failure-acknowledgement": 204,
    });
    renderWorklist();

    await screen.findByText(failedTitle);
    expect(screen.queryByRole("button", { name: "Run again" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Acknowledge" }));

    await waitFor(() => {
      expect(screen.queryByText(failedTitle)).toBeNull();
    });
    expect(posted).toEqual(["/v1/approvals/a1/failure-acknowledgement"]);
    expect(bodies).toEqual([{ failed_at: failedAt }]);
  });

  it("says so and keeps the row when a newer failure refuses the acknowledgement", async () => {
    const user = userEvent.setup();
    stubServer(["acknowledge"], {
      "/v1/approvals/a1/failure-acknowledgement": 409,
    });
    renderWorklist();

    await screen.findByText(failedTitle);
    await user.click(screen.getByRole("button", { name: "Acknowledge" }));

    expect(
      await screen.findByText("Item was not marked as seen. Retry."),
    ).toBeTruthy();
    expect(screen.getByText(failedTitle)).toBeTruthy();
  });

  it("approves the change again when the reader retries it", async () => {
    const user = userEvent.setup();
    const { posted } = stubServer(["acknowledge", "retry"], {
      "/v1/approvals/a1/approve": 200,
    });
    renderWorklist();

    await screen.findByText(failedTitle);
    await user.click(screen.getByRole("button", { name: "Run again" }));

    expect(await screen.findByText("Approved change completed")).toBeTruthy();
    expect(posted).toEqual(["/v1/approvals/a1/approve"]);
  });

  it("keeps the pressed button focusable while its request is pending", async () => {
    const user = userEvent.setup();
    let release = () => {};
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    stubServer(
      ["acknowledge", "retry"],
      { "/v1/approvals/a1/approve": 200 },
      held,
    );
    renderWorklist();

    await screen.findByText(failedTitle);
    const runAgain = screen.getByRole("button", { name: "Run again" });
    await user.click(runAgain);

    await waitFor(() => {
      expect(runAgain.getAttribute("aria-busy")).toBe("true");
    });
    expect(runAgain.hasAttribute("disabled")).toBe(false);
    expect(document.activeElement).toBe(runAgain);
    expect(
      screen
        .getByRole("button", { name: "Acknowledge" })
        .hasAttribute("disabled"),
    ).toBe(true);
    release();
    expect(await screen.findByText("Approved change completed")).toBeTruthy();
  });

  it("says so and keeps the row when the retry fails again", async () => {
    const user = userEvent.setup();
    stubServer(["acknowledge", "retry"], {
      "/v1/approvals/a1/approve": 500,
    });
    renderWorklist();

    await screen.findByText(failedTitle);
    await user.click(screen.getByRole("button", { name: "Run again" }));

    expect(
      await screen.findByText(
        "The approved change still did not run. Retry later, or acknowledge it.",
      ),
    ).toBeTruthy();
    expect(screen.getByText(failedTitle)).toBeTruthy();
  });
});
