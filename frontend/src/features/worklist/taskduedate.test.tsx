// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Moving a task to a day the reader names.
//
// The snooze beside this one adds a day to the task's OWN due date, so it
// cannot reach a named day in principle rather than merely in clicks: a task
// three days overdue snoozes to two days overdue, and a rep who means tomorrow
// presses it four times.

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TaskQuickActions, useTaskUpdate } from "../../components/taskactions";
import { LocaleProvider } from "../../i18n";
import { RecordZoneProvider } from "../../lib/recordzone";

const TASK = "01a05500-0000-7000-8000-000000000a01";

// The installation this record belongs to. Named rather than read off the
// runner, because the rule under test is that a deadline reads as the SAME day
// for every colleague — a suite that took the machine's own zone would assert
// nothing when the machine happened to sit in it.
const RECORD_ZONE = "Europe/Berlin";

// The task's due instant, minted the way the product mints one: the end of a
// calendar day on the RECORD's clock. Berlin is two hours ahead of UTC in
// September, so this is 21:59:59Z — a value that names 1 September in Berlin
// and 2 September in Bangkok, which is exactly the disagreement the record-zone
// rule exists to settle.
const DUE_AT = "2026-09-01T21:59:59.000Z";

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
      <RecordZoneProvider zone={RECORD_ZONE}>
        <LocaleProvider initial="en">
          <Harness />
        </LocaleProvider>
      </RecordZoneProvider>
    </QueryClientProvider>,
  );
}

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

const picker = () =>
  screen.getByRole<HTMLInputElement>("textbox", { name: "Move to" });

describe("moving a task to a named day", () => {
  it("sends the day picked on the calendar as the task's due date", async () => {
    const user = userEvent.setup();
    renderVerbs(DUE_AT);
    await user.click(screen.getByRole("button", { name: /^Choose date/ }));
    await user.click(
      await screen.findByRole("button", { name: "Tuesday, 15 September 2026" }),
    );

    await waitFor(() => expect(sent).toHaveLength(1));
    // The END of the picked day on the RECORD's clock, so a task due "the
    // 15th" is not overdue at nine that morning and still reads as the 15th to
    // a colleague in another zone. dueInstant owns that rule; this asserts the
    // row passes it the record zone rather than the browser's.
    const body = sent[0] as { due_at: string };
    expect(body.due_at).toBe("2026-09-15T21:59:59.000Z");
  });

  it("sends a day typed in the reader's pattern the same way", async () => {
    const user = userEvent.setup();
    renderVerbs(DUE_AT);
    await user.tripleClick(picker());
    await user.keyboard("15/09/2026");
    expect(sent).toHaveLength(0);
    await user.keyboard("{Enter}");

    await waitFor(() => expect(sent).toHaveLength(1));
    const body = sent[0] as { due_at: string };
    expect(body.due_at).toBe("2026-09-15T21:59:59.000Z");
  });

  it("opens on the day the task is already due", async () => {
    // A picker opening on a different day than the meta line beside it reads
    // as two answers to "when is this due".
    const user = userEvent.setup();
    renderVerbs(DUE_AT);
    expect(picker().value).toBe("01/09/2026");

    await user.click(screen.getByRole("button", { name: /^Choose date/ }));
    const chosen = await screen.findByRole("gridcell", { selected: true });
    expect(
      within(chosen).getByRole("button", { name: "Tuesday, 1 September 2026" }),
    ).toBeTruthy();
  });

  it("is offered on an undated task, where the snooze is not", async () => {
    // The one a rep most wants to date, and the snooze is hidden for it —
    // one day after nothing is nothing.
    renderVerbs(null);
    expect(picker().value).toBe("");
    expect(screen.queryByRole("button", { name: /snooze/i })).toBeNull();
  });

  it("is off unless the caller asks for it", () => {
    // The default is the point: this component is a per-row action slot on the
    // 360 next-steps lists, where a date box in every row is heavier than the
    // surface intends. The detail modal opts in.
    // The prop is OMITTED, not passed false: the default is what the 360 rows
    // get, and a test that spelled it out would pass over a default flipped on.
    renderDefaults(DUE_AT);
    expect(
      screen.queryByRole("textbox", { name: "Move to" }),
      "a caller that did not ask for the picker was given one",
    ).toBeNull();
  });
});
