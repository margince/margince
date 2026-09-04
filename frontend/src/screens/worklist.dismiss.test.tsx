// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { WorklistScreen } from "./worklist";
import { day, jsonResponse, row, stub } from "./worklist.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// Setting a quiet contact aside, and putting them back.
//
// Nobody is waiting on a lapsed relationship, which is exactly why the row kept
// coming back: there was no way to say "not this one, not now".

describe("a contact the reader is not chasing", () => {
  it("is set aside for a month, and can be put back", async () => {
    stub(
      day({
        queue: [
          row({
            id: "01a05500-0000-7000-8000-0000000000p1",
            source: "relationship_decay",
            title: "Kirsten Vogel",
            actions: ["open", "dismiss"],
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );
    const user = userEvent.setup();
    renderUnderAToastRegion();

    await user.click(
      await screen.findByRole("button", { name: en["worklist.verb.dismiss"] }),
    );

    // THIRTY days, chosen here rather than asked of the reader: a picker would
    // ask a rep to predict when a quiet relationship becomes worth chasing.
    await waitFor(async () => {
      expect(await dismissal()).toEqual({
        method: "PUT",
        body: { days: 30 },
      });
    });

    await user.click(
      await screen.findByRole("button", {
        name: en["worklist.verb.dismissUndo"],
      }),
    );

    // The undo is a DELETE, not a second dismissal with a shorter span.
    await waitFor(async () => {
      expect((await dismissal())?.method).toBe("DELETE");
    });
  });

  // The SERVER decides whether the verb is offered, and the row only draws what
  // it was given. Without this the row would show a control on every lapsed
  // contact, including the ones a future rule withholds it from — and the test
  // above cannot see that, because its own fixture always sends the verb.
  it("is offered no way to be set aside when the server withholds the verb", async () => {
    stub(
      day({
        queue: [
          row({
            id: "0199f5c0-0000-7000-8000-000000000d01",
            source: "relationship_decay",
            title: "Kirsten Vogel",
            actions: ["open"],
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );
    renderUnderAToastRegion();

    // Waited for, not asked once: the row arrives asynchronously, so a bare
    // absence check passes against a queue that has not rendered yet.
    expect(await screen.findByText("Kirsten Vogel")).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: en["worklist.verb.dismiss"] }),
    ).toBeNull();
  });

  // A REFUSED undo has to say so, and the row is gone by the time it is
  // pressed.
  //
  // The dismissal removes the contact from the lane, so the component that
  // owns the Undo button is unmounted before the reader can press it. A
  // per-call `onError` hangs off that component's React Query observer and is
  // dropped with it, so the failure reached nobody: the reader pressed the one
  // control that undoes their misclick, it was refused, and the screen said
  // nothing while the contact stayed hidden for a month.
  //
  // The test above cannot see this — its stub answers the same row forever, so
  // the component never unmounts and the dropped callback still fires.
  it("says so when the undo is refused after the row has gone", async () => {
    stubVanishingContact();
    const user = userEvent.setup();
    renderUnderAToastRegion();

    await user.click(
      await screen.findByRole("button", { name: en["worklist.verb.dismiss"] }),
    );
    // The row is gone: the second read answers an empty day, which is what
    // unmounts the component the callback would have hung off.
    await waitFor(() => {
      expect(screen.queryByText("Kirsten Vogel")).toBeNull();
    });

    await user.click(
      await screen.findByRole("button", {
        name: en["worklist.verb.dismissUndo"],
      }),
    );

    expect(
      await screen.findByText(en["worklist.verb.dismissUndoFailed"]),
    ).toBeTruthy();
  });
});

// stubVanishingContact answers the way the server does around a dismissal: the
// contact is on the lane until they are set aside and gone afterwards, and the
// undo is refused.
function stubVanishingContact() {
  let dismissed = false;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      const method = input instanceof Request ? input.method : "GET";
      if (url.includes("nudge-dismissal")) {
        if (method === "DELETE") {
          // A problem body, the way the server refuses: an empty 403 leaves
          // openapi-fetch nothing to parse into `error`, so the client would
          // read a refusal as a success and the test would prove nothing.
          return new Response(
            JSON.stringify({ title: "Forbidden", status: 403 }),
            { status: 403, headers: { "content-type": "application/json" } },
          );
        }
        dismissed = true;
        return new Response(null, { status: 204 });
      }
      if (url.split("?")[0].endsWith("/worklist")) {
        return jsonResponse(dismissed ? day() : DAY_WITH_THE_CONTACT);
      }
      return jsonResponse({ data: [] });
    }),
  );
}

const DAY_WITH_THE_CONTACT = day({
  queue: [
    row({
      id: "0199f5c0-0000-7000-8000-000000000d01",
      source: "relationship_decay",
      title: "Kirsten Vogel",
      actions: ["open", "dismiss"],
    }),
  ],
  summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
});

// renderUnderAToastRegion draws the screen the way the shell draws it.
//
// The confirmation this test presses lives in a toast, and renderWorklist
// mounts no region — the testkit is production-shaped for a conformance gate
// that allows one ToastProvider and one ToastRegion and names main.tsx as their
// home. So the region is mounted here, in a file the gate does not scan, the
// same way worklist.taskdone.test.tsx does it.
function renderUnderAToastRegion() {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({
          defaultOptions: {
            queries: { retry: false },
            // The removed mutation is reclaimed at once rather than after the
            // five-minute default. A reader takes seconds to read the
            // confirmation and press Undo, so by then the observer the
            // dismissal ran under is long gone; holding it for the length of
            // the test instead would keep a per-call `onError` alive that
            // production has already dropped, and the refused-undo test below
            // would pass against the bug it exists to catch.
            mutations: { gcTime: 0 },
          },
        })
      }
    >
      <LocaleProvider initial="en">
        <ToastProvider>
          <WorklistScreen />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// dismissal is the LAST nudge-dismissal call this render sent.
async function dismissal(): Promise<
  { method: string; body: unknown } | undefined
> {
  const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
  for (let at = calls.length - 1; at >= 0; at--) {
    const [input] = calls[at] as [RequestInfo | URL];
    if (!(input instanceof Request) || !input.url.includes("nudge-dismissal")) {
      continue;
    }
    if (input.method === "DELETE") {
      return { method: "DELETE", body: undefined };
    }
    return { method: input.method, body: await input.clone().json() };
  }
  return undefined;
}
