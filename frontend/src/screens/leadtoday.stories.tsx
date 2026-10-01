// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { useLocale, useT } from "../i18n";
import { leadTodoRows } from "./leadtoday";
import { TodayPanel } from "./record360/today";
import { StoryProviders } from "./story-utils";

// Recorded lead tasks, including an imported prospect with no planned work.

type Lead = components["schemas"]["Lead"];

const meta: Meta = {
  title: "Records/Leads/Today",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const lead = (over: Partial<Lead> = {}): Lead =>
  ({
    id: "l-1",
    full_name: "Jonas Petersen",
    company_name: "Nordwind Logistik",
    status: "contacted",
    score: 72,
    source: "manual",
    captured_by: "human:u1",
    version: 1,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...over,
  }) as Lead;

// The rows are a function rather than a component, so the story mounts the one
// host the screen mounts them in — a row drawn outside the panel's own rhythm
// would be a picture of something the product never draws.
function Today({
  over,
  refused,
}: Readonly<{ over?: Partial<Lead>; refused?: string }>) {
  const t = useT();
  const { locale } = useLocale();
  // The record's own zone, from the one module that names one: a task's
  // deadline is the team's day, and a story pinning a zone of its own would be
  // a second answer to which day that is.
  const recordZone = useRecordZone();
  return (
    <>
      <TodayPanel onOpenTasks={() => {}} tasksLabel={t("today.workQueue")}>
        {leadTodoRows(lead(over), t, locale, recordZone, () => {})}
      </TodayPanel>
      {/* The page's ONE sentence about why this lead takes no writes. It is
          rendered here because `reasonId` points at it: an id naming nothing
          would describe a refused control to nobody. */}
      {refused && <p id={refused}>{t("lead.terminalReadOnly")}</p>}
    </>
  );
}

const today = (over?: Partial<Lead>, refused?: string) => (
  <StoryProviders>
    <Today over={over} refused={refused} />
  </StoryProviders>
);

export const ImportedWithoutTask: Story = {
  render: () => today({ source: "import", status: "new" }),
};
export const PlannedOutreach: Story = {
  render: () =>
    today({
      next_task_subject: "Call selected prospect",
      next_task_due_at: "2026-09-19T09:00:00Z",
    }),
};
export const PlannedOutreachDark: Story = {
  globals: { theme: "dark" },
  render: PlannedOutreach.render,
};
export const ClosedDrawsNothing: Story = {
  render: () => today({ archived_at: "2026-09-01T00:00:00Z" }),
};
