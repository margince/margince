// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Acting on several rows at once, from the Worklist, in selection mode.
//
// After Select, a row takes a checkbox when it offers Done. That is a task or
// a promise to complete, or a row whose Done takes it off the reader's own
// list. The two verbs stay apart. An approval is answered one at a time.

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  render,
  renderHook,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { day, jsonResponse, row, stub } from "../../testing/worklist.testkit";
import { WorklistScreen } from "./worklist";
import { useWorklistPicks } from "./worklist.bulkdone";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const aTask = row({
  id: "01a05500-0000-7000-8000-0000000000a1",
  source: "task",
  title: "Send the retrofit quote",
  actions: ["complete"],
  version: 3,
});
const aPromise = row({
  id: "01a05500-0000-7000-8000-0000000000a2",
  source: "conversation_claim",
  category: "customer_waiting",
  title: "Confirm the meeting by Friday",
  actions: ["complete", "open"],
  version: 2,
});
const aWaitingMessage = row({
  id: "01a05500-0000-7000-8000-0000000000a3",
  source: "customer_waiting",
  category: "customer_waiting",
  title: "Question about the invoice",
  actions: ["open"],
});
const anApproval = row({
  id: "01a05500-0000-7000-8000-0000000000a4",
  source: "approval",
  category: "decisions",
  title: "Send the follow-up",
  actions: ["decide"],
});
const aSystemRow = row({
  id: "01a05500-0000-7000-8000-0000000000a5",
  source: "task",
  category: "system",
  title: "Reconnect the mailbox",
  actions: ["complete"],
});

const aDealAtRisk = row({
  id: "01a05500-0000-7000-8000-0000000000a6",
  source: "deal_at_risk",
  category: "deals_at_risk",
  title: "Retrofit for the north plant",
  actions: ["open"],
  dispositions: ["handled"],
});

function theDay() {
  return day({
    queue: [aTask, aPromise, aWaitingMessage, anApproval, aSystemRow],
    summary: { urgent: 0, due: 0, lower_priority: 5, total: 5 },
  });
}

/** Enters selection mode, which is what draws the checkboxes. */
async function startSelecting(user = userEvent.setup()) {
  await user.click(
    await screen.findByRole("button", { name: en["worklist.bulk.select"] }),
  );
  return user;
}

function selectBox(title: string) {
  return screen.queryByRole("checkbox", {
    name: en["bulk.selectRow"].replace("{name}", title),
  });
}

describe("ticking Worklist rows in selection mode", () => {
  it("draws no checkbox until the reader presses Select", async () => {
    stub(theDay());
    renderUnderAToastRegion();

    await screen.findByText("Question about the invoice");
    expect(selectBox("Send the retrofit quote")).toBeNull();
    await startSelecting();
    expect(selectBox("Send the retrofit quote")).not.toBeNull();
  });

  it("offers a checkbox on task and promise rows only", async () => {
    stub(theDay());
    renderUnderAToastRegion();

    await screen.findByText("Question about the invoice");
    await startSelecting();
    expect(selectBox("Send the retrofit quote")).not.toBeNull();
    expect(selectBox("Confirm the meeting by Friday")).not.toBeNull();
    expect(selectBox("Question about the invoice")).toBeNull();
    expect(selectBox("Send the follow-up")).toBeNull();
    expect(selectBox("Reconnect the mailbox")).toBeNull();
  });

  it("ticks one row, not every row that shares its record's id", async () => {
    const twin = row({
      ...aPromise,
      id: aTask.id,
      title: "Promised the same thing",
    });
    stub(
      day({
        queue: [aTask, twin],
        summary: { urgent: 0, due: 0, lower_priority: 2, total: 2 },
      }),
    );
    renderUnderAToastRegion();
    const user = await startSelecting();

    await user.click(
      (
        await screen.findAllByRole("checkbox", {
          name: en["bulk.selectRow"].replace(
            "{name}",
            "Send the retrofit quote",
          ),
        })
      )[0],
    );

    expect(
      (selectBox("Send the retrofit quote") as HTMLInputElement).checked,
    ).toBe(true);
    expect(
      (selectBox("Promised the same thing") as HTMLInputElement).checked,
    ).toBe(false);
  });

  it("drops the selection when the reader changes the filter", () => {
    const { result, rerender } = renderHook(
      ({ view }) => useWorklistPicks([aTask, aPromise], view),
      { initialProps: { view: "mine/all/" } },
    );
    act(() => result.current.selectAll());
    expect(result.current.rows).toHaveLength(2);

    rerender({ view: "mine/tasks/" });
    expect(result.current.rows).toHaveLength(0);

    rerender({ view: "mine/all/" });
    expect(result.current.rows).toHaveLength(0);
  });

  it("keeps Select all to the rows it saw when the next batch arrives", () => {
    const later = row({
      id: "01a05500-0000-7000-8000-0000000000b1",
      source: "task",
      title: "Arrived with the next batch",
      actions: ["complete"],
    });
    const { result, rerender } = renderHook(
      ({ queue }) => useWorklistPicks(queue, "mine/all/"),
      { initialProps: { queue: [aTask, aWaitingMessage, aPromise] } },
    );
    act(() => result.current.selectAll());
    expect(result.current.rows).toEqual([aTask, aPromise]);

    rerender({ queue: [aTask, aWaitingMessage, aPromise, later] });
    expect(result.current.rows).toEqual([aTask, aPromise]);
  });

  it("selects every shown task and promise and previews them as one change", async () => {
    const previewed: unknown[] = [];
    stub(theDay());
    const passthrough = globalThis.fetch;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.endsWith("/bulk/preview")) {
          const body =
            input instanceof Request
              ? await input.clone().text()
              : String(init?.body ?? "");
          previewed.push(JSON.parse(body));
          return jsonResponse({
            record_type: "worklist_item",
            verb: "complete",
            count: 2,
            affected: [aTask.id, aPromise.id],
            excluded: [],
            sample: [],
            requires_confirmation: false,
          });
        }
        return passthrough(input, init);
      }),
    );
    renderUnderAToastRegion();
    const user = await startSelecting();

    await user.click(
      await screen.findByRole("button", {
        name: en["worklist.bulk.selectAll_other"].replace("{count}", "2"),
      }),
    );
    await user.click(
      screen.getByRole("button", {
        name: en["worklist.bulk.complete_other"].replace("{count}", "2"),
      }),
    );

    await waitFor(() => {
      expect(previewed).toEqual([
        {
          record_type: "worklist_item",
          verb: "complete",
          items: [
            { id: aTask.id, version: 3 },
            { id: aPromise.id, version: 2 },
          ],
        },
      ]);
    });
  });
});

