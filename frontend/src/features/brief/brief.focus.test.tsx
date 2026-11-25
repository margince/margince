/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import type { TaskType } from "../../lib/tasktype";
import { type WriteTo, WriteToProvider } from "../../lib/writeto";
import { taskRow, waitingEmailRow } from "../../testing/brief.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { Triage } from "./brief.focus";

afterEach(cleanup);

type WorklistItem = components["schemas"]["WorklistItem"];

const LEAD: WorklistItem = {
  ...taskRow("task-1", "Call back Alice about the renewal"),
  contact: { id: "c-1", label: "Alice Brandt" },
};

function stubReader() {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }, { roles: ["rep"] }),
  });
}

/** The day's cards with the one at `at` in hand. */
function triageOf(
  rows: WorklistItem[],
  at: number,
  onChoose: (item: WorklistItem) => void = () => {},
) {
  return (
    <StoryProviders>
      <Triage
        rows={rows}
        at={at}
        lead={rows[at]}
        onChoose={onChoose}
        onOpenEmail={() => {}}
      />
    </StoryProviders>
  );
}

function showLead(
  lead: WorklistItem,
  taskType: TaskType,
  writeTo: WriteTo | null = null,
  replyChannel: "email" | "call" | null = null,
) {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }, { roles: ["rep"] }),
    "GET /activities/task-1": () =>
      jsonResponse({
        id: "task-1",
        kind: "task",
        subject: LEAD.title,
        occurred_at: "2026-09-08T09:00:00Z",
        is_done: false,
        task_type: taskType,
        reply_channel: replyChannel,
        source_activity_id: "call-1",
        links: [{ entity_type: "contact", entity_id: "c-1" }],
        // Only a mail task names a target; a call finds its contact by link.
        ...(taskType === "reach_out" || taskType === "reply"
          ? { email_target: { entity_type: "contact", entity_id: "c-1" } }
          : {}),
      }),
    "GET /activities/call-1": () =>
      jsonResponse({
        id: "call-1",
        kind: "call",
        content_state: "available",
        occurred_at: "2026-09-08T08:00:00Z",
      }),
    "GET /contacts/c-1": () =>
      jsonResponse({
        id: "c-1",
        full_name: "Alice Brandt",
        primary_email: "alice@example.com",
        phones: [{ id: "ph-1", phone: "+491517654321", is_primary: true }],
      }),
  });
  render(
    <StoryProviders>
      <WriteToProvider writeTo={writeTo}>
        <Triage
          rows={[lead]}
          at={0}
          lead={lead}
          onChoose={() => {}}
          onOpenEmail={() => {}}
          onContext={() => {}}
        />
      </WriteToProvider>
    </StoryProviders>,
  );
}

it("a reply to a call shows the number to copy and a filled Call, with Done outlined", async () => {
  showLead(LEAD, "reply", null, "call");

  const card = within(await screen.findByRole("article"));
  const dial = await screen.findByRole("link", {
    name: en["record.verb.call"],
  });
  expect(dial.getAttribute("href")).toBe("tel:+491517654321");
  expect(dial.classList.contains("btn-primary")).toBe(true);
  expect(
    card.getByRole("button", { name: en["task.next.copyNumber"] }),
  ).toBeTruthy();
  const done = card.getByRole("button", { name: en["tasks.complete"] });
  expect(done.classList.contains("btn-primary")).toBe(false);
  expect(
    card.queryByRole("link", { name: en["record.verb.email"] }),
  ).toBeNull();
  const toContact = card
    .getAllByRole("link")
    .filter((link) => link.getAttribute("href") === "/#/contacts/c-1");
  expect(toContact).toHaveLength(1);
});

it("a task to do has no step of its own, so Done is the filled button", async () => {
  showLead(LEAD, "do");

  const card = within(await screen.findByRole("article"));
  // The type chip comes with the fetched task, so the Do step is settled.
  await card.findByText(en["task.type.do"]);
  expect(
    card
      .getByRole("button", { name: en["tasks.complete"] })
      .classList.contains("btn-primary"),
  ).toBe(true);
  expect(card.queryByRole("link", { name: en["record.verb.call"] })).toBeNull();
});

