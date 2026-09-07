// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Moving a task to a day the reader names.
//
// The snooze beside this one adds a day to the task's OWN due date, so it
// cannot reach a named day in principle rather than merely in clicks: a task
// three days overdue snoozes to two days overdue, and a rep who means tomorrow
// presses it four times.

/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { TaskQuickActions, useTaskUpdate } from "./taskactions";

const TASK = "01a05500-0000-7000-8000-000000000a01";

// The task's due instant, minted the way the product mints one: the end of a
// calendar day in the RUNNING machine's zone. A fixed UTC string would name a
// different calendar day depending on where the suite runs — 23:59:59Z on the
// 1st is already the 2nd anywhere east of UTC — and the picker reads the day in
// the viewer's zone, correctly, so the fixture has to speak the same clock.
const DUE_DAY = "2026-09-01";
const DUE_AT = new Date(`${DUE_DAY}T23:59:59`).toISOString();

let sent: unknown[] = [];

beforeEach(() => {
  sent = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input instanceof Request ? input.url : input);
      if (url.includes("/activities/")) {
        const body =
          input instanceof Request
            ? await input.clone().text()
            : String(init?.body ?? "");
        sent.push(JSON.parse(body));
      }
      return new Response(JSON.stringify({}), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }),
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The version the row was drawn at, which every write on it is pinned to.
const TASK_VERSION = 3;

function renderVerbs(dueAt?: string | null) {
  function Harness() {
    const update = useTaskUpdate([["tasks"]]);
    return (
      <TaskQuickActions
        activityId={TASK}
        version={TASK_VERSION}
        dueAt={dueAt}
        update={update}
        showDuePicker
      />
    );
  }
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <Harness />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// The picker is a native type="date" input, which exposes no role of its own.
// The same harness with the picker prop left out entirely.
function renderDefaults(dueAt?: string | null) {
  function Harness() {
    const update = useTaskUpdate([["tasks"]]);
    return (
      <TaskQuickActions
        activityId={TASK}
        version={TASK_VERSION}
        dueAt={dueAt}
        update={update}
      />
    );
  }
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <Harness />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

function picker(container: HTMLElement): HTMLInputElement {
  const found = container.querySelector<HTMLInputElement>('input[type="date"]');
  if (!found) {
    throw new Error("no date picker rendered");
  }
  return found;
}

describe("moving a task to a named day", () => {
  it("sends the picked day as the task's due date", async () => {
    const { container } = renderVerbs(DUE_AT);
    fireEvent.change(picker(container), { target: { value: "2026-09-15" } });

    await waitFor(() => expect(sent).toHaveLength(1));
    // The END of the picked day, so a task due "the 15th" is not overdue at
    // nine that morning. dueInstant owns that rule; this asserts the row uses
    // it rather than sending midnight.
    const body = sent[0] as { due_at: string };
    expect(new Date(body.due_at).getTime()).toBe(
      new Date("2026-09-15T23:59:59").getTime(),
    );
  });

  it("opens on the day the task is already due", async () => {
    // A picker opening on a different day than the meta line beside it reads
    // as two answers to "when is this due".
    const { container } = renderVerbs(DUE_AT);
    expect(picker(container).value).toBe(DUE_DAY);
  });

  it("is offered on an undated task, where the snooze is not", async () => {
    // The one a rep most wants to date, and the snooze is hidden for it —
    // one day after nothing is nothing.
    const { container } = renderVerbs(null);
    expect(picker(container)).not.toBeNull();
    expect(screen.queryByRole("button", { name: /snooze/i })).toBeNull();
  });

  it("is off unless the caller asks for it", () => {
    // The default is the point: this component is a per-row action slot on the
    // 360 next-steps lists, where a date box in every row is heavier than the
    // surface intends. The detail modal opts in.
    // The prop is OMITTED, not passed false: the default is what the 360 rows
    // get, and a test that spelled it out would pass over a default flipped on.
    const { container } = renderDefaults(DUE_AT);
    expect(
      container.querySelector('input[type="date"]'),
      "a caller that did not ask for the picker was given one",
    ).toBeNull();
  });
});
