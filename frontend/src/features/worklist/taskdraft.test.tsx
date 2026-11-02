/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vitest";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import { type WriteTo, WriteToProvider } from "../../lib/writeto";
import { SourceHost } from "../../testing/sourcehost";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { TimelineActions } from "../records/timelineactions";
import { TaskDetailModal } from "./taskdetailmodal";

afterEach(cleanup);

// Draft email on a task goes where the server says (`email_target`): one
// contact, else one live lead, else one company. The dialog, the Home card and
// the timeline all read that one answer; nothing here resolves links itself.

type Activity = components["schemas"]["Activity"];
type EmailTarget = components["schemas"]["ActivityEmailTarget"];

const SUBJECT = "Email Company A";
const INTENT = `${en["task.next.draftIntent"]}: ${SUBJECT}`;

function taskWith(target: EmailTarget | undefined): Activity {
  return {
    id: "task-1",
    kind: "task",
    writable: true,
    subject: SUBJECT,
    occurred_at: "2026-09-08T09:00:00Z",
    occurred_precision: "instant",
    is_done: false,
    source_activity_id: null,
    links: [{ entity_type: "company", entity_id: "co-1" }],
    offers_email: target !== undefined && target.entity_type !== "company",
    email_target: target,
    source: "manual",
    captured_by: "human:u-1",
    created_at: "2026-09-08T09:00:00Z",
    updated_at: "2026-09-08T09:00:00Z",
  };
}

function stubTask(task: Activity | null) {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read", "update"] }, { seat: "full" }),
    "GET /activities/task-1": () =>
      task
        ? jsonResponse(task)
        : jsonResponse({ title: "Unavailable", status: 503 }, 503),
    "GET /contacts/c-1": () =>
      jsonResponse({
        id: "c-1",
        full_name: "Alice Brandt",
        primary_email: "alice@example.com",
        phones: [],
        archived_at: null,
      }),
  });
}

function openTask(writeTo: WriteTo) {
  render(
    <StoryProviders>
      <SourceHost>
        <WriteToProvider writeTo={writeTo}>
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
        </WriteToProvider>
      </SourceHost>
    </StoryProviders>,
  );
}

const draftEmail = () =>
  screen.findByRole("button", { name: en["task.next.draftEmail"] });

test.each([
  [
    "a contact",
    { entity_type: "contact", entity_id: "c-1" },
    { entityType: "contact", entityId: "c-1", address: "alice@example.com" },
  ],
  [
    "a lead",
    { entity_type: "lead", entity_id: "l-1" },
    { entityType: "lead", entityId: "l-1" },
  ],
  [
    "a company",
    { entity_type: "company", entity_id: "co-1" },
    { entityType: "company", entityId: "co-1" },
  ],
] as const)(
  "a task whose target is %s drafts to it",
  async (_, target, opened) => {
    const user = userEvent.setup();
    const writeTo = vi.fn();
    stubTask(taskWith(target));
    openTask(writeTo);

    await user.click(await draftEmail());
    expect(writeTo).toHaveBeenCalledWith({ ...opened, draftIntent: INTENT });
  },
);

test.each([
  ["lead_closed", "lead", en["lead.terminalReadOnly"]],
  ["no_address", "lead", en["task.next.noLeadAddress"]],
  ["no_address", "contact", en["task.next.noAddress"]],
  ["archived", "contact", en["contact.rail.archivedReadOnly"]],
] as const)("a %s refusal on a %s says why", async (refusal, kind, reason) => {
  stubTask(
    taskWith({
      entity_type: kind,
      entity_id: kind === "contact" ? "c-1" : "l-1",
      refusal,
    }),
  );
  openTask(vi.fn());

  const email = await draftEmail();
  expect(email.hasAttribute("disabled")).toBe(true);
  expect(screen.getByText(reason)).toBeTruthy();
});

test("a task the server names no target for offers no Draft email", async () => {
  stubTask(taskWith(undefined));
  openTask(vi.fn());

  await screen.findByText(SUBJECT);
  expect(
    screen.queryByRole("button", { name: en["task.next.draftEmail"] }),
  ).toBeNull();
});

test("a task that could not be read offers no Draft email", async () => {
  stubTask(null);
  openTask(vi.fn());

  await waitFor(() =>
    expect(document.querySelector(".task-next-pending")).toBeNull(),
  );
  expect(
    screen.queryByRole("button", { name: en["task.next.draftEmail"] }),
  ).toBeNull();
});

// The timeline row and the dialog read the same field, so one task offers
// the email on both or on neither.
test.each([
  ["a lead", { entity_type: "lead", entity_id: "l-1" }, true],
  ["a company", { entity_type: "company", entity_id: "co-1" }, true],
  [
    "a closed lead",
    { entity_type: "lead", entity_id: "l-1", refusal: "lead_closed" },
    false,
  ],
  ["nothing", undefined, false],
] as const)(
  "the timeline and the dialog agree on a task targeting %s",
  async (_, target, offered) => {
    const task = taskWith(target);
    stubTask(task);
    render(
      <StoryProviders>
        <TimelineActions activity={task} entityType="company" entityId="co-1" />
      </StoryProviders>,
    );
    await screen.findByRole("button", { name: "Open task" });
    const onTimeline =
      screen.queryByRole("button", { name: en["compose.writeEmail"] }) !== null;
    cleanup();

    openTask(vi.fn());
    await screen.findByText(SUBJECT);
    // A lead or company target needs no further read, so the verbs are drawn
    // with the task.
    const draft = screen.queryByRole("button", {
      name: en["task.next.draftEmail"],
    });
    const inDialog = draft !== null && !draft.hasAttribute("disabled");

    expect(onTimeline).toBe(offered);
    expect(inDialog).toBe(offered);
  },
);