describe("marking the ticked rows done", () => {
  it("shows what changes and what is left, runs it, and clears the ticks", async () => {
    const executed: unknown[] = [];
    stub(theDay());
    const passthrough = globalThis.fetch;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.endsWith("/bulk/preview")) {
          return jsonResponse({
            record_type: "worklist_item",
            verb: "complete",
            count: 1,
            affected: [aTask.id],
            excluded: [{ id: aPromise.id, reason: "no_change" }],
            sample: [
              {
                id: aTask.id,
                label: "Send the retrofit quote",
                before: { owner_id: null, archived: false, done: false },
                after: { owner_id: null, archived: false, done: true },
              },
            ],
            requires_confirmation: false,
            confirm_token: "token-1",
          });
        }
        if (url.endsWith("/bulk/execute")) {
          executed.push(url);
          return jsonResponse({
            batch_id: "01a05500-0000-7000-8000-0000000000b1",
            changed: 1,
            skipped: [{ id: aPromise.id, reason: "no_change" }],
          });
        }
        return passthrough(input, init);
      }),
    );
    renderUnderAToastRegion();
    const user = await startSelecting();

    await user.click(
      await screen.findByRole("button", {
        name: en["worklist.bulk.selectAll_other"].replace("{count}", "2"),
      }),
    );
    await user.click(
      screen.getByRole("button", {
        name: en["worklist.bulk.complete_other"].replace("{count}", "2"),
      }),
    );

    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).getByText(
        en["bulk.titleComplete"].replace("{unit}", en["unit.worklistItems"]),
      ),
    ).not.toBeNull();
    expect(within(dialog).getByText(en["bulk.stateOpen"])).not.toBeNull();
    expect(within(dialog).getByText(en["bulk.stateDone"])).not.toBeNull();
    expect(
      within(dialog).getByText(en["bulk.reason.no_change_done"]),
    ).not.toBeNull();

    await user.click(
      within(dialog).getByRole("button", { name: en["bulk.confirmComplete"] }),
    );

    await screen.findByText(
      new RegExp(en["bulk.doneWorklistItems_one"].replace("{count}", "1")),
    );
    expect(executed).toHaveLength(1);
    // The change ends selection mode, so no box stays ticked out of sight.
    expect(selectBox("Send the retrofit quote")).toBeNull();
    expect(selectBox("Confirm the meeting by Friday")).toBeNull();
  });
});

