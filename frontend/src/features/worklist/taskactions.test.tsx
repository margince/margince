/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vitest";
import { CallLogHost } from "../../components/calllog.host";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { useMe } from "../../lib/session";
import { type WriteTo, WriteToProvider } from "../../lib/writeto";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "../../testing/story-utils";
import { TaskDetailModal } from "./taskdetailmodal";

afterEach(cleanup);

// The provenance sentence and the way back to the record behind it.
//
// A task read out of a meeting says so in words — "committed to this in the
// meeting transcript (line 6)" — and for as long as that sentence was all it
// carried, the only route to the meeting was the record's history tab and a
// search for its exact subject. These hold the link, and hold that a task
// nobody read out of anything does not offer one.

function stubTask(sourceActivityId: string | null, capturedBy = "human:u-1") {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    "GET /activities/task-1": () =>
      jsonResponse({
        id: "task-1",
        kind: "task",
        subject: "Create a comparison sheet with two products",
        body: "Lena Fischer committed to this in the meeting transcript (line 6).",
        due_at: "2026-09-11T21:59:59Z",
        occurred_at: "2026-09-08T09:00:00Z",
        is_done: false,
        source_activity_id: sourceActivityId,
        captured_by: capturedBy,
      }),
    // The PLAIN activity read, which is what the reader calls. Stubbing the
    // email presentation instead is what let a 404 ship: that endpoint refuses
    // any kind but `email`, and a fixture answering it anyway proved only that
    // the fixture answered.
    "GET /activities/meeting-9": () =>
      jsonResponse({
        id: "meeting-9",
        kind: "meeting",
        subject: "Akeneo — Vergleichsblatt",
        body: "Lena: Ich mache ein Vergleichsblatt mit zwei Produkten.",
        occurred_at: "2026-09-08T08:00:00Z",
        source: "transcript",
        captured_by: "human:u-1",
        version: 1,
      }),
  });
}

function openTask() {
  return render(
    <StoryProviders>
      <TaskDetailModal
        activityId="task-1"
        readOnly={false}
        onClose={() => {}}
        update={
          {
            mutate: () => {},
            isPending: false,
          } as unknown as Parameters<typeof TaskDetailModal>[0]["update"]
        }
      />
    </StoryProviders>,
  );
}

test("a task read out of a meeting opens that meeting", async () => {
  const user = userEvent.setup();
  stubTask("meeting-9");
  openTask();

  const open = await screen.findByRole("button", { name: "Open original" });
  await user.click(open);

  // The meeting itself, not another copy of the sentence naming it.
  await waitFor(() => {
    expect(screen.getByText("Akeneo — Vergleichsblatt")).not.toBeNull();
  });
});

test("a task nobody read out of a meeting offers no way back", async () => {
  stubTask(null);
  openTask();

  // The positive control is the test above: without it a build that never
  // rendered the button would pass this one for the wrong reason.
  await screen.findByText(/committed to this in the meeting transcript/);
  expect(screen.queryByRole("button", { name: "Open original" })).toBeNull();
});

test("a transcript refused on reopen is not painted from the cache", async () => {
  const user = userEvent.setup();
  // The kind lookup and the reader share one query key, and the lookup's
  // observer outlives the drawer — so `gcTime: 0` cannot evict between the
  // first read and the second, and the refused reopen has a cached transcript
  // sitting in `data`. Rendering that beside the error is what this holds.
  let refused = false;
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    "GET /activities/task-1": () =>
      jsonResponse({
        id: "task-1",
        kind: "task",
        subject: "Create a comparison sheet with two products",
        occurred_at: "2026-09-08T09:00:00Z",
        is_done: false,
        source_activity_id: "meeting-9",
      }),
    "GET /activities/meeting-9": () =>
      refused
        ? jsonResponse({ title: "Not found", status: 404 }, 404)
        : jsonResponse({
            id: "meeting-9",
            kind: "meeting",
            subject: "Akeneo — Vergleichsblatt",
            body: "Lena: Ich mache ein Vergleichsblatt mit zwei Produkten.",
            occurred_at: "2026-09-08T08:00:00Z",
            version: 1,
          }),
  });
  openTask();

  await user.click(
    await screen.findByRole("button", { name: "Open original" }),
  );
  await screen.findByText(/Vergleichsblatt mit zwei Produkten/);

  // Access goes away, and the reader is opened again. Escape rather than the
  // close control: the task drawer and the transcript reader are both dialogs
  // and both draw a "Close", so naming one finds two.
  refused = true;
  await user.keyboard("{Escape}");
  await user.click(
    await screen.findByRole("button", { name: "Open original" }),
  );

  // The words are gone, not merely accompanied by an error.
  await waitFor(() => {
    expect(screen.queryByText(/Vergleichsblatt mit zwei Produkten/)).toBeNull();
  });
});

