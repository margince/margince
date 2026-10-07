// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A second press on Done, and a Done the server refuses as stale.
//
// The first press completes the task, but the row stays drawn at its old
// version until the list reloads. Pressing again re-sends that old version and
// is refused as skew, and the row then says the task was NOT completed when it
// was. Its own file: worklist.taskdone.test.tsx holds the verb's happy path.

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { WorklistScreen } from "./worklist";
import { day, jsonResponse, row } from "./worklist.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const openDay = () =>
  day({
    queue: [
      row({
        id: "task-1",
        source: "task",
        title: "Send the retrofit quote",
        actions: ["complete"],
        primary_action: "complete",
      }),
    ],
    summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
  });

function renderUnderAToastRegion() {
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

function patchCount(): number {
  const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
  return calls.filter(
    ([input]) => input instanceof Request && input.method === "PATCH",
  ).length;
}

describe("a second press on Done", () => {
  it("sends nothing while the list is still reloading", async () => {
    let completed = false;
    let releaseReload: () => void = () => undefined;
    const reload = new Promise<void>((resolve) => {
      releaseReload = resolve;
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const request = input instanceof Request ? input : undefined;
        if (request?.method === "PATCH") {
          completed = true;
          return jsonResponse({ version: 4 });
        }
        if (/\/worklist/.test(String(request ? request.url : input))) {
          if (completed) {
            // The reload is slow: the row is still on screen, at its old
            // version, for as long as this is held.
            await reload;
          }
          return jsonResponse(openDay());
        }
        return jsonResponse({ data: [] });
      }),
    );
    const user = userEvent.setup();
    renderUnderAToastRegion();

    const done = await screen.findByRole("button", {
      name: en["tasks.complete"],
    });
    await user.click(done);
    await waitFor(() => expect(completed).toBe(true));
    await user.click(done);
    await user.click(done);

    expect(patchCount()).toBe(1);
    releaseReload();
  });
});

describe("a Done refused as stale", () => {
  it("says the task changed rather than that it was not completed, and reloads", async () => {
    let worklistReads = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const request = input instanceof Request ? input : undefined;
        if (request?.method === "PATCH") {
          return new Response(
            JSON.stringify({
              title: "Conflict",
              status: 409,
              code: "version_skew",
            }),
            {
              status: 409,
              headers: { "content-type": "application/problem+json" },
            },
          );
        }
        if (/\/worklist/.test(String(request ? request.url : input))) {
          worklistReads += 1;
          return jsonResponse(openDay());
        }
        return jsonResponse({ data: [] });
      }),
    );
    const user = userEvent.setup();
    renderUnderAToastRegion();

    const done = await screen.findByRole("button", {
      name: en["tasks.complete"],
    });
    const readsBeforeRefusal = worklistReads;
    await user.click(done);

    await waitFor(() => {
      expect(screen.getByText(en["worklist.verb.completeStale"])).toBeTruthy();
    });
    expect(screen.queryByText(en["worklist.verb.completeFailed"])).toBeNull();
    await waitFor(() => {
      expect(worklistReads).toBeGreaterThan(readsBeforeRefusal);
    });
  });
});