it("reach out with no draft to offer makes Call the filled verb", async () => {
  showLead(LEAD, "reach_out");

  const card = within(await screen.findByRole("article"));
  const email = await card.findByRole("link", {
    name: en["record.verb.email"],
  });
  expect(email.classList.contains("btn-primary")).toBe(false);
  const dial = card.getByRole("link", { name: en["record.verb.call"] });
  expect(dial.classList.contains("btn-primary")).toBe(true);
  expect(
    card
      .getByRole("button", { name: en["tasks.complete"] })
      .classList.contains("btn-primary"),
  ).toBe(false);
});

it("a reach out whose row names no contact still offers the task's contact verbs", async () => {
  showLead({ ...LEAD, contact: undefined }, "reach_out");

  const card = within(await screen.findByRole("article"));
  expect(
    await card.findByRole("link", { name: en["record.verb.email"] }),
  ).toBeTruthy();
});

it("reach out drafts the mail first, then the number and an outlined Call", async () => {
  showLead(LEAD, "reach_out", vi.fn());

  const card = within(await screen.findByRole("article"));
  const draft = await card.findByRole("button", {
    name: en["task.next.draftEmail"],
  });
  expect(draft.classList.contains("btn-ai")).toBe(true);
  // One filled button to a card: beside the draft, Done is outlined.
  const done = card.getByRole("button", { name: en["tasks.complete"] });
  expect(done.classList.contains("btn-primary")).toBe(false);
  // The way to read more starts the line, so it comes first in the page too.
  const details = card.getByRole("button", { name: en["brief.focus.context"] });
  expect(
    details.compareDocumentPosition(draft) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
  expect(
    card.queryByRole("button", { name: en["record.verb.email"] }),
  ).toBeNull();
  const dial = card.getByRole("link", { name: en["record.verb.call"] });
  expect(dial.classList.contains("btn-primary")).toBe(false);
  expect(
    draft.compareDocumentPosition(dial) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
});

it("a reply to a mail answers by mail, and the menu still offers Call", async () => {
  showLead(LEAD, "reply", vi.fn(), "email");
  const user = userEvent.setup();

  const card = within(await screen.findByRole("article"));
  await card.findByRole("button", { name: en["task.next.draftEmail"] });
  expect(card.queryByRole("link", { name: en["record.verb.call"] })).toBeNull();
  await user.click(card.getByRole("button", { name: /^More actions for/ }));
  const dial = await screen.findByRole("link", {
    name: en["record.verb.call"],
  });
  expect(dial.getAttribute("href")).toBe("tel:+491517654321");
});

it("a reply to a call answers by phone, and the menu still offers Email", async () => {
  showLead(LEAD, "reply", null, "call");
  const user = userEvent.setup();

  const card = within(await screen.findByRole("article"));
  await card.findByRole("link", { name: en["record.verb.call"] });
  expect(
    card.queryByRole("link", { name: en["record.verb.email"] }),
  ).toBeNull();
  await user.click(card.getByRole("button", { name: /^More actions for/ }));
  const email = await screen.findByRole("link", {
    name: en["record.verb.email"],
  });
  expect(email.getAttribute("href")).toBe("mailto:alice@example.com");
});

it("a reach out on a promise made by mail drafts the mail, and Call waits in the menu", async () => {
  showLead(LEAD, "reach_out", vi.fn(), "email");
  const user = userEvent.setup();

  const card = within(await screen.findByRole("article"));
  await card.findByRole("button", { name: en["task.next.draftEmail"] });
  expect(card.queryByRole("link", { name: en["record.verb.call"] })).toBeNull();
  expect(
    card.queryByRole("button", { name: en["task.next.copyNumber"] }),
  ).toBeNull();
  await user.click(card.getByRole("button", { name: /^More actions for/ }));
  const dial = await screen.findByRole("link", {
    name: en["record.verb.call"],
  });
  expect(dial.getAttribute("href")).toBe("tel:+491517654321");
});

it("a reach out after a call leads with a filled Call, and Email waits in the menu", async () => {
  showLead(LEAD, "reach_out", vi.fn(), "call");
  const user = userEvent.setup();

  const card = within(await screen.findByRole("article"));
  const dial = await card.findByRole("link", { name: en["record.verb.call"] });
  expect(dial.classList.contains("btn-primary")).toBe(true);
  expect(
    card.queryByRole("button", { name: en["task.next.draftEmail"] }),
  ).toBeNull();
  await user.click(card.getByRole("button", { name: /^More actions for/ }));
  expect(
    await screen.findByRole("button", { name: en["task.next.draftEmail"] }),
  ).toBeTruthy();
});

// The verbs arrive after the card: the task is read, then its contact. Their
// row holds one verb's room meanwhile, so the card does not grow under a reader.
it("a task lead holds its verbs' room until the task and its contact are read", async () => {
  let landTask: (response: Response) => void = () => {};
  let landContact: ((response: Response) => void) | undefined;
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }, { roles: ["rep"] }),
    "GET /activities/task-1": () =>
      new Promise<Response>((land) => {
        landTask = land;
      }),
    "GET /contacts/c-1": () =>
      new Promise<Response>((land) => {
        landContact = land;
      }),
  });
  render(
    <StoryProviders>
      <Triage
        rows={[LEAD]}
        at={0}
        lead={LEAD}
        onChoose={() => {}}
        onOpenEmail={() => {}}
        onContext={() => {}}
      />
    </StoryProviders>,
  );
  const article = await screen.findByRole("article");
  expect(article.querySelector(".task-next-pending")).not.toBeNull();

  landTask(
    jsonResponse({
      id: "task-1",
      kind: "task",
      subject: LEAD.title,
      occurred_at: "2026-09-08T09:00:00Z",
      is_done: false,
      task_type: "reach_out",
      links: [{ entity_type: "contact", entity_id: "c-1" }],
      email_target: { entity_type: "contact", entity_id: "c-1" },
    }),
  );
  await vi.waitFor(() => expect(landContact).toBeDefined());
  expect(article.querySelector(".task-next-pending")).not.toBeNull();

  landContact?.(
    jsonResponse({
      id: "c-1",
      full_name: "Alice Brandt",
      primary_email: "alice@example.com",
    }),
  );
  const card = within(article);
  expect(
    await card.findByRole("link", { name: en["record.verb.email"] }),
  ).toBeTruthy();
  expect(article.querySelector(".task-next-pending")).toBeNull();
});