test("a source that cannot be read says so instead of disappearing", async () => {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    "GET /activities/task-1": () =>
      jsonResponse({
        id: "task-1",
        kind: "task",
        subject: "Create a comparison sheet with two products",
        occurred_at: "2026-09-08T09:00:00Z",
        is_done: false,
        source_activity_id: "meeting-9",
      }),
    // The kind lookup fails, so nothing downstream knows which reader to be.
    "GET /activities/meeting-9": () =>
      jsonResponse({ title: "Server error", status: 500 }, 500),
  });
  openTask();

  // A retry, rather than a panel that silently removed the evidence: the email
  // panel's own failure arm cannot speak here, because it never mounts.
  expect(await screen.findByRole("button", { name: "Retry" })).toBeTruthy();
});

// An email-request task whose source message carries `parties`, read through
// the same presentation endpoint the panel asks.
function stubEmailRequest(parties: Record<string, unknown> = {}) {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    "GET /activities/task-1": () =>
      jsonResponse({
        id: "task-1",
        kind: "task",
        subject: "Report requested",
        source_activity_id: "email-1",
        occurred_at: "2026-09-01T09:00:00Z",
        is_done: false,
      }),
    "GET /activities/email-1": () =>
      jsonResponse({
        id: "email-1",
        kind: "email",
        body: "Unparsed body must not be displayed",
      }),
    "GET /activities/email-1/email-presentation": () =>
      jsonResponse({
        id: "email-1",
        lifecycle: "delivered",
        occurred_at: "2026-09-01T09:00:00Z",
        version: 1,
        summary: {
          activity_id: "email-1",
          occurred_at: "2026-09-01T09:00:00Z",
          version: 1,
          subject: "The original request",
          display_status: "team",
          move: "needs_reply",
          attachment_count: 0,
        },
        body: "Please send the report.",
        from: [],
        to: [],
        cc: [],
        bcc: [],
        bcc_withheld: false,
        attachments: [],
        links: [],
        thread: { members: [], next_cursor: null },
        can_reply: false,
        can_relink: false,
        access: {
          content_state: "available",
          display_status: "team",
          audience: "workspace",
          can_change: false,
          change_mode: "none",
        },
        ...parties,
      }),
  });
}

