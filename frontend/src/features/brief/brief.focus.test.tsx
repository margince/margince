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
import { taskRow } from "../../testing/brief.fixtures";
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
  const details = card.getByRole("button", { name: en["brief.focus.context"] });
  expect(
    draft.compareDocumentPosition(details) & Node.DOCUMENT_POSITION_FOLLOWING,
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

it("Next task opens the following card, wraps past the last, and is absent alone", async () => {
  const second = taskRow("task-2", "Send the sheet");
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }, { roles: ["rep"] }),
  });
  const chosen = vi.fn();
  const triage = (rows: WorklistItem[], at: number) => (
    <StoryProviders>
      <Triage
        rows={rows}
        at={at}
        lead={rows[at]}
        onChoose={chosen}
        onOpenEmail={() => {}}
      />
    </StoryProviders>
  );
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
