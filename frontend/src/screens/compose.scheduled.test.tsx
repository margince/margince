/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { pickOption } from "../design-system/select-testing";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { ComposeModal } from "./compose";

// The door out of a scheduled send.
//
// The composer already computed whether it had scheduled — 201 waits, 202 has
// gone — and its own comment said the caller is told "so it can say so". It
// then closed both the same way and said nothing, while the confirm dialog the
// rep had just read promised: "you can move it or take it back from Scheduled
// messages." Nothing in the product went there (issue #1533).
//
// Its own file rather than compose.test.tsx, which is 2128 lines against the
// 1000-line ceiling frontend/AGENTS.md sets.

type Activity = components["schemas"]["Activity"];

// A reason that carries no unsubscribe surface, so the form is sendable with
// nothing else filled in.
const WHY_LABEL = "About a deal we are working on";

const ACTIVITY: Activity = {
  id: "act-1",
  kind: "email",
  subject: "Re: Q3",
  occurred_at: "2026-07-01T00:00:00Z",
  is_done: false,
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-07-01T00:00:00Z",
  updated_at: "2026-07-01T00:00:00Z",
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// The send answers with the status under test and everything else answers
// empty, so nothing this screen reads on mount can fail the case for a reason
// that is not about the door.
function stubSend(status: number) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      if (method === "POST" && url.pathname.endsWith("/send-email")) {
        return jsonResponse(ACTIVITY, status);
      }
      return jsonResponse({ data: [], page: { next_cursor: null } });
    }),
  );
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          {ui}
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

async function composeAndSend() {
  const onClose = vi.fn();
  render(
    <ComposeModal
      activityId="act-1"
      entityType="person"
      entityId="p-1"
      open
      onClose={onClose}
    />,
  );
  await screen.findByRole("combobox");
  await userEvent.type(screen.getByLabelText("To"), "a@x.com");
  await userEvent.tab();
  await userEvent.type(screen.getByPlaceholderText("Subject"), "Hi there");
  await userEvent.type(screen.getByPlaceholderText("Body"), "Body content");
  await pickOption(userEvent.setup(), screen.getByRole("combobox"), WHY_LABEL);
  await userEvent.click(screen.getByRole("button", { name: "Send" }));
  return onClose;
}

beforeEach(() => {
  window.location.hash = "";
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("a scheduled send is reachable again", () => {
  it("names the queue and opens it", async () => {
    stubSend(201);
    await composeAndSend();

    // The message says the send has NOT happened, which is the half a rep acts
    // on: a confirmation reading "sent" beside a door to withdraw it would
    // contradict itself.
    expect(
      await screen.findByText("Scheduled. It has not gone out yet."),
    ).toBeTruthy();

    const door = screen.getByRole("button", { name: "Scheduled messages" });
    await userEvent.click(door);
    expect(window.location.hash).toBe("#/scheduled");
  });

  // The door belongs to the outcome, not to the composer: a message that has
  // GONE cannot be moved or taken back, and offering the queue for one would
  // point a rep at a screen their message is not on.
  it("offers no door when the message has already gone", async () => {
    stubSend(202);
    const onClose = await composeAndSend();

    // Waited on the send having COMPLETED, not on a duration: an absence
    // asserted before the response landed is an absence that would survive the
    // door being added back.
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(
      screen.queryByRole("button", { name: "Scheduled messages" }),
    ).toBeNull();
    expect(
      screen.queryByText("Scheduled. It has not gone out yet."),
    ).toBeNull();
  });
});
