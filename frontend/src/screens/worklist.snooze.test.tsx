// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { DispositionVerbs } from "./worklist.dispositions";
import type { WorklistItem } from "./worklist.queries";

// How long a row is put down for.
//
// The server has always accepted any future instant — `snoozed_until` is
// caller-supplied and validated only as "later than now" — and this client only
// ever sent tomorrow. A rep who knew a customer was away all week pressed the
// same button seven mornings running, and each press was a row that came back
// and a count that stayed wrong.
//
// These assert the INSTANT that reaches the wire, because that is the whole
// change: the button already existed and already worked.

const NOW = new Date("2026-08-31T09:00:00Z");

function row(): WorklistItem {
  return {
    id: "01a05500-0000-7000-8000-0000000000a1",
    source: "customer_waiting",
    category: "customer_waiting",
    level: 1,
    consequence: "buyer_waits",
    title: "Anna Weber is waiting",
    because: [],
    actions: [],
    dispositions: ["snooze", "not_mine"],
  };
}

// The body each write carried, in order.
//
// The generated client hands `fetch` a Request rather than a (url, init) pair,
// so the body has to be read back off the Request — an `init.body` lookup finds
// null on every call and a test built on it reports "nothing was sent" for a
// request that went out correctly.
async function sentBodies(
  fetch: ReturnType<typeof vi.fn>,
): Promise<Array<Record<string, unknown>>> {
  const bodies: Array<Record<string, unknown>> = [];
  for (const [input, init] of fetch.mock.calls) {
    const request = input instanceof Request ? input.clone() : undefined;
    const raw = request ? await request.text() : String(init?.body ?? "");
    if (raw !== "") {
      bodies.push(JSON.parse(raw) as Record<string, unknown>);
    }
  }
  return bodies;
}

function draw() {
  const fetch = vi.fn(
    async () =>
      new Response(JSON.stringify({}), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
  );
  vi.stubGlobal("fetch", fetch);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <DispositionVerbs item={row()} />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return fetch;
}

// Days between the frozen now and the instant the client sent.
function daysSent(body: Record<string, unknown>): number {
  const until = new Date(String(body.snoozed_until));
  return Math.round((until.getTime() - NOW.getTime()) / 86_400_000);
}

beforeEach(() => {
  // shouldAdvanceTime, because react-query's own timers have to keep running:
  // a frozen clock never settles the mutation and the assertions below would
  // time out rather than fail on what they meant to check.
  vi.useFakeTimers({ shouldAdvanceTime: true });
  vi.setSystemTime(NOW);
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("how long a row is put down for", () => {
  // The fast path is unchanged. A rep reaching for "not today" most often means
  // tomorrow, and charging the common case a second click to buy the rare one
  // would be a worse trade than the limit it replaces.
  it("still snoozes until tomorrow on the plain press", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const fetch = draw();

    await user.click(
      screen.getByRole("button", {
        name: en["worklist.disposition.verb.snooze"],
      }),
    );

    await waitFor(async () => expect((await sentBodies(fetch)).length).toBe(1));
    expect(daysSent((await sentBodies(fetch))[0])).toBe(1);
  });

  // The case the change is for: a week, in one press instead of seven.
  //
  // EVERY declared span, not a sample. The list is the product decision, so a
  // test covering two of three would let the middle one be deleted or wired to
  // the wrong number with nothing failing.
  it.each([1, 3, 7])(
    "sends the %i-day span the reader picked",
    async (days) => {
      const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
      const fetch = draw();

      await user.click(screen.getByRole("button", { name: "For how long" }));
      await user.click(
        await screen.findByRole("button", {
          name: days === 1 ? "1 day" : `${days} days`,
        }),
      );

      await waitFor(async () =>
        expect((await sentBodies(fetch)).length).toBe(1),
      );
      expect(daysSent((await sentBodies(fetch))[0])).toBe(days);
    },
  );

  // The answer a span cannot give.
  //
  // Every duration above is a guess about when the customer will move, and the
  // rep is usually wrong in one of two directions. This asserts the WIRE rather
  // than the button, because a picker that renders the line and sends a
  // one-day snooze underneath looks identical on screen and is the same old
  // guess.
  it("sends the reply condition and no moment when the reader picks it", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const fetch = draw();

    await user.click(screen.getByRole("button", { name: "For how long" }));
    await user.click(
      await screen.findByRole("button", {
        name: en["worklist.disposition.snoozeUntil.reply"],
      }),
    );

    await waitFor(async () => expect((await sentBodies(fetch)).length).toBe(1));
    const body = (await sentBodies(fetch))[0];
    expect(body.reopen_on).toBe("reply");
    // No moment: the server refuses a reply snooze carrying one, and a client
    // that sent tomorrow alongside would turn every such press into a 422.
    expect(body.snoozed_until).toBeUndefined();
  });

  // The default press still names the clock EXPLICITLY.
  //
  // The server treats an absent condition as `time`, so omitting it would work
  // today — and would silently become a different snooze the day that default
  // changed. Saying which one is meant costs one field.
  it("names the clock condition on the plain press", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const fetch = draw();

    await user.click(
      screen.getByRole("button", {
        name: en["worklist.disposition.verb.snooze"],
      }),
    );

    await waitFor(async () => expect((await sentBodies(fetch)).length).toBe(1));
    expect((await sentBodies(fetch))[0].reopen_on).toBe("time");
  });

  // The spans are offered only where the verb is. A duration picker beside a
  // row the server never offered a snooze on is a control that 404s.
  it("offers no spans on a row the server does not let a reader snooze", () => {
    const client = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
    const unsnoozable = {
      ...row(),
      dispositions: ["not_mine"],
    } as WorklistItem;
    render(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <ToastProvider>
            <DispositionVerbs item={unsnoozable} />
            <ToastRegion />
          </ToastProvider>
        </LocaleProvider>
      </QueryClientProvider>,
    );

    expect(screen.queryByRole("button", { name: "For how long" })).toBeNull();
  });

  // What the confirmation SAYS a press did.
  //
  // The copy was fixed at "back tomorrow" and correct while tomorrow was the
  // only span. The moment a reader could pick seven days, that same line told
  // them the row returns tomorrow when it returns next week — a confirmation
  // that misstates what just happened, which the reader believes.
  it("says how long the row is actually gone for", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    draw();

    await user.click(screen.getByRole("button", { name: "For how long" }));
    await user.click(await screen.findByRole("button", { name: "7 days" }));

    expect(
      await screen.findByText("Back on your list in 7 days."),
    ).toBeTruthy();
  });

  // And the default press still says tomorrow, because for that press it is
  // true. A change that made every confirmation say "in 1 days" would be the
  // same defect wearing the fix's clothes.
  it("still says tomorrow on the plain press", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    draw();

    await user.click(
      screen.getByRole("button", {
        name: en["worklist.disposition.verb.snooze"],
      }),
    );

    expect(await screen.findByText("Back on your list tomorrow.")).toBeTruthy();
  });

  // Singular reads as singular. "1 days" is a different kind of wrong from a
  // missing feature — it is the product looking unfinished on every press.
  it("says one day, not one days", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    draw();

    await user.click(screen.getByRole("button", { name: "For how long" }));

    expect(await screen.findByRole("button", { name: "1 day" })).toBeTruthy();
  });
});
