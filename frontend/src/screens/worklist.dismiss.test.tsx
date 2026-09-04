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
import { day, row, stub } from "./worklist.testkit";

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
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
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
