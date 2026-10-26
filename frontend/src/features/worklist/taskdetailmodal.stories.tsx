// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useTaskUpdate } from "../../components/taskactions";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { TaskDetailModal } from "./taskdetailmodal";
import { worklistKey } from "./worklist.queries";

export {
  CapturedEmailRequest,
  DetailOpen,
  FiledUnderDeal,
  SourceTranscriptOpen,
} from "./taskactions.stories";

export default {
  title: "Patterns/Task detail",
  component: TaskDetailModal,
} satisfies Meta<typeof TaskDetailModal>;

type Story = StoryObj<typeof TaskDetailModal>;

// Raised inside StoryProviders, which holds the query client the hook reads.
function ReminderFromRow() {
  const update = useTaskUpdate([worklistKey]);
  return (
    <TaskDetailModal
      activityId="reminder-1"
      readOnly={false}
      onClose={() => {}}
      update={update}
      item={{
        id: "reminder-1",
        source: "task",
        category: "tasks",
        level: 2,
        consequence: "task_slips",
        because: [{ kind: "due_today" }, { kind: "unassigned" }],
        actions: ["complete", "snooze"],
      }}
    />
  );
}

// A reminder an automation wrote, opened from the list. It has no body, so the
// drawer says what made it and why it is on the list.
export const AutomaticReminderFromRow: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read", "update"] }, { seat: "full" }),
      "GET /activities/reminder-1": () =>
        jsonResponse({
          id: "reminder-1",
          kind: "task",
          writable: true,
          subject: "Check in — no activity since 2026-06-30",
          body: null,
          due_at: "2026-09-10T21:59:59Z",
          occurred_at: "2026-09-06T13:42:00Z",
          is_done: false,
          version: 1,
          source_system: "no_activity_reminder",
          captured_by: "system:automation",
          links: [{ entity_type: "company", entity_id: "o-1" }],
        }),
    });
    return (
      <StoryProviders>
        <ReminderFromRow />
      </StoryProviders>
    );
  },
};
