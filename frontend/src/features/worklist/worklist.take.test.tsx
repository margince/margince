// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { findRowMenu, jsonResponse, row } from "../../testing/worklist.testkit";
import type { WorklistItem } from "./worklist.queries";
import { RowActs } from "./worklist.rowverbs";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const READER = "u-reader";

function stubSession(write: Response) {
  const fetched = vi.fn(async (input: RequestInfo | URL) => {
    const request = input instanceof Request ? input : undefined;
    if (request && request.method !== "GET") {
      return write.clone();
    }
    if (String(request ? request.url : input).includes("/me")) {
      return jsonResponse({ user: { id: READER, display_name: "Lena" } });
    }
    return jsonResponse({ data: [] });
  });
  vi.stubGlobal("fetch", fetched);
  return fetched;
}

function renderRow(item: WorklistItem) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <RowActs
            item={item}
            title="Send the quote"
            href={undefined}
            owner=""
          />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

function writesSent(fetched: ReturnType<typeof vi.fn>): Request[] {
  return fetched.mock.calls
    .map(([input]) => input)
    .filter(
      (input): input is Request =>
        input instanceof Request && input.method !== "GET",
    );
}

const unheld = () =>
  row({ id: "task-1", actions: ["complete", "snooze", "take"] });

describe("taking a task nobody holds", () => {
  it("assigns the task to the reader, pinned to the row's version", async () => {
    const fetched = stubSession(jsonResponse({ id: "task-1", links: [] }));
    const user = userEvent.setup();
    renderRow(unheld());

    await user.click(
      await screen.findByRole("button", { name: en["worklist.take.action"] }),
    );

    expect(await screen.findByText(en["worklist.take.done"])).toBeTruthy();
    const [sent] = writesSent(fetched);
    expect(sent?.method).toBe("PATCH");
    expect(sent?.url).toContain("/activities/task-1");
    expect(sent?.headers.get("If-Match")).toBe("3");
    expect(await sent?.clone().json()).toEqual({ assignee_id: READER });
  });

  it("says so when the take is refused", async () => {
    stubSession(
      new Response(JSON.stringify({ title: "Forbidden", status: 403 }), {
        status: 403,
        headers: { "content-type": "application/problem+json" },
      }),
    );
    const user = userEvent.setup();
    renderRow(unheld());

    await user.click(
      await screen.findByRole("button", { name: en["worklist.take.action"] }),
    );

    expect(await screen.findByText(en["worklist.take.failed"])).toBeTruthy();
  });

  it("is not drawn on a row the server did not offer it on", async () => {
    stubSession(jsonResponse({}));
    renderRow(row({ id: "task-1", actions: ["complete", "snooze"] }));

    await findRowMenu();
    expect(
      screen.queryByRole("button", { name: en["worklist.take.action"] }),
    ).toBeNull();
  });
});

describe("the hand-off on a row the reader may only read", () => {
  it("is not offered", async () => {
    stubSession(jsonResponse({}));
    const user = userEvent.setup();
    renderRow(
      row({
        id: "task-1",
        actions: [],
        because: [{ kind: "unassigned" }, { kind: "read_only" }],
      }),
    );

    await user.click(await findRowMenu());
    expect(
      screen.queryByRole("button", { name: en["worklist.manager.reassign"] }),
    ).toBeNull();
  });

  it("is still offered on a row the reader may change", async () => {
    stubSession(jsonResponse({}));
    const user = userEvent.setup();
    renderRow(row({ id: "task-1", actions: ["complete", "snooze", "take"] }));

    await user.click(await findRowMenu());
    expect(
      await screen.findByRole("button", {
        name: en["worklist.manager.reassign"],
      }),
    ).toBeTruthy();
  });
});