// Done's weight follows the task's type and contact, so it waits for both.
// A filled Done that a filled step then quiets is a flash.
it("a task lead draws Done only once the read that weighs it has landed", async () => {
  let landTask: (response: Response) => void = () => {};
  let landContact: ((response: Response) => void) | undefined;
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }, { roles: ["rep"] }),
    "GET /activities/task-1": () =>
      new Promise<Response>((land) => {
        landTask = land;
      }),
    "GET /contacts/c-1": () =>
      new Promise<Response>((land) => {
        landContact = land;
      }),
  });
  render(triageOf([LEAD], 0));
  const card = within(await screen.findByRole("article"));
  const done = () => card.queryByRole("button", { name: en["tasks.complete"] });
  expect(done()).toBeNull();

  landTask(
    jsonResponse({
      id: "task-1",
      kind: "task",
      subject: LEAD.title,
      occurred_at: "2026-09-08T09:00:00Z",
      is_done: false,
      task_type: "meet",
      links: [{ entity_type: "contact", entity_id: "c-1" }],
    }),
  );
  await vi.waitFor(() => expect(landContact).toBeDefined());
  await card.findByText(en["task.type.meet"]);
  expect(done()).toBeNull();

  landContact?.(
    jsonResponse({ id: "c-1", full_name: "Alice Brandt", phones: [] }),
  );
  const drawn = await card.findByRole("button", {
    name: en["tasks.complete"],
  });
  expect(drawn.classList.contains("btn-primary")).toBe(false);
});