test("an email request is read in the task, with no click to open it", async () => {
  stubEmailRequest();
  openTask();

  // The message itself, with nothing pressed. It used to sit behind "Open
  // original", which put a second drawer over the task and cost a click to see
  // the one fact the task rests on.
  expect(await screen.findByText("Please send the report.")).toBeTruthy();
  // The presentation's body, never the raw activity's — that endpoint is the
  // normalised read, and the plain activity carries an unparsed copy.
  expect(screen.queryByText("Unparsed body must not be displayed")).toBeNull();
  // And no button, because there is nothing left for one to open.
  expect(screen.queryByRole("button", { name: "Open original" })).toBeNull();

  // WHERE it sits is the requirement, not merely that it is present: the
  // message reads under the task's own verbs, so a mail long enough to scroll
  // cannot push the Done button and the date picker off the panel.
  const moveTo = screen.getByText("Move to");
  const message = screen.getByText("Please send the report.");
  expect(
    moveTo.compareDocumentPosition(message) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
});

test("the source email names who sent it and who was copied, above the words", async () => {
  stubEmailRequest({
    from: [
      {
        address: "minh@example.com",
        display_name: "Nhật Minh Nguyễn",
        contact_id: "c0000000-0000-4000-8000-000000000001",
      },
    ],
    to: [{ address: "lars@example.com", display_name: "Lars" }],
    cc: [
      { address: "anna@example.com", display_name: "Anna Weber" },
      { address: "tom@example.com", display_name: null },
    ],
    bcc_withheld: true,
  });
  openTask();

  const sender = await screen.findByRole("link", { name: "Nhật Minh Nguyễn" });
  expect(sender.getAttribute("href")).toContain(
    "c0000000-0000-4000-8000-000000000001",
  );
  const line = (label: string) => screen.getByText(label).parentElement;
  expect(line("From")?.textContent).toBe("FromNhật Minh Nguyễn");
  expect(line("To")?.textContent).toBe("ToLars");
  // A party with no display name is named by its address, not dropped.
  expect(line("Cc")?.textContent).toBe("CcAnna Weber, tom@example.com");
  expect(screen.getByText(/blind-copied and are not shown/)).toBeTruthy();

  // Above the body: the quoted history's own From/To lines sit below the
  // reply's words, and with no envelope over them a reader took them for the
  // reply's.
  const message = screen.getByText("Please send the report.");
  const envelope = [
    line("From"),
    line("To"),
    line("Cc"),
    screen.getByText(/blind-copied and are not shown/),
  ];
  for (const envelopeLine of envelope) {
    expect(
      (envelopeLine?.compareDocumentPosition(message) ?? 0) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  }
});

test("a task an agent wrote is marked as AI-assisted", async () => {
  stubTask(null, "agent:commitment-reader");
  openTask();
  expect(await screen.findByText("AI-assisted")).not.toBeNull();
});

test("a task a colleague typed carries no AI mark", async () => {
  stubTask(null);
  openTask();
  await screen.findByText(/committed to this in the meeting transcript/);
  expect(screen.queryByText("AI-assisted")).toBeNull();
});

// The record a task is filed under, named and reachable from the panel.

function stubFiledTask(links: { entity_type: string; entity_id: string }[]) {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    "GET /activities/task-1": () =>
      jsonResponse({
        id: "task-1",
        kind: "task",
        subject: "Send the renewal paperwork",
        occurred_at: "2026-09-08T09:00:00Z",
        is_done: false,
        links,
      }),
    "GET /deals/deal-1": () =>
      jsonResponse({ id: "deal-1", name: "Brandt renewal 2027" }),
    "GET /companies/co-1": () =>
      jsonResponse({ id: "co-1", display_name: "Brandt Automotive" }),
  });
}

test("a task filed under a deal and its company names the deal first", async () => {
  // The company first on the wire, so the order on screen is the panel's own.
  stubFiledTask([
    { entity_type: "company", entity_id: "co-1" },
    { entity_type: "deal", entity_id: "deal-1" },
  ]);
  openTask();

  const deal = await screen.findByRole("link", { name: "Brandt renewal 2027" });
  await screen.findByRole("link", { name: "Brandt Automotive" });
  const names = screen.getAllByRole("link").map((link) => link.textContent);
  expect(names).toEqual(["Brandt renewal 2027", "Brandt Automotive"]);
  // Opened in place: following the record is leaving the panel for it.
  expect(deal.getAttribute("href")).toBe("#/deals/deal-1");
  expect(deal.getAttribute("target")).toBeNull();
});

test("a task filed under nothing names no record", async () => {
  stubFiledTask([]);
  openTask();

  await screen.findByText(/^Logged/);
  expect(screen.queryByRole("link")).toBeNull();
});

// What the rep does next about the contact a task is owed to.

const ALICE = {
  id: "c-1",
  full_name: "Alice Brandt",
  primary_email: "alice@example.com",
  phones: [{ id: "ph-1", phone: "+49 30 1234567", is_primary: true }],
  archived_at: null,
};

type NextStepsCase = Readonly<{
  source?: Record<string, unknown> | null;
  contact?: Record<string, unknown>;
  links?: { entity_type: string; entity_id: string }[];
  isDone?: boolean;
  contactRefused?: () => boolean;
  routes?: RouteMap;
}>;