describe("completing part of a mixed selection", () => {
  it("unticks only the completed rows and stays in selection mode", async () => {
    stub(
      day({
        queue: [aTask, aDealAtRisk],
        summary: { urgent: 0, due: 0, lower_priority: 2, total: 2 },
      }),
    );
    const passthrough = globalThis.fetch;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.endsWith("/bulk/preview")) {
          return jsonResponse({
            record_type: "worklist_item",
            verb: "complete",
            count: 1,
            affected: [aTask.id],
            excluded: [],
            sample: [],
            requires_confirmation: false,
            confirm_token: "token-1",
          });
        }
        if (url.endsWith("/bulk/execute")) {
          return jsonResponse({
            batch_id: "01a05500-0000-7000-8000-0000000000b2",
            changed: 1,
            skipped: [],
          });
        }
        return passthrough(input, init);
      }),
    );
    renderUnderAToastRegion();
    const user = await startSelecting();
    await user.click(
      await screen.findByRole("button", {
        name: en["worklist.bulk.selectAll_other"].replace("{count}", "2"),
      }),
    );
    await user.click(
      screen.getByRole("button", {
        name: en["worklist.bulk.complete_one"].replace("{count}", "1"),
      }),
    );
    const dialog = await screen.findByRole("dialog");
    await user.click(
      within(dialog).getByRole("button", { name: en["bulk.confirmComplete"] }),
    );

    await waitFor(() => {
      expect(
        (selectBox("Send the retrofit quote") as HTMLInputElement).checked,
      ).toBe(false);
    });
    expect(
      (selectBox("Retrofit for the north plant") as HTMLInputElement).checked,
    ).toBe(true);
  });
});

describe("leaving selection mode", () => {
  it("gives focus back to Select when Cancel closes the mode", async () => {
    stub(theDay());
    renderUnderAToastRegion();
    const user = await startSelecting();

    await user.click(
      screen.getByRole("button", { name: en["worklist.bulk.cancel"] }),
    );

    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: en["worklist.bulk.select"] }),
    );
  });
});

describe("a queue that mixes rows that can be ticked with rows that cannot", () => {
  it("keeps a slot where a row has no box, so the ranks line up", async () => {
    stub(theDay());
    const { container } = renderUnderAToastRegion();

    await screen.findByText("Question about the invoice");
    await startSelecting();
    // Three rows take no box: the waiting message, the approval and the
    // system row.
    expect(container.querySelectorAll(".worklist-row-pick-slot")).toHaveLength(
      3,
    );
  });

  it("keeps no slot when no row on screen can be ticked", async () => {
    stub(
      day({
        queue: [aWaitingMessage],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );
    const { container } = renderUnderAToastRegion();

    await screen.findByText("Question about the invoice");
    expect(container.querySelectorAll(".worklist-row-pick-slot")).toHaveLength(
      0,
    );
  });
});

describe("the two verbs for ticked rows", () => {
  it("completes the tasks and takes the other rows off the list, each by its own verb", async () => {
    const sent: { path: string; body: unknown }[] = [];
    stub(
      day({
        queue: [aTask, aDealAtRisk, anApproval],
        summary: { urgent: 0, due: 0, lower_priority: 3, total: 3 },
      }),
    );
    const passthrough = globalThis.fetch;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = new URL(
          String(input instanceof Request ? input.url : input),
        );
        const done = url.pathname.endsWith("/worklist/done");
        if (done || url.pathname.endsWith("/bulk/preview")) {
          const body =
            input instanceof Request
              ? await input.clone().text()
              : String(init?.body ?? "");
          sent.push({ path: url.pathname, body: JSON.parse(body) });
          return done
            ? new Response(null, { status: 204 })
            : jsonResponse({
                record_type: "worklist_item",
                verb: "complete",
                count: 1,
                affected: [aTask.id],
                excluded: [],
                sample: [],
                requires_confirmation: false,
              });
        }
        return passthrough(input, init);
      }),
    );
    renderUnderAToastRegion();

    await screen.findByText("Retrofit for the north plant");
    const user = await startSelecting();
    expect(selectBox("Send the follow-up")).toBeNull();
    await user.click(
      await screen.findByRole("button", {
        name: en["worklist.bulk.selectAll_other"].replace("{count}", "2"),
      }),
    );
    await user.click(
      screen.getByRole("button", {
        name: en["worklist.bulk.putDown_one"].replace("{count}", "1"),
      }),
    );
    await user.click(
      screen.getByRole("button", {
        name: en["worklist.bulk.complete_one"].replace("{count}", "1"),
      }),
    );

    await waitFor(() => {
      // Two writes, in no promised order: the dialog previews while the
      // other rows leave.
      expect([...sent].sort((a, b) => (a.path < b.path ? -1 : 1))).toEqual([
        {
          path: "/v1/bulk/preview",
          body: {
            record_type: "worklist_item",
            verb: "complete",
            items: [{ id: aTask.id, version: 3 }],
          },
        },
        {
          path: "/v1/worklist/done",
          body: { rows: [{ source: "deal_at_risk", row_id: aDealAtRisk.id }] },
        },
      ]);
    });
  });
});

// The toast region is mounted here for the reason worklist.taskdone.test.tsx
// gives: the app mounts the one region in main.tsx, which a screen test does
// not render.
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
