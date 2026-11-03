/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import type { components } from "../api/schema";
import { TimelineActions } from "../features/records/timelineactions";
import { TaskDetailModal } from "../features/worklist/taskdetailmodal";
import { en } from "../i18n/en";
import { WriteToProvider } from "../lib/writeto";
import { SourceHost } from "../testing/sourcehost";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";

afterEach(cleanup);

// A task's timeline row and its dialog read one server answer
// (`email_target`), so the same task offers the email on both or on neither.
// Here because only the shell may hold two features in one test.

type Activity = components["schemas"]["Activity"];
type EmailTarget = components["schemas"]["ActivityEmailTarget"];

const SUBJECT = "Email Company A";

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

function stubTask(task: Activity) {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read", "update"] }, { seat: "full" }),
    "GET /activities/task-1": () => jsonResponse(task),
  });
}

function openTask() {
  render(
    <StoryProviders>
      <SourceHost>
        <WriteToProvider writeTo={vi.fn()}>
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

    openTask();
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