function stubNextSteps({
  source = { kind: "call", content_state: "available" },
  contact = ALICE,
  links = [{ entity_type: "contact", entity_id: "c-1" }],
  isDone = false,
  contactRefused = () => false,
  routes = {},
}: NextStepsCase = {}) {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    "GET /activities/task-1": () =>
      jsonResponse({
        id: "task-1",
        kind: "task",
        subject: "Call back Alice about the renewal",
        occurred_at: "2026-09-08T09:00:00Z",
        is_done: isDone,
        source_activity_id: source ? "src-1" : null,
        links,
      }),
    "GET /activities/src-1": () =>
      jsonResponse({
        id: "src-1",
        subject: "Voicemail",
        occurred_at: "2026-09-08T08:00:00Z",
        version: 1,
        ...source,
      }),
    "GET /contacts/c-1": () =>
      contactRefused()
        ? jsonResponse({ title: "Forbidden", status: 403 }, 403)
        : jsonResponse(contact),
    "GET /contacts/c-2": () =>
      jsonResponse({ ...ALICE, id: "c-2", full_name: "Bob Brandt" }),
    ...routes,
  });
}

function openTaskWith(
  options: Readonly<{ readOnly?: boolean; writeTo?: WriteTo }> = {},
) {
  const modal = (
    <TaskDetailModal
      activityId="task-1"
      readOnly={options.readOnly ?? false}
      onClose={() => {}}
      update={
        {
          mutate: () => {},
          isPending: false,
        } as unknown as Parameters<typeof TaskDetailModal>[0]["update"]
      }
    />
  );
  return render(
    <StoryProviders>
      {options.writeTo ? (
        <WriteToProvider writeTo={options.writeTo}>{modal}</WriteToProvider>
      ) : (
        modal
      )}
    </StoryProviders>,
  );
}

const call = () => screen.queryByRole("link", { name: en["record.verb.call"] });
const describedBy = (control: HTMLElement) =>
  (control.getAttribute("aria-describedby") ?? "")
    .split(" ")
    .map((id) => document.getElementById(id)?.textContent)
    .join(" ");
const openContact = () =>
  screen.queryByRole("link", { name: en["task.next.openContact"] });

test("a task from a call offers Call, Email and Open contact", async () => {
  const user = userEvent.setup();
  const writeTo = vi.fn();
  stubNextSteps();
  openTaskWith({ writeTo });

  const dial = await screen.findByRole("link", {
    name: en["record.verb.call"],
  });
  expect(dial.getAttribute("href")).toBe("tel:+49301234567");
  expect(openContact()?.getAttribute("href")).toBe("#/contacts/c-1");
  await user.click(
    screen.getByRole("button", { name: en["record.verb.email"] }),
  );
  expect(writeTo).toHaveBeenCalledWith({
    entityType: "contact",
    entityId: "c-1",
    address: "alice@example.com",
  });
});

test("with no composer to open, Email hands the address to the mail client", async () => {
  stubNextSteps();
  openTaskWith();

  const email = await screen.findByRole("link", {
    name: en["record.verb.email"],
  });
  expect(email.getAttribute("href")).toBe("mailto:alice@example.com");
});

test("Call dials the primary number, not the first one listed", async () => {
  stubNextSteps({
    contact: {
      ...ALICE,
      phones: [
        { id: "ph-1", phone: "+49301234567", is_primary: false },
        { id: "ph-2", phone: "+491517654321", is_primary: true },
      ],
    },
  });
  openTaskWith();

  const dial = await screen.findByRole("link", {
    name: en["record.verb.call"],
  });
  expect(dial.getAttribute("href")).toBe("tel:+491517654321");
});

test.each([
  ["typed by hand", null],
  ["read out of a note", { kind: "note", content_state: "available" }],
  ["read out of a withheld call", { kind: "call", content_state: "withheld" }],
])(
  "a task %s offers Email and Open contact, and no Call",
  async (_, source) => {
    stubNextSteps({ source });
    openTaskWith();

    await waitFor(() => expect(openContact()).not.toBeNull());
    // The source has answered, so a missing Call is the decision and not a wait.
    if (source?.content_state === "withheld") {
      await screen.findByText(en["email.detail.withheldReason"]);
    } else if (source) {
      await screen.findByRole("button", { name: "Open original" });
    }
    expect(
      screen.getByRole("link", { name: en["record.verb.email"] }),
    ).toBeTruthy();
    expect(call()).toBeNull();
  },
);

