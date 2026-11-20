// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useMessageAudience, useThreadAudience } from "./audienceservice";

// A thread decision changes several messages at once, and they are filed
// against whatever records each one touches. The caller names what its own
// screen has to refresh; the service refreshes every read that could be drawing
// one of those messages, because no caller can know which records they are on —
// the answer names ACTIVITIES, and `Activity.links` is deliberately not on it.
//
// The failure this guards is quiet: the row you pressed updates, the same
// message on another record keeps showing the audience it had before, and the
// control looks like it worked everywhere.

const REACHED = [
  "11111111-1111-4111-8111-111111111111",
  "22222222-2222-4222-8222-222222222222",
];

function Harness({ onDone }: Readonly<{ onDone?: () => void }>) {
  const mutation = useThreadAudience({
    invalidate: () => [["held-threads"]],
    onSettled: () => onDone?.(),
  });
  return (
    <button
      type="button"
      onClick={() => mutation.mutate({ threadKey: "t-1", share: true })}
    >
      release
    </button>
  );
}

// The reads a reader could have open elsewhere while they press release here:
// another record's timeline, the composite payload that carries a contact's
// first page, and a drawer anchored on a message this decision never named.
const ELSEWHERE = [
  ["activities", "deal", "d-1"],
  ["contact360", "p-9"],
  ["email-presentation", "33333333-3333-4333-8333-333333333333"],
];

// And the reads that carry no message, which a release must leave alone.
const UNTOUCHED = [["project", "j-1"], ["deals"]];

function renderHarness(body: unknown) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify(body), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
    ),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  // Cached rather than spied on: what matters is that these reads end up
  // needing to be re-read, not which call did it. A spy on invalidateQueries
  // would pass for a predicate that matched nothing.
  for (const key of [...ELSEWHERE, ...UNTOUCHED]) {
    client.setQueryData(key, { drawn: "before the release" });
  }
  const spy = vi.spyOn(client, "invalidateQueries");
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  const done = vi.fn();
  render(<Harness onDone={done} />, { wrapper });
  return { spy, done, client };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("useThreadAudience", () => {
  it("refreshes other records' timelines, not only the caller's own screen", async () => {
    const user = userEvent.setup();
    const { spy, done, client } = renderHarness({
      messages: 2,
      shared: true,
      held_by_others: 0,
      activity_ids: REACHED,
    });

    await user.click(screen.getByRole("button", { name: "release" }));
    await waitFor(() => expect(done).toHaveBeenCalled());

    // The caller's own key, because the queue it sits in has to reload.
    const invalidated = spy.mock.calls.map(([arg]) =>
      JSON.stringify(arg?.queryKey),
    );
    expect(invalidated).toContain(JSON.stringify(["held-threads"]));

    // And every read that could be drawing one of the changed messages,
    // whichever record it is filed against. None of these is the screen the
    // press happened on, and none of them is named by the answer.
    for (const key of ELSEWHERE) {
      expect(client.getQueryState(key)?.isInvalidated).toBe(true);
    }
    // A release is not a reason to re-read the whole cache.
    for (const key of UNTOUCHED) {
      expect(client.getQueryState(key)?.isInvalidated).toBe(false);
    }
  });

  it("reports a share that changed nothing, so the control does not look broken", async () => {
    const user = userEvent.setup();
    const outcomes: unknown[] = [];
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              messages: 1,
              shared: false,
              held_by_others: 1,
              activity_ids: [REACHED[0]],
            }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
      ),
    );
    function Reporting() {
      const mutation = useThreadAudience({
        invalidate: () => [],
        onSettled: (result) => outcomes.push(result.outcome),
      });
      return (
        <button
          type="button"
          onClick={() => mutation.mutate({ threadKey: "t-1", share: true })}
        >
          release
        </button>
      );
    }
    render(
      <QueryClientProvider client={client}>
        <Reporting />
      </QueryClientProvider>,
    );

    await user.click(screen.getByRole("button", { name: "release" }));
    await waitFor(() => expect(outcomes).toHaveLength(1));

    // A share that did not open the thread means somebody else still holds it,
    // and the count reaches the caller so it can say so. Never a name.
    expect(outcomes[0]).toMatchObject({ shared: false, held_by_others: 1 });
  });
});

// The open message is drawn from its own read, and the write's answer cannot
// stand in for it. A write reported done before that read lands closes the
// dialog over a message that still says its old audience.
describe("an audience write reports done only once the open message shows it", () => {
  const MESSAGE = "44444444-4444-4444-8444-444444444444";
  const OPEN_MESSAGE = ["email-presentation", MESSAGE, 1];

  function serverWithAudience(before: string, after: string) {
    const server = { audience: before };
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        server.audience = after;
        return new Response(
          JSON.stringify({ id: MESSAGE, audience: after, version: 4 }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }),
    );
    return server;
  }

  function OpenMessage({
    server,
    children,
  }: Readonly<{ server: { audience: string }; children: ReactNode }>) {
    const read = useQuery({
      queryKey: OPEN_MESSAGE,
      queryFn: async () => ({ audience: server.audience }),
    });
    return (
      <>
        <p>{read.data?.audience}</p>
        {children}
      </>
    );
  }

  function MessageSave({ onDone }: Readonly<{ onDone: () => void }>) {
    const mutation = useMessageAudience({
      invalidate: () => [],
      onSettled: onDone,
    });
    return (
      <button
        type="button"
        onClick={() =>
          mutation.mutate({
            activityId: MESSAGE,
            version: 3,
            audience: "participants",
          })
        }
      >
        save
      </button>
    );
  }

  function ThreadSave({ onDone }: Readonly<{ onDone: () => void }>) {
    const mutation = useThreadAudience({
      invalidate: () => [],
      onSettled: onDone,
    });
    return (
      <button
        type="button"
        onClick={() => mutation.mutate({ threadKey: "t-1", share: false })}
      >
        save
      </button>
    );
  }

  async function shownWhenDone(
    server: { audience: string },
    Save: typeof MessageSave,
  ) {
    const user = userEvent.setup();
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const seenWhenDone: unknown[] = [];
    render(
      <QueryClientProvider client={client}>
        <OpenMessage server={server}>
          <Save
            onDone={() => seenWhenDone.push(client.getQueryData(OPEN_MESSAGE))}
          />
        </OpenMessage>
      </QueryClientProvider>,
    );
    await screen.findByText("workspace");
    await user.click(screen.getByRole("button", { name: "save" }));
    await waitFor(() => expect(seenWhenDone).toHaveLength(1));
    return seenWhenDone[0];
  }

  it("a message's own audience", async () => {
    const server = serverWithAudience("workspace", "participants");
    expect(await shownWhenDone(server, MessageSave)).toEqual({
      audience: "participants",
    });
  });

  it("a thread's audience", async () => {
    const server = serverWithAudience("workspace", "private");
    expect(await shownWhenDone(server, ThreadSave)).toEqual({
      audience: "private",
    });
  });
});
