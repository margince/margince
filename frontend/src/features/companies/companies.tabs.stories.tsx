// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { taskWriteKeys } from "../../lib/activitykeys";
import { company, company360 } from "../../testing/company.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { useTaskUpdate } from "../worklist/taskactions";
import { CompanyDealsAndTasksTabs } from "./companies.tabs";

export default {
  title: "Records/Company 360/Task context",
  component: CompanyDealsAndTasksTabs,
} satisfies Meta<typeof CompanyDealsAndTasksTabs>;

type Story = StoryObj;

function TaskTab() {
  const [opened, setOpened] = useState<string | null>("task-one");
  const update = useTaskUpdate(taskWriteKeys("company", company.id));
  return (
    <CompanyDealsAndTasksTabs
      tab="tasks"
      company={company}
      view={company360}
      loading={false}
      failed={false}
      openTaskId={opened}
      onOpenTask={setOpened}
      onOpenRecord={() => {}}
      onOpenReceipt={() => {}}
      taskUpdate={update}
    />
  );
}

export const SourceLinkedTask: Story = {
  render() {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read", "update"] }),
      "GET /activities/task-one": () =>
        jsonResponse({
          id: "task-one",
          kind: "task",
          subject: "Send revised scope",
          body: "Include delivery options.",
          occurred_at: "2026-07-06T09:00:00Z",
          content_state: "available",
          links: [],
          version: 1,
          source_activity_id: "note-one",
        }),
      "GET /activities/note-one": () =>
        jsonResponse({
          id: "note-one",
          kind: "note",
          subject: "Scope discussion",
          body: "The buyer asked for delivery options.",
          occurred_at: "2026-07-05T09:00:00Z",
          content_state: "available",
          links: [],
        }),
    });
    return (
      <StoryProviders>
        <TaskTab />
      </StoryProviders>
    );
  },
};
export const SourceLinkedTaskDark: Story = {
  ...SourceLinkedTask,
  globals: { theme: "dark" },
};