test("a contact with no phone gets no Call", async () => {
  stubNextSteps({ contact: { ...ALICE, phones: [] } });
  openTaskWith();

  await waitFor(() => expect(openContact()).not.toBeNull());
  expect(call()).toBeNull();
});

test("a contact with no address gets Email refused with the reason", async () => {
  stubNextSteps({ contact: { ...ALICE, primary_email: null } });
  openTaskWith();

  const email = await screen.findByRole("button", {
    name: en["record.verb.email"],
  });
  expect(email.hasAttribute("disabled")).toBe(true);
  expect(describedBy(email)).toBe(en["task.next.noAddress"]);
  expect(call()).not.toBeNull();
});

test("an archived contact gets no Call and a refused Email, and can still be opened", async () => {
  stubNextSteps({
    contact: { ...ALICE, archived_at: "2026-09-01T00:00:00Z" },
  });
  openTaskWith();

  const email = await screen.findByRole("button", {
    name: en["record.verb.email"],
  });
  expect(email.hasAttribute("disabled")).toBe(true);
  expect(describedBy(email)).toBe(en["contact.rail.archivedReadOnly"]);
  expect(openContact()).not.toBeNull();
  expect(call()).toBeNull();
});

test("a refused Email is said once, after every verb in the row", async () => {
  stubNextSteps({ contact: { ...ALICE, primary_email: null } });
  openTaskWith();

  const email = await screen.findByRole("button", {
    name: en["record.verb.email"],
  });
  const reasons = screen.getAllByText(en["task.next.noAddress"]);
  expect(reasons).toHaveLength(1);
  const row = email.closest(".form-actions");
  expect(reasons[0]?.parentElement).toBe(row);
  expect(row?.lastElementChild).toBe(reasons[0]);
});

test.each([
  ["no contact", [{ entity_type: "company", entity_id: "co-1" }]],
  [
    "two contacts",
    [
      { entity_type: "contact", entity_id: "c-1" },
      { entity_type: "contact", entity_id: "c-2" },
    ],
  ],
])("a task linking %s offers no next steps", async (_, links) => {
  stubNextSteps({ links });
  openTaskWith();

  await screen.findByText(/^Logged/);
  // The source read answers before the verbs would be drawn.
  await screen.findByRole("button", { name: "Open original" });
  expect(openContact()).toBeNull();
  expect(screen.queryByText(en["record.verb.email"])).toBeNull();
  expect(call()).toBeNull();
});

test("a contact that stops being readable takes its verbs with it", async () => {
  let refused = false;
  stubNextSteps({ contactRefused: () => refused });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <TaskDetailModal
          activityId="task-1"
          readOnly={false}
          onClose={() => {}}
          update={
            {
              mutate: () => {},
              isPending: false,
            } as unknown as Parameters<typeof TaskDetailModal>[0]["update"]
          }
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  await screen.findByRole("link", { name: en["record.verb.call"] });

  refused = true;
  await client.refetchQueries({ queryKey: ["contact", "c-1"] });

  await waitFor(() => expect(openContact()).toBeNull());
  expect(call()).toBeNull();
});

test.each([
  ["a done task", { isDone: true }, {}],
  ["a read-only dialog", {}, { readOnly: true }],
])("%s still offers the next steps", async (_, task, dialog) => {
  stubNextSteps(task);
  openTaskWith(dialog);

  expect(
    await screen.findByRole("link", { name: en["record.verb.call"] }),
  ).toBeTruthy();
  expect(openContact()).not.toBeNull();
  expect(screen.queryByRole("button", { name: en["tasks.snooze"] })).toBeNull();
});

