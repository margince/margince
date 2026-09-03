// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { emailPresentationKey } from "./activitykeys";
import { useThreadAudience } from "./audienceservice";

// A thread decision changes several messages at once, and they are filed
// against whatever records each one touches. The caller names what its own
// screen has to refresh; the service refreshes the messages the server says
// were reached, because no caller can know that list and every caller needs it.
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
  const spy = vi.spyOn(client, "invalidateQueries");
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  const done = vi.fn();
  render(<Harness onDone={done} />, { wrapper });
  return { spy, done };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("useThreadAudience", () => {
  it("refreshes every message the decision reached, not only the caller's own screen", async () => {
    const user = userEvent.setup();
    const { spy, done } = renderHarness({
      messages: 2,
      shared: true,
      held_by_others: 0,
      activity_ids: REACHED,
    });

    await user.click(screen.getByRole("button", { name: "release" }));
    await waitFor(() => expect(done).toHaveBeenCalled());

    const invalidated = spy.mock.calls.map(([arg]) =>
      JSON.stringify(arg?.queryKey),
    );
    // The caller's own key, because the queue it sits in has to reload.
    expect(invalidated).toContain(JSON.stringify(["held-threads"]));
    // And every message the server named, wherever else it is mounted.
    for (const id of REACHED) {
      expect(invalidated).toContain(JSON.stringify(emailPresentationKey(id)));
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
