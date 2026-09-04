// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import {
  companyBackstop,
  emptyPage,
  emptySection,
  jsonResponse,
  org,
  org360,
  stubFetch,
} from "./company.fixtures";
import { CompanyScreen } from "./organizations";

// The company record's Tasks tab: tick-to-complete without leaving the
// account, a withheld section that says so, an archived account that offers no
// write it cannot honour, and the detail modal's id surviving no longer than
// the tab that renders it.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  // Handed back per case, so one test pretending to be elsewhere never decides
  // what the next one reads.
  vi.restoreAllMocks();
  window.location.hash = "";
});

// The viewer's zone as a screen asks for it. The formatters take their zone as
// an argument and never consult resolvedOptions, so this redirects only a
// screen's own lookup — which is what a test of the zone CHOICE needs.
function pretendViewerZone(timeZone: string): void {
  const real = Intl.DateTimeFormat().resolvedOptions();
  vi.spyOn(Intl.DateTimeFormat.prototype, "resolvedOptions").mockReturnValue({
    ...real,
    timeZone,
  });
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

// One open, dated task, as the 360 serves it on the Tasks tab.
const openTask = {
  activity_id: "a-1",
  subject: "Follow up on contract renewal",
  due_at: "2026-08-20T00:00:00Z",
  overdue: false,
  assignee_id: null,
  linked_deal_id: null,
  linked_person_id: null,
};

// The same task as the ACTIVITY read the detail modal fires when a row is
// expanded — the composite's summary shape carries no version or done flag, so
// the modal reads the record itself.
const openTaskActivity = {
  id: openTask.activity_id,
  organization_id: "o-1",
  type: "task",
  subject: openTask.subject,
  occurred_at: "2026-08-01T09:00:00Z",
  due_at: openTask.due_at,
  is_done: false,
  captured_by: "human:u1",
  source: "manual",
  version: 1,
};

const org360WithOpenTask = {
  ...org360,
  next_steps: { ...emptySection, data: [openTask] },
};

// The step nobody has accepted yet: the account has an open deal and nothing
// scheduled, so the rule worked out what to do and prepared the body that
// writes it. The tab draws it beside the open work rather than leaving it in
// the day's brief, where a reader who came looking for what happens next
// would never meet it.
const RECOMMENDED = 'Agree the next step on "PIM rollout Phase 2"';
const preparedStep = {
  subject: RECOMMENDED,
  source: "ui",
  links: [{ entity_type: "deal", entity_id: "d-1" }],
};
const org360WithRecommendedStep = {
  ...org360,
  suggestions: [
    {
      kind: "no_next_step",
      fingerprint: "f1",
      title: RECOMMENDED,
      reason:
        '"PIM rollout Phase 2" is open and no task says what happens next.',
      evidence: [{ entity_type: "deal", entity_id: "d-1" }],
      action: { kind: "add_task", deal_id: "d-1", task: preparedStep },
    },
  ],
  suggestions_dropped: 0,
};

// A due date that falls on a DIFFERENT calendar day in the two zones this page
// could read it in. `dueInstant` mints a due date as the end of the picked day
// in the BROWSER's zone, so this is what a writer in Los Angeles picking 21
// August actually stores: 23:59:59 there, and already the 22nd in Berlin.
const STRADDLING_DUE_AT = "2026-08-22T06:59:59Z";
const WRITERS_ZONE = "America/Los_Angeles";
const PICKED_DAY = "Due 21/08/2026";
const DAY_AFTER = "Due 22/08/2026";

const org360WithStraddlingTask = {
  ...org360,
  next_steps: {
    ...emptySection,
    data: [{ ...openTask, due_at: STRADDLING_DUE_AT }],
  },
};

// The section the reader's role cannot read: absent from the payload and named
// in `sections_omitted`, which is a different fact from an empty one.
const org360WithheldTasks = {
  ...org360,
  next_steps: undefined,
  sections_omitted: ["next_steps"],
};

// Driven through the reader's own instance: a second implicit one forgets
// which keys and buttons the first left held.
// The tab button's accessible name carries a live count ("Tasks 3"), so a
// prefix match is the only one that survives the account having any tasks at
// all.
async function openTasksTab(user: UserEvent) {
  await user.click(await screen.findByRole("button", { name: /^Tasks/ }));
}

describe("CompanyScreen — the Tasks tab", () => {
  it("completes a task without leaving the account", async () => {
    const user = userEvent.setup();
    let patched: unknown;
    stubFetch(
      async (url, method, request) => {
        if (method === "PATCH" && url.endsWith("/activities/a-1")) {
          patched = await request.json();
          return jsonResponse({});
        }
        return companyBackstop(url);
      },
      { org360: org360WithOpenTask },
    );
    render(<CompanyScreen id="o-1" />);
    await openTasksTab(user);

    await waitFor(() =>
      expect(screen.getByText(openTask.subject)).toBeTruthy(),
    );
    await user.click(screen.getByRole("checkbox", { name: "Done" }));

    await waitFor(() => expect(patched).toEqual({ is_done: true }));
  });

  it("dates a task's deadline on the reader's own clock, the same day on the row and in the detail", async () => {
    // A due date is not a fact about the record the way an occurrence is: the
    // stored instant is minted as the end of the picked day in the BROWSER's
    // zone, so it already carries the picker's clock. Read on the
    // organization's clock it names the day AFTER the one the picker chose for
    // everybody outside that zone — and the row and the modal disagreed with
    // each other about which of the two it was.
    const user = userEvent.setup();
    pretendViewerZone(WRITERS_ZONE);
    stubFetch(
      async (url) => {
        if (url.endsWith("/activities/a-1")) {
          return jsonResponse({
            ...openTaskActivity,
            due_at: STRADDLING_DUE_AT,
          });
        }
        return companyBackstop(url);
      },
      { org360: org360WithStraddlingTask },
    );
    render(<CompanyScreen id="o-1" />);
    await openTasksTab(user);

    expect(await screen.findByText(PICKED_DAY)).toBeTruthy();
    await user.click(screen.getByText(openTask.subject));
    await waitFor(() =>
      expect(
        screen.getByRole("dialog", { name: openTask.subject }),
      ).toBeTruthy(),
    );
    // Row and detail, both open: one deadline, one day.
    expect(await screen.findAllByText(PICKED_DAY)).toHaveLength(2);
    expect(screen.queryByText(DAY_AFTER)).toBeNull();
  });

  // The button used to do nothing at all: the row said "Add the next step" and
  // the page had no surface to route it to, so a reader pressed it and the
  // account stayed exactly as it was. The step is the server's own sentence
  // now, and the button writes THAT — not one this page recomposed from the
  // row's words, which is how the sentence a reader accepted and the task they
  // get come apart.
  it("writes the recommended step, exactly as the server prepared it", async () => {
    const user = userEvent.setup();
    let posted: unknown;
    const { urls } = stubFetch(
      async (url, method, request) => {
        if (method === "POST" && url.endsWith("/tasks")) {
          posted = await request.json();
          return jsonResponse({ id: "a-9" }, 201);
        }
        return companyBackstop(url);
      },
      { org360: org360WithRecommendedStep },
    );
    render(<CompanyScreen id="o-1" />);
    await openTasksTab(user);

    // The ask is the step, in the words the button writes — not "Set the next
    // step", which hands the reader back the finding as an instruction.
    await waitFor(() => expect(screen.getByText(RECOMMENDED)).toBeTruthy());
    await user.click(screen.getByRole("button", { name: "Add the next step" }));

    await waitFor(() => expect(posted).toEqual(preparedStep));
    // The POST returning is not the end of the write: it invalidates the
    // account's own read, because the advice fired on there being no open task
    // and there is one now. Waiting for that re-read is what stops this test
    // finishing with a render still queued behind it — which, in the full
    // suite, lands in whichever file jsdom has already torn down.
    await waitFor(() =>
      expect(urls.filter((url) => url.endsWith("/360")).length).toBeGreaterThan(
        1,
      ),
    );
  });

  it("recommends no step on an archived account", async () => {
    // The write would only be refused, and a button that cannot fire is the
    // control-that-does-nothing this surface exists to avoid.
    const user = userEvent.setup();
    stubFetch(
      async (url) => {
        if (url.endsWith("/organizations/o-1")) {
          return jsonResponse({ ...org, archived_at: "2026-07-13T00:00:00Z" });
        }
        return companyBackstop(url);
      },
      { org360: org360WithRecommendedStep },
    );
    render(<CompanyScreen id="o-1" />);
    await openTasksTab(user);

    await waitFor(() =>
      expect(screen.getByText("No open task on this account.")).toBeTruthy(),
    );
    expect(screen.queryByText(RECOMMENDED)).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Add the next step" }),
    ).toBeNull();
  });

  it("says the section is withheld rather than rendering it as empty", async () => {
    const user = userEvent.setup();
    stubFetch(companyBackstop, { org360: org360WithheldTasks });
    render(<CompanyScreen id="o-1" />);
    await openTasksTab(user);

    // Scoped to the tab's own panel: the account facts strip says the same
    // sentence about a withheld deal or project count, and a page-wide match
    // would pass on either of those while this tab drew nothing at all.
    const heading = await screen.findByRole("heading", { name: "Tasks" });
    const tasks = heading.closest("section");
    if (!tasks) {
      throw new Error("the tasks tab has no section wrapper");
    }
    await waitFor(() =>
      expect(
        within(tasks).getByText("Hidden — your role cannot read this"),
      ).toBeTruthy(),
    );
    expect(
      within(tasks).queryByText("No open task on this account."),
    ).toBeNull();
  });

  it("shows no task-completing verb on an archived account", async () => {
    const user = userEvent.setup();
    // The server refuses a write on an archived account, so the tab omits the
    // verb rather than offering a button that can only 404.
    stubFetch(
      async (url) => {
        if (url.endsWith("/organizations/o-1")) {
          return jsonResponse({ ...org, archived_at: "2026-07-13T00:00:00Z" });
        }
        if (url.endsWith("/activities/a-1")) {
          return jsonResponse(openTaskActivity);
        }
        return emptyPage();
      },
      { org360: org360WithOpenTask },
    );
    render(<CompanyScreen id="o-1" />);
    await openTasksTab(user);

    await waitFor(() =>
      expect(screen.getByText(openTask.subject)).toBeTruthy(),
    );
    expect(screen.queryByRole("checkbox", { name: "Done" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Snooze 1d" })).toBeNull();

    // The same withheld verb holds inside the detail modal, not only the row.
    await user.click(screen.getByText(openTask.subject));
    await waitFor(() =>
      expect(
        screen.getByRole("dialog", { name: openTask.subject }),
      ).toBeTruthy(),
    );
    expect(screen.queryByRole("button", { name: "Done" })).toBeNull();
  });

  // The detail modal renders on this tab and nowhere else, so a tab change
  // takes it off screen without its own onClose ever running. An open id that
  // survived that would be waiting the next time the reader came back to
  // Tasks — a dialog reopening itself, having been closed by nobody.
  //
  // Driven through the tab pill on purpose. A reader cannot reach the pill
  // while the dialog covers it, and that is a fact about Modal's backdrop
  // rather than about this page: the id must not depend on it.
  it("opens no dialog when the reader returns to Tasks after changing tab", async () => {
    const user = userEvent.setup();
    stubFetch(
      async (url) => {
        if (url.endsWith("/activities/a-1")) {
          return jsonResponse(openTaskActivity);
        }
        return companyBackstop(url);
      },
      { org360: org360WithOpenTask },
    );
    render(<CompanyScreen id="o-1" />);
    await openTasksTab(user);
    await user.click(await screen.findByText(openTask.subject));
    await waitFor(() =>
      expect(
        screen.getByRole("dialog", { name: openTask.subject }),
      ).toBeTruthy(),
    );

    await user.click(screen.getByRole("button", { name: /^Deals/ }));
    await waitFor(() =>
      expect(
        screen.queryByRole("dialog", { name: openTask.subject }),
      ).toBeNull(),
    );
    await user.click(screen.getByRole("button", { name: /^Tasks/ }));

    await waitFor(() =>
      expect(screen.getByText(openTask.subject)).toBeTruthy(),
    );
    expect(screen.queryByRole("dialog", { name: openTask.subject })).toBeNull();
  });
});