// The task, its call source and its contact already cached when the dialog opens.
function openWarm({ isDone = false, readOnly = false } = {}) {
  stubNextSteps({ isDone });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["activity", "task-1"], {
    id: "task-1",
    kind: "task",
    subject: "Call back Alice about the renewal",
    body: "Agenda: https://example.com/agenda",
    occurred_at: "2026-09-08T09:00:00Z",
    is_done: isDone,
    source_activity_id: "src-1",
    links: [{ entity_type: "contact", entity_id: "c-1" }],
  });
  client.setQueryData(["activity", "src-1"], {
    id: "src-1",
    kind: "call",
    content_state: "available",
    occurred_at: "2026-09-08T08:00:00Z",
  });
  client.setQueryData(["contact", "c-1"], ALICE);
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <TaskDetailModal
          activityId="task-1"
          readOnly={readOnly}
          onClose={() => {}}
          update={
            {
              mutate: () => {},
              isPending: false,
            } as unknown as Parameters<typeof TaskDetailModal>[0]["update"]
          }
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return client;
}

test("a dialog opened on warm reads puts first focus on Done, not on a link or Call", async () => {
  const client = openWarm();

  const done = screen.getByRole("button", { name: en["tasks.complete"] });
  expect(
    screen.getByRole("link", { name: "https://example.com/agenda" }),
  ).toBeTruthy();
  expect(
    screen.getByRole("link", { name: en["record.verb.call"] }),
  ).toBeTruthy();
  expect(document.activeElement).toBe(done);
  // Reads answering after the open move nothing.
  await waitFor(() => expect(client.isFetching()).toBe(0));
  expect(document.activeElement).toBe(done);
});

test.each([
  ["a done task", { isDone: true }],
  ["a read-only dialog", { readOnly: true }],
])(
  "%s opened on warm reads puts first focus on the dialog box",
  async (_, options) => {
    const client = openWarm(options);

    const box = screen.getByRole("dialog");
    expect(
      screen.getByRole("link", { name: en["record.verb.call"] }),
    ).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "https://example.com/agenda" }),
    ).toBeTruthy();
    expect(document.activeElement).toBe(box);
    await waitFor(() => expect(client.isFetching()).toBe(0));
    expect(document.activeElement).toBe(box);
  },
);

function SessionAnswered() {
  return useMe().data ? <p>session answered</p> : null;
}

test("Call logs the call inside the dialog, on the task's contact, and leaves the task open", async () => {
  const user = userEvent.setup();
  const posted: unknown[] = [];
  stubNextSteps({
    routes: {
      "GET /me": meRoute({ activity: ["read", "create"] }, { seat: "full" }),
      "POST /activities": (body) => {
        posted.push(body);
        return jsonResponse({ id: "a-9", kind: "call" }, 201);
      },
    },
  });
  render(
    <StoryProviders>
      <CallLogHost>
        <SessionAnswered />
        <TaskDetailModal
          activityId="task-1"
          readOnly={false}
          onClose={() => {}}
          update={
            {
              mutate: () => {},
              isPending: false,
            } as unknown as Parameters<typeof TaskDetailModal>[0]["update"]
          }
        />
      </CallLogHost>
    </StoryProviders>,
  );
  await screen.findByText("session answered");

  const dial = await screen.findByRole("link", {
    name: en["record.verb.call"],
  });
  dial.focus();
  await user.keyboard("{Enter}");
  const log = await screen.findByRole("region", { name: en["log.title"] });
  expect(screen.getAllByRole("dialog")).toHaveLength(1);
  expect(screen.getByRole("dialog").contains(log)).toBe(true);
  expect(log.contains(document.activeElement)).toBe(true);
  expect(
    within(log).getByRole("combobox", { name: "Type" }).textContent,
  ).toContain("Call");
  await user.type(
    within(log).getByRole("textbox", { name: "Subject" }),
    "Left a voicemail",
  );
  await user.click(within(log).getByRole("button", { name: "Log" }));

  await waitFor(() => expect(posted).toHaveLength(1));
  expect(posted[0]).toMatchObject({
    kind: "call",
    links: [{ entity_type: "contact", entity_id: "c-1" }],
  });
  await waitFor(() =>
    expect(screen.queryByRole("region", { name: en["log.title"] })).toBeNull(),
  );
  expect(screen.getByRole("dialog")).toBeTruthy();
  expect(document.activeElement).toBe(dial);
});
