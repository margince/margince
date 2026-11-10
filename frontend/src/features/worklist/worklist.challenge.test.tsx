// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { retryQuery } from "../../app/queryclient";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { EmptySuccessError } from "../../lib/problem";
import { worklistKey } from "../../lib/workkeys";
import {
  day,
  jsonResponse,
  row,
  type Worklist,
} from "../../testing/worklist.testkit";
import { WorklistScreen } from "./worklist";

// The day read through the real client, query and screen when an intermediary
// answers it with a challenge instead of the API.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function challenge(): Response {
  return new Response(null, {
    status: 202,
    headers: {
      "content-type": "text/html; charset=UTF-8",
      "x-amzn-waf-action": "challenge",
    },
  });
}

function dayWith(title: string): Worklist {
  return day({
    queue: [row({ id: title, title })],
    summary: { urgent: 0, due: 1, lower_priority: 0, total: 1 },
  });
}

// The day's answers in order; every other read gets an empty list.
function serveDays(answers: (() => Response)[]) {
  const reads: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname;
      if (!path.endsWith("/worklist")) return jsonResponse({ data: [] });
      reads.push(path);
      const next = answers.length > 1 ? answers.shift() : answers[0];
      return next ? next() : challenge();
    }),
  );
  return reads;
}

// The failure the day's query holds: the client's own, not a property read
// that a missing page would have thrown later.
function dayFailure(client: QueryClient): unknown {
  const [read] = client.getQueryCache().findAll({ queryKey: worklistKey });
  return read?.state.error;
}

function renderDay() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: retryQuery } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <WorklistScreen />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return client;
}

describe("a Worklist an intermediary challenged", () => {
  it("says the day did not load, and Retry brings it back", async () => {
    const user = userEvent.setup();
    const reads = serveDays([
      challenge,
      () => jsonResponse(dayWith("Call Katrin back")),
    ]);
    const client = renderDay();

    expect(await screen.findByText(en["state.failed"])).toBeTruthy();
    expect(dayFailure(client)).toBeInstanceOf(EmptySuccessError);
    // Not retried behind the reader's back, so a standing challenge is asked once.
    expect(reads).toHaveLength(1);

    await user.click(screen.getByRole("button", { name: en["state.retry"] }));

    expect(await screen.findByText("Call Katrin back")).toBeTruthy();
    expect(reads).toHaveLength(2);
  });

  it("keeps the rows on screen through a challenged refresh", async () => {
    const reads = serveDays([
      () => jsonResponse(dayWith("Call Katrin back")),
      challenge,
      () => jsonResponse(dayWith("Send Philipp the offer")),
    ]);
    const client = renderDay();
    expect(await screen.findByText("Call Katrin back")).toBeTruthy();

    await act(() => client.refetchQueries());

    await waitFor(() => expect(reads).toHaveLength(2));
    expect(screen.getByText("Call Katrin back")).toBeTruthy();
    expect(screen.queryByText(en["state.failed"])).toBeNull();
    await waitFor(() =>
      expect(dayFailure(client)).toBeInstanceOf(EmptySuccessError),
    );

    await act(() => client.refetchQueries());

    expect(await screen.findByText("Send Philipp the offer")).toBeTruthy();
  });
});
