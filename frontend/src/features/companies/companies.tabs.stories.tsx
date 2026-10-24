// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { useTaskUpdate } from "../../components/taskactions";
import { taskWriteKeys } from "../../lib/activitykeys";
import { company, company360 } from "../../testing/company.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
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
      onOpenEmail={() => {}}
      onPrepareMeeting={() => {}}
      onDraftTo={() => {}}
      onPerform={() => {}}
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

// The Deals and projects tab with a live pipeline: one open deal with every
// column filled, one won deal with no amount or close day.
const pipeline = {
  ...company360,
  deals: {
    data: [
      {
        deal_id: "d-1",
        name: "Fleet retrofit",
        status: "open" as const,
        stage_name: "Solution Discovery",
        amount: { amount_minor: 4_800_000, currency: "EUR" },
        expected_close_date: "2026-09-30",
        stalled: false,
      },
      {
        deal_id: "d-2",
        name: "Phase one licence",
        status: "won" as const,
        stage_name: "Closed Won",
        stalled: false,
      },
    ],
    page: { has_more: false, next_cursor: null },
    won_lifetime: { amount_minor: 450_000, currency: "EUR" },
    lost_count: 0,
  },
  projects: [],
  projects_page: { has_more: false, next_cursor: null },
};

function DealsTab() {
  const update = useTaskUpdate(taskWriteKeys("company", company.id));
  return (
    <CompanyDealsAndTasksTabs
      tab="deals"
      company={company}
      view={pipeline}
      loading={false}
      failed={false}
      openTaskId={null}
      onOpenTask={() => {}}
      onOpenRecord={() => {}}
      onOpenReceipt={() => {}}
      onOpenEmail={() => {}}
      onPrepareMeeting={() => {}}
      onDraftTo={() => {}}
      onPerform={() => {}}
      taskUpdate={update}
    />
  );
}

export const DealsTable: Story = {
  render() {
    installFetchStub({ "GET /me": meRoute({ deal: ["read", "create"] }) });
    return (
      <StoryProviders>
        <DealsTab />
      </StoryProviders>
    );
  },
};
export const DealsTableDark: Story = {
  ...DealsTable,
  globals: { theme: "dark" },
};
