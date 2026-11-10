// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { retryQuery } from "../../app/queryclient";
import { ToastProvider } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { ScheduledSendsScreen } from "./scheduledsends";

// The queue read through the real client, query and screen when an
// intermediary answers it with a challenge instead of the API.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const readerZone = Intl.DateTimeFormat().resolvedOptions().timeZone;

function waiting(subject: string) {
  return {
    id: "019f7e65-fbf7-7114-b114-40af4af63ae8",
    status: "scheduled",
    scheduled_at: "2026-09-01T07:00:00Z",
    scheduled_tz: readerZone,
    subject,
    to: ["ceo@acme.test"],
    version: 3,
    created_at: "2026-08-20T09:00:00Z",
    updated_at: "2026-08-20T09:00:00Z",
  };
}

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    headers: { "content-type": "application/json" },
  });
}

function challenge(): Response {
  return new Response(null, {
    status: 202,
    headers: {
      "content-type": "text/html; charset=UTF-8",
      "x-amzn-waf-action": "challenge",
    },
  });
}

// The queue's answers in order; every other read gets an empty list.
function serveQueue(answers: (() => Response)[]) {
  const reads: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname;
      if (!path.endsWith("/scheduled-sends")) return json({ data: [] });
      reads.push(path);
      const next = answers.length > 1 ? answers.shift() : answers[0];
      return next ? next() : challenge();
    }),
  );
  return reads;
}

function renderQueue() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: retryQuery } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <ScheduledSendsScreen />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return client;
}

describe("a scheduled-send queue an intermediary challenged", () => {
  it("says the queue did not load, and Retry brings it back", async () => {
    const user = userEvent.setup();
    const reads = serveQueue([
      challenge,
      () => json([waiting("The renewal quote")]),
    ]);
    renderQueue();

    expect(await screen.findByText(en["common.error"])).toBeTruthy();
    // Not an empty queue: a failed read never says nothing is waiting.
    expect(screen.queryByText(en["sched.group.waitingEmpty"])).toBeNull();
    expect(reads).toHaveLength(1);

    await user.click(screen.getByRole("button", { name: en["common.retry"] }));

    expect(await screen.findByText("The renewal quote")).toBeTruthy();
  });

  it("keeps the queue on screen through a challenged refresh", async () => {
    const reads = serveQueue([
      () => json([waiting("The renewal quote")]),
      challenge,
      () => json([waiting("The revised quote")]),
    ]);
    const client = renderQueue();
    expect(await screen.findByText("The renewal quote")).toBeTruthy();

    await act(() => client.refetchQueries({ queryKey: ["scheduled-sends"] }));

    await waitFor(() => expect(reads).toHaveLength(2));
    expect(screen.getByText("The renewal quote")).toBeTruthy();
    expect(screen.queryByText(en["common.error"])).toBeNull();

    await act(() => client.refetchQueries({ queryKey: ["scheduled-sends"] }));

    expect(await screen.findByText("The revised quote")).toBeTruthy();
  });
});
