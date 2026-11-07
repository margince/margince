// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { meetingRow } from "../../testing/brief.fixtures";
import { type WorklistItem, worklistKey } from "./worklist.queries";
import { WorklistRow } from "./worklist.row";

// "I'm ready" on the reader's own meeting. The press sends the revision the row
// was read at. The toast offers the undo, and the row says what the server says.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function draw(item: WorklistItem, status = 200) {
  const fetch = vi.fn(
    async () =>
      new Response(
        status === 200
          ? "{}"
          : '{"title":"Conflict","status":409,"code":"version_skew"}',
        {
          status,
          headers: { "content-type": "application/json" },
        },
      ),
  );
  vi.stubGlobal("fetch", fetch);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <WorklistRow item={item} position={1} owner="" />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return fetch;
}

async function sent(fetch: ReturnType<typeof vi.fn>, call: number) {
  const [input] = fetch.mock.calls[call] ?? [];
  const request = input as Request;
  return {
    route: `${request.method} ${new URL(request.url).pathname}`,
    body: JSON.parse(await request.clone().text()) as Record<string, unknown>,
  };
}

describe("I'm ready on a meeting the reader still has to prepare", () => {
  it("sends the revision the row was read at, and the undo takes it back", async () => {
    const user = userEvent.setup();
    const fetch = draw(meetingRow("m1", false));

    await user.click(
      screen.getByRole("button", { name: en["worklist.ready.confirm"] }),
    );
    const confirm = await sent(fetch, 0);
    expect(confirm.route).toMatch(/PUT .*\/worklist\/ready$/);
    expect(confirm.body).toEqual({
      activity_id: "m1",
      preparation_revision: "rev-1",
    });

    await user.click(
      await screen.findByRole("button", {
        name: en["worklist.disposition.undo"],
      }),
    );
    await waitFor(() =>
      expect(fetch.mock.calls.length).toBeGreaterThanOrEqual(2),
    );
    const undo = await sent(fetch, 1);
    expect(undo.route).toMatch(/POST .*\/worklist\/ready\/undo$/);
    expect(undo.body).toEqual({
      activity_id: "m1",
      preparation_revision: "rev-1",
    });
  });

  it("says the meeting changed and reloads the row when the server refuses an old revision", async () => {
    const user = userEvent.setup();
    const reload = vi.spyOn(QueryClient.prototype, "invalidateQueries");
    draw(meetingRow("m1", false), 409);

    await user.click(
      screen.getByRole("button", { name: en["worklist.ready.confirm"] }),
    );

    expect(await screen.findByText(en["worklist.ready.stale"])).toBeTruthy();
    expect(reload).toHaveBeenCalledWith({ queryKey: worklistKey });
    reload.mockRestore();
  });

  it("names what changed since the reader was ready", () => {
    const item = meetingRow("m1", false);
    draw({
      ...item,
      preparation: {
        activity_id: "m1",
        state: "needs_review",
        revision: "rev-2",
        changed: ["start_time", "participants"],
      },
    });

    expect(
      screen.getByText("Changed since you were ready: start time, guests"),
    ).toBeTruthy();
  });
});

describe("an undo that fails", () => {
  it("says so after the row turned ready and lost its button", async () => {
    const user = userEvent.setup();
    const statuses = [200, 500];
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        const status = statuses.shift() ?? 500;
        return new Response(
          status === 200 ? "{}" : '{"title":"Internal","status":500}',
          { status, headers: { "content-type": "application/json" } },
        );
      }),
    );
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
    const page = (item: WorklistItem) => (
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <ToastProvider>
            <WorklistRow item={item} position={1} owner="" />
            <ToastRegion />
          </ToastProvider>
        </LocaleProvider>
      </QueryClientProvider>
    );
    const { rerender } = render(page(meetingRow("m1", false)));

    await user.click(
      screen.getByRole("button", { name: en["worklist.ready.confirm"] }),
    );
    const undo = await screen.findByRole("button", {
      name: en["worklist.disposition.undo"],
    });
    rerender(page(meetingRow("m1", true)));
    expect(
      screen.queryByRole("button", { name: en["worklist.ready.confirm"] }),
    ).toBeNull();
    await user.click(undo);

    expect(
      await screen.findByText(en["worklist.ready.undoFailed"]),
    ).toBeTruthy();
  });
});

describe("a meeting the reader is ready for", () => {
  it("stays as a reminder with no button and no Needs prep", () => {
    draw(meetingRow("m1", true));

    expect(screen.getByText(en["worklist.ready.badge"])).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: en["worklist.ready.confirm"] }),
    ).toBeNull();
    expect(screen.queryByText(en["worklist.needsPrep"])).toBeNull();
    expect(screen.getByText(/starts/i)).toBeTruthy();
  });
});

describe("a meeting the reader does not attend", () => {
  it("offers no readiness at all", () => {
    const { preparation: _absent, ...item } = meetingRow("m1", false);
    draw({ ...item, because: [], consequence: "none" });

    expect(
      screen.queryByRole("button", { name: en["worklist.ready.confirm"] }),
    ).toBeNull();
    expect(screen.queryByText(en["worklist.ready.badge"])).toBeNull();
  });
});