it("Next task opens the following card, wraps past the last, and is absent alone", async () => {
  const second = taskRow("task-2", "Send the sheet");
  stubReader();
  const chosen = vi.fn();
  const triage = (rows: WorklistItem[], at: number) =>
    triageOf(rows, at, chosen);
  const user = userEvent.setup();
  const view = render(triage([LEAD, second], 0));
  const next = () =>
    screen.getByRole("button", { name: en["brief.focus.next"] });

  // It ends the title line, apart from the verbs that answer the task.
  expect(next().closest(".worklist-row-title")).not.toBeNull();
  await user.click(next());
  expect(chosen).toHaveBeenLastCalledWith(second);

  view.rerender(triage([LEAD, second], 1));
  await user.click(next());
  expect(chosen).toHaveBeenLastCalledWith(LEAD);

  view.rerender(triage([LEAD], 0));
  expect(
    screen.queryByRole("button", { name: en["brief.focus.next"] }),
  ).toBeNull();
});

// A promise carrying the email it was read out of is still a task. It is named
// once, by the promise, and the card does not call it an email.
it("a promise carrying its email is named once and drawn as a task", async () => {
  const mail = waitingEmailRow().email_summary;
  if (!mail) throw new Error("the fixture carries its message");
  const promise: WorklistItem = {
    ...LEAD,
    band: "now",
    due_at: "2026-09-15T10:00:00Z",
    actions: ["complete", "reply"],
    subject: { type: "contact", id: "c-1", label: "Alice Brandt" },
    move: { action: "draft_reply", activity_id: mail.activity_id },
    email_summary: { ...mail, direction: "outbound", move: "waiting_for_them" },
  };
  showLead(promise, "do");

  const article = await screen.findByRole("article");
  const card = within(article);
  expect(card.getAllByText(LEAD.title ?? "")).toHaveLength(1);
  expect(card.queryByRole("heading", { name: LEAD.title })).toBeNull();
  // The email is drawn under the promise; the card's own meta names a task.
  const meta = article.querySelector(".brief-triage-lead-head");
  expect(meta?.textContent).not.toContain(en["timeline.kind.email"]);
  expect(article.querySelector(".task-meta-status")).toBeNull();
});

it("a card done from its own tick hands focus to the card that takes its place", () => {
  stubReader();
  const second = taskRow("task-2", "Send the sheet");
  const third = taskRow("task-3", "Book the venue");
  const triage = (rows: WorklistItem[]) => triageOf(rows, 0);
  const view = render(triage([LEAD, second, third]));
  screen.getByRole("button", { name: /^Mark done: Send the sheet/ }).focus();

  view.rerender(triage([LEAD, third]));

  const [, next] = document.querySelectorAll(
    ".brief-task-open, .brief-triage-frame",
  );
  expect(next?.textContent).toContain("Book the venue");
  expect(document.activeElement).toBe(next);
});

it("a card that leaves while focus is elsewhere moves no focus", () => {
  stubReader();
  const second = taskRow("task-2", "Send the sheet");
  const triage = (rows: WorklistItem[]) => triageOf(rows, 0);
  const view = render(triage([LEAD, second]));

  view.rerender(triage([LEAD]));

  expect(document.activeElement).toBe(document.body);
});

it("the card in hand reaches its controls in the order it draws them", async () => {
  showLead(LEAD, "do");
  const article = await screen.findByRole("article");
  const done = await within(article).findByRole("button", { name: "Done" });
  const stops = [...article.querySelectorAll<HTMLElement>("a[href], button")];
  const at = (stop: HTMLElement) => stops.indexOf(stop);
  const card = within(article);
  const title = card.getByRole("button", { name: LEAD.title });
  const chip = card.getByRole("link", { name: /Alice Brandt/ });
  const details = card.getByRole("button", {
    name: en["brief.focus.context"],
  });

  expect(at(title)).toBeLessThan(at(chip));
  expect(at(chip)).toBeLessThan(at(details));
  expect(at(details)).toBeLessThan(at(done));
});
