/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vitest";
import { en } from "../../i18n/en";
import { type WriteTo, WriteToProvider } from "../../lib/writeto";
import { SourceHost } from "../../testing/sourcehost";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { TaskDetailModal } from "./taskdetailmodal";

afterEach(cleanup);

// Every task offers Draft email to the record it is about. One contact is the
// contact case. Without one, the task drafts to its live lead, else to its
// company. The task's title is never read to decide.

type Link = { entity_type: string; entity_id: string };

const SUBJECT = "Email Company A";
const INTENT = `${en["task.next.draftIntent"]}: ${SUBJECT}`;

const LEAD = {
  id: "l-1",
  full_name: "Lena Lead",
  email: "lena@prospect.example",
  status: "contacted",
  archived_at: null,
};

function stubTask(links: Link[], lead: Record<string, unknown> = LEAD) {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read", "update"] }, { seat: "full" }),
    "GET /activities/task-1": () =>
      jsonResponse({
        id: "task-1",
        kind: "task",
        writable: true,
        subject: SUBJECT,
        occurred_at: "2026-09-08T09:00:00Z",
        is_done: false,
        source_activity_id: null,
        links,
      }),
    "GET /leads/l-1": () => jsonResponse(lead),
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
    "one contact",
    [
      { entity_type: "contact", entity_id: "c-1" },
      { entity_type: "company", entity_id: "co-1" },
    ],
    { entityType: "contact", entityId: "c-1", address: "alice@example.com" },
  ],
  [
    "a live lead",
    [
      { entity_type: "lead", entity_id: "l-1" },
      { entity_type: "company", entity_id: "co-1" },
    ],
    { entityType: "lead", entityId: "l-1", address: "lena@prospect.example" },
  ],
  [
    "a company",
    [{ entity_type: "company", entity_id: "co-1" }],
    { entityType: "company", entityId: "co-1" },
  ],
])("a task linked to %s drafts to it", async (_, links, target) => {
  const user = userEvent.setup();
  const writeTo = vi.fn();
  stubTask(links);
  openTask(writeTo);

  await user.click(await draftEmail());
  expect(writeTo).toHaveBeenCalledWith({ ...target, draftIntent: INTENT });
});

test("a task on a closed lead drafts to the company it also names", async () => {
  const user = userEvent.setup();
  const writeTo = vi.fn();
  stubTask(
    [
      { entity_type: "lead", entity_id: "l-1" },
      { entity_type: "company", entity_id: "co-1" },
    ],
    { ...LEAD, status: "disqualified", archived_at: "2026-09-01T00:00:00Z" },
  );
  openTask(writeTo);

  await user.click(await draftEmail());
  expect(writeTo).toHaveBeenCalledWith({
    entityType: "company",
    entityId: "co-1",
    draftIntent: INTENT,
  });
});

test.each([
  [
    "a closed lead and no company",
    { ...LEAD, archived_at: "2026-09-01T00:00:00Z" },
    en["lead.terminalReadOnly"],
  ],
  [
    "a lead with no address",
    { ...LEAD, email: null },
    en["task.next.noLeadAddress"],
  ],
])("a task on %s is refused with the reason", async (_, lead, reason) => {
  stubTask([{ entity_type: "lead", entity_id: "l-1" }], lead);
  openTask(vi.fn());

  const email = await draftEmail();
  expect(email.hasAttribute("disabled")).toBe(true);
  expect(screen.getByText(reason)).toBeTruthy();
  expect(
    screen.queryByRole("link", { name: en["task.next.openContact"] }),
  ).toBeNull();
});

test("a task linked to no record offers no Draft email", async () => {
  stubTask([]);
  openTask(vi.fn());

  await screen.findByText(SUBJECT);
  await waitFor(() =>
    expect(
      screen.queryByRole("button", { name: en["task.next.draftEmail"] }),
    ).toBeNull(),
  );
});
