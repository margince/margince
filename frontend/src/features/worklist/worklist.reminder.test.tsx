// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { OverflowMenu } from "../../design-system/atoms";
import { ToastProvider } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { ViewerZoneProvider } from "../../lib/viewerzone";
import { FIXTURE_TIMEZONE, meFixture } from "../../testing/mefixture";
import { row } from "./worklist.fixtures";
import { RowTaskReminder } from "./worklist.reminder";

const ME = meFixture({}).user.id;
const TASK = "01a05500-0000-7000-8000-000000000c01";
// The task as its own read answers it: newer than the row, and reminded.
const STORED = {
  id: TASK,
  kind: "task",
  version: 7,
  assignee_id: ME,
  remind_at: "2026-03-04T08:00:00.000Z",
};

let patches: { body: unknown; ifMatch: string | null }[] = [];

beforeEach(() => {
  patches = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : new Request(input);
      if (request.method === "PATCH") {
        patches.push({
          body: JSON.parse(await request.text()),
          ifMatch: request.headers.get("if-match"),
        });
      }
      return new Response(JSON.stringify(STORED), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }),
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function renderRowMenu(owner: { kind: "user"; id: string }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["me"], meFixture({}));
  return render(
    <QueryClientProvider client={client}>
      <ViewerZoneProvider zone={FIXTURE_TIMEZONE}>
        <LocaleProvider initial="en">
          <ToastProvider>
            <OverflowMenu label="Row actions">
              <RowTaskReminder
                item={row({
                  id: TASK,
                  version: 3,
                  owner,
                  actions: ["complete"],
                })}
              />
            </OverflowMenu>
          </ToastProvider>
        </LocaleProvider>
      </ViewerZoneProvider>
    </QueryClientProvider>,
  );
}

describe("RowTaskReminder", () => {
  it("opens the reminder lines in the row's menu and writes at the task's own version", async () => {
    const user = userEvent.setup();
    renderRowMenu({ kind: "user", id: ME });
    await user.click(screen.getByRole("button", { name: "Row actions" }));
    const line = screen.getByRole("button", { name: "Remind me" });
    await user.click(line);
    // A line that draws more open is a setting: the menu stays up under it.
    expect(line.getAttribute("aria-expanded")).toBe("true");
    await user.click(
      await screen.findByRole("button", { name: "Remove reminder" }),
    );
    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toEqual({ body: { remind_at: null }, ifMatch: "7" });
  });

  it("is not in the menu of a colleague's task", async () => {
    const user = userEvent.setup();
    renderRowMenu({
      kind: "user",
      id: "00000000-0000-4000-8000-0000000000c2",
    });
    await user.click(screen.getByRole("button", { name: "Row actions" }));
    expect(screen.queryByRole("button", { name: "Remind me" })).toBe(null);
  });
});
