// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Finishing a task from the row, and taking it back.
//
// Its own file rather than more of worklist.test.tsx, which had reached the
// thousand-line ceiling. The seam is the subject: everything here is about ONE
// verb and what happens after it — the write, the confirmation, the undo, and
// the undo being refused — while the file it left covers what the queue draws.

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

// Done REMOVES the row, which is what makes an undo worth having: a misclick
// otherwise costs the reader the only address they had for that task, and they
// have to remember what it was to find it again. Every disposition beside it is
// undoable from its own confirmation; this verb was not.
describe("finishing a task from the row", () => {
  it("completes it, then reopens it when the reader undoes", async () => {
    stub(
      day({
        queue: [
          row({
            id: "task-1",
            source: "task",
            title: "Send the retrofit quote",
            actions: ["complete"],
            primary_action: "complete",
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );
    const user = userEvent.setup();
    renderUnderAToastRegion();

    await user.click(
      await screen.findByRole("button", { name: en["tasks.complete"] }),
    );

    // The write itself, not merely that a button was pressed: a control that
    // renamed the promise and sent nothing would satisfy a visibility check.
    await waitFor(async () => {
      expect(await patchedTask()).toEqual({
        id: "task-1",
        body: { is_done: true },
      });
    });

    await user.click(
      await screen.findByRole("button", {
        name: en["worklist.verb.completeUndo"],
      }),
    );

    await waitFor(async () => {
      expect(await patchedTask()).toEqual({
        id: "task-1",
        body: { is_done: false },
      });
    });
  });
});

// The figures still describe the rows after the work is done.
//
// The plan asks for counts that reconcile "after load, action, and refresh",
// and the load half was already held (worklist_test.go and paging). This is the
// ACTION half, and it is the one that can rot silently: a client that removes
// the finished row and leaves the headline alone still draws a queue, still
// draws a number, and the two simply disagree — one row on screen under a
// sentence saying two are waiting.
//
// The server answers DIFFERENTLY after the write, which is the whole point. A
// stub that replayed the first day would let a client that never re-read pass:
// the row would vanish from the DOM optimistically and the stale figure would
// be the one still on screen, which is exactly the defect.
describe("the day's figures after the work is done", () => {
  it("AC-WORKLIST-SDR-04: re-reads the count rather than leaving the old one on screen", async () => {
    const before = day({
      queue: [
        row({
          id: "task-1",
          source: "task",
          title: "Send the retrofit quote",
          actions: ["complete"],
          primary_action: "complete",
        }),
        row({
          id: "task-2",
          source: "task",
          title: "Call the buyer back",
          actions: ["complete"],
          primary_action: "complete",
        }),
      ],
      summary: { urgent: 0, due: 0, lower_priority: 2, total: 2 },
    });
    const after = day({
      queue: [
        row({
          id: "task-2",
          source: "task",
          title: "Call the buyer back",
          actions: ["complete"],
          primary_action: "complete",
        }),
      ],
      summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
    });
    // One day before the write and the other after it, decided by whether the
    // PATCH has been seen rather than by a call count — the queue is read more
    // than once on arrival, and counting reads would make this depend on how
    // many.
    let done = false;
    let readAfterTheWrite = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input instanceof Request ? input.url : input);
        if (input instanceof Request && input.method === "PATCH") {
          done = true;
          return jsonResponse({});
        }
        if (url.includes("/worklist")) {
          if (done) {
            readAfterTheWrite += 1;
          }
          return jsonResponse(done ? after : before);
        }
        return jsonResponse({ data: [] });
      }),
    );
    const user = userEvent.setup();
    renderUnderAToastRegion();

    // The figure the day starts on, so the assertion below is a CHANGE rather
    // than a number that happened to be right from the first render.
    await screen.findByText(/2 in all/);
    await user.click(
      (await screen.findAllByRole("button", { name: en["tasks.complete"] }))[0],
    );

    await waitFor(() => {
      expect(screen.queryByText("Send the retrofit quote")).toBeNull();
    });
    // The row left AND the headline followed it. Asserting only the first is
    // what lets a stale count ship: the finished row disappears optimistically
    // while the sentence above it still says two are waiting.
    await waitFor(() => {
      expect(screen.getByText(/1 in all/)).toBeTruthy();
    });
    expect(screen.queryByText(/2 in all/)).toBeNull();
    // And the day still holds the work that was not finished, so the count fell
    // because one row left rather than because the queue emptied itself.
    expect(screen.getByText("Call the buyer back")).toBeTruthy();
    // The figure came from the SERVER, not from arithmetic on the client.
    // Without this the test passes for a client that decremented its own copy
    // and never asked again — which is right for this fixture and wrong the
    // moment the write changes anything the client did not predict, such as a
    // second row leaving with it or a band the row moves between.
    expect(readAfterTheWrite).toBeGreaterThan(0);
  });
});

// patchedTask is the LAST task PATCH this render sent, read off the fetch stub.
//
// The body matters as much as the address: `is_done: true` and `is_done: false`
// go to one endpoint, so a test that only counted requests could not tell
// completing from reopening.
async function patchedTask(): Promise<
  { id: string; body: unknown } | undefined
> {
  const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
  for (let at = calls.length - 1; at >= 0; at--) {
    // The client sends a Request object, so the method and the body ride on it
    // rather than on a second init argument — reading init here found nothing
    // and the assertion compared undefined against the write it wanted.
    const [input] = calls[at] as [RequestInfo | URL, RequestInit?];
    if (!(input instanceof Request) || input.method !== "PATCH") {
      continue;
    }
    const match = /\/activities\/([^/?]+)$/.exec(input.url.split("?")[0]);
    if (match) {
      return { id: match[1], body: await input.clone().json() };
    }
  }
  return undefined;
}

// A failed undo must SAY so, and the row leaving the queue is what makes that
// hard: the completion's refetch drops the row, the component unmounts, and a
// per-call onError passed to `mutate` goes with it. The reader then presses
// Undo, the PATCH is refused, and nothing appears at all.
describe("an undo that is refused", () => {
  it("says so, even though the row it belonged to is gone", async () => {
    let completed = false;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const request = input instanceof Request ? input : undefined;
        const url = String(request ? request.url : input);
        if (request?.method === "PATCH") {
          const body = (await request.clone().json()) as { is_done: boolean };
          if (body.is_done) {
            // The completion succeeds, and the row LEAVES the day — which is
            // the state a static fixture never reaches and the one the failure
            // below depends on.
            completed = true;
            return new Response(null, { status: 204 });
          }
          // The undo is refused, the way a permission change between two
          // writes refuses it.
          return new Response(
            JSON.stringify({ title: "Forbidden", status: 403 }),
            {
              status: 403,
              headers: { "content-type": "application/problem+json" },
            },
          );
        }
        if (/\/worklist/.test(url)) {
          return new Response(
            JSON.stringify(
              day({
                queue: completed
                  ? []
                  : [
                      row({
                        id: "task-1",
                        source: "task",
                        title: "Send the retrofit quote",
                        actions: ["complete"],
                        primary_action: "complete",
                      }),
                    ],
                summary: completed
                  ? { urgent: 0, due: 0, lower_priority: 0, total: 0 }
                  : { urgent: 0, due: 0, lower_priority: 1, total: 1 },
              }),
            ),
            { status: 200, headers: { "content-type": "application/json" } },
          );
        }
        return new Response(JSON.stringify({ data: [] }), {
          status: 200,
          headers: { "content-type": "application/json" },
        });
      }),
    );
    const user = userEvent.setup();
    renderUnderAToastRegion();

    await user.click(
      await screen.findByRole("button", { name: en["tasks.complete"] }),
    );
    await user.click(
      await screen.findByRole("button", {
        name: en["worklist.verb.completeUndo"],
      }),
    );

    await waitFor(() => {
      expect(
        screen.getByText(en["worklist.verb.completeUndoFailed"]),
      ).toBeTruthy();
    });
  });
});

// renderUnderAToastRegion draws the screen the way the shell draws it.
//
// The confirmation this test presses lives in a toast, and `renderWorklist`
// mounts no region — deliberately: the testkit is production-shaped for the
// conformance gate, which allows exactly one ToastProvider and one ToastRegion
// in the tree and names main.tsx as their home. So the region is mounted HERE,
// in a test file the gate does not scan, the same way archive.test.tsx does it.
//
// Without it `toast.show` renders nothing, the Undo button never appears, and a
// test that asserted only the completion would pass over a verb offering no way
// back.
function renderUnderAToastRegion() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <WorklistScreen />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}
