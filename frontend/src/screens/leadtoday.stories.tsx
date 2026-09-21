// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { useLocale, useT } from "../i18n";
import { leadTodoRows } from "./leadtoday";
import { TodayPanel } from "./record360/today";
import { StoryProviders } from "./story-utils";

// What a lead asks of a rep today, in the panel that hosts it. Two rows and no
// more: answer the lead if nobody has, and the next task if it carries one. The
// states worth looking at are the CLOCKS — a first response owed, one at risk,
// one already breached — because the row's tone is the only thing that tells
// them apart at a glance.

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
        {leadTodoRows(
          lead(over),
          t,
          locale,
          recordZone,
          () => {},
          () => {},
          refused,
        )}
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

/** Nobody has answered yet, and the clock has not started: owed, undated. */
export const AnswerOwed: Story = { render: () => today() };

/** The clock is running and still has room — the caution before the fact. */
export const FirstResponseAtRisk: Story = {
  render: () =>
    today({ sla_state: "at_risk", sla_deadline_at: "2026-09-20T09:00:00Z" }),
};

/** The clock ran out. Danger, because there is nothing left to stop. */
export const FirstResponseBreached: Story = {
  render: () =>
    today({ sla_state: "breached", sla_deadline_at: "2026-09-10T09:00:00Z" }),
};

/**
 * A lead the reader may not write to. The Reply verb keeps its place and points
 * at the page's one sentence — a press that did nothing would say less than a
 * refusal that says why.
 */
export const ReplyRefused: Story = {
  render: () => today({}, "lead-read-only"),
};

/** Both rows: the answer owed, and the task the lead already carries. */
export const AnswerAndTask: Story = {
  render: () =>
    today({
      next_task_subject: "Send the fleet pricing sheet",
      next_task_due_at: "2026-09-19T09:00:00Z",
    }),
};

/** A closed lead is not worked, so it draws no rows at all. */
export const ClosedDrawsNothing: Story = {
  render: () => today({ archived_at: "2026-09-01T00:00:00Z" }),
};

export const FirstResponseBreachedDark: Story = {
  globals: { theme: "dark" },
  render: () =>
    today({ sla_state: "breached", sla_deadline_at: "2026-09-10T09:00:00Z" }),
};
