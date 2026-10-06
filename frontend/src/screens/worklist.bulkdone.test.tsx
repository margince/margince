// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Marking several tasks and promises done at once, from the Worklist.
//
// Only a row whose own Done marks a task or a promise done takes a checkbox. A
// waiting message, an approval and a system row are answered one at a time, so
// a box on them would offer a verb the change refuses.

/** @vitest-environment happy-dom */
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

function theDay() {
  return day({
    queue: [aTask, aPromise, aWaitingMessage, anApproval, aSystemRow],
    summary: { urgent: 0, due: 0, lower_priority: 5, total: 5 },
  });
}

function selectBox(title: string) {
  return screen.queryByRole("checkbox", {
    name: en["bulk.selectRow"].replace("{name}", title),
  });
}

describe("ticking Worklist rows for Mark done", () => {
  it("offers a checkbox on task and promise rows only", async () => {
    stub(theDay());
    renderUnderAToastRegion();

    await screen.findByText("Question about the invoice");
    expect(selectBox("Send the retrofit quote")).not.toBeNull();
    expect(selectBox("Confirm the meeting by Friday")).not.toBeNull();
    expect(selectBox("Question about the invoice")).toBeNull();
    expect(selectBox("Send the follow-up")).toBeNull();
    expect(selectBox("Reconnect the mailbox")).toBeNull();
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
    const user = userEvent.setup();
    renderUnderAToastRegion();

    await user.click(
      await screen.findByRole("button", {
        name: en["worklist.bulk.selectAll_other"].replace("{count}", "2"),
      }),
    );
    await user.click(
      screen.getByRole("button", { name: en["worklist.bulk.markDone"] }),
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
