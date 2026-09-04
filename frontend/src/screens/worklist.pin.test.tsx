// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */

// The reader's own override.
//
// Every other control on this page changes what the SERVER thinks — a
// disposition, a filter, a scope. This is the only one that says "I know, and I
// want this first anyway", and the ranking has carried a pin level since it was
// written with nothing on screen able to set it.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { WorklistScreen } from "./worklist";
import { day, renderWorklist, row } from "./worklist.testkit";

// The confirmation this suite reads lives in a toast, and renderWorklist mounts
// no region — the conformance gate allows exactly one ToastProvider and one
// ToastRegion in production-shaped files and names main.tsx as their home, and
// the testkit is scanned as one. So the region is mounted here, in a file the
// gate does not scan, exactly as archive.test.tsx does it.
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

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The calls this render made, by method — the address and the body are what a
// label cannot tell you, and a button wired the wrong way round renders
// identically to one wired correctly.
function pinCalls(fetched: ReturnType<typeof vi.fn>) {
  return fetched.mock.calls
    .map(([input]) => (input instanceof Request ? input : undefined))
    .filter((request) => request?.url.includes("/worklist/pins"));
}

function stubbing(queue: ReturnType<typeof row>[]) {
  const fetched = vi.fn(async (input: RequestInfo | URL) => {
    const request = input instanceof Request ? input : undefined;
    const url = String(request ? request.url : input);
    if (url.includes("/worklist/pins")) {
      return new Response(null, { status: 204 });
    }
    if (url.includes("/worklist")) {
      return new Response(
        JSON.stringify(
          day({
            queue,
            summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
          }),
        ),
        { status: 200, headers: { "content-type": "application/json" } },
      );
    }
    return new Response(JSON.stringify({ data: [] }), {
      status: 200,
      headers: { "content-type": "application/json" },
    });
  });
  vi.stubGlobal("fetch", fetched);
  return fetched;
}

describe("a reader can put a row at the top of their own day", () => {
  it("pins by the row's identity, which is the pair and not the id", async () => {
    const fetched = stubbing([
      row({ id: "task-1", source: "task", title: "Send the retrofit quote" }),
    ]);
    renderWorklist();

    await userEvent.click(
      await screen.findByRole("button", { name: en["worklist.verb.pin"] }),
    );

    await waitFor(async () => {
      const [call] = pinCalls(fetched);
      expect(call?.method).toBe("PUT");
      // The SOURCE travels with the id. The lanes mint ids independently, so an
      // id alone can name a row in a lane the reader was not looking at — and
      // the pin store keys on the pair for exactly that reason.
      expect(await call?.clone().json()).toEqual({
        source: "task",
        row_id: "task-1",
      });
    });
  });

  it("offers the way back on a row that already leads the day", async () => {
    const fetched = stubbing([
      row({
        id: "task-1",
        source: "task",
        title: "Send the retrofit quote",
        // The server says so, and the client reads it rather than remembering
        // what it pressed: a local flag would disagree with the page the moment
        // the reader pinned from another tab.
        because: [{ kind: "pinned" }],
      }),
    ]);
    renderWorklist();

    await userEvent.click(
      await screen.findByRole("button", { name: en["worklist.verb.unpin"] }),
    );

    await waitFor(() => {
      const [call] = pinCalls(fetched);
      expect(call?.method).toBe("DELETE");
      expect(call?.url).toContain("source=task");
      expect(call?.url).toContain("row_id=task-1");
    });
  });

  it("says so when the write is refused, rather than leaving the row unmoved and silent", async () => {
    const fetched = vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : undefined;
      const url = String(request ? request.url : input);
      if (url.includes("/worklist/pins")) {
        return new Response(
          JSON.stringify({ title: "Forbidden", status: 403 }),
          {
            status: 403,
            headers: { "content-type": "application/problem+json" },
          },
        );
      }
      if (url.includes("/worklist")) {
        return new Response(
          JSON.stringify(
            day({
              queue: [
                row({
                  id: "task-1",
                  source: "task",
                  title: "Send the retrofit quote",
                }),
              ],
              summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
            }),
          ),
          { status: 200, headers: { "content-type": "application/json" } },
        );
      }
      return new Response(JSON.stringify({ data: [] }), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    });
    vi.stubGlobal("fetch", fetched);
    renderUnderAToastRegion();

    await userEvent.click(
      await screen.findByRole("button", { name: en["worklist.verb.pin"] }),
    );

    // A refused pin leaves the row exactly where it was, which is also what a
    // press that did nothing looks like. Saying so is the difference.
    expect(await screen.findByText(en["worklist.verb.pinFailed"])).toBeTruthy();
  });

  it("offers no pin on a folded group, whose id the fold mints", async () => {
    stubbing([
      row({
        id: "batch-1",
        source: "batch",
        title: "Twelve contact questions",
        batch: { key: "uncertain_contact", count: 12 },
      }),
    ]);
    renderWorklist();

    await screen.findByText(/12 addresses to decide on/);
    // A group's id is synthetic and minted by the fold, so a pin on one names a
    // group that will not exist under that key on the next read — the reader
    // would press it, and their day would look unchanged forever.
    expect(
      screen.queryByRole("button", { name: en["worklist.verb.pin"] }),
    ).toBeNull();
  });
});
