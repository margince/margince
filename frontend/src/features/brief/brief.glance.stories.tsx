// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { WorklistItem } from "../../lib/worklisttypes";
import {
  firstWeek,
  readingsDay,
  taskRow,
  waitingEmailRow,
} from "../../testing/brief.fixtures";
import { StoryProviders } from "../../testing/story-utils";
import { BriefDials } from "./brief.dials";
import { BriefGlance } from "./brief.glance";

const meta: Meta<typeof BriefGlance> = {
  title: "Shell/Home/Agenda summary",
  component: BriefGlance,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof BriefGlance>;
const items = [waitingEmailRow(), taskRow("proposal", "Send revised pricing")];
const day = {
  ...readingsDay({}, items),
  focus: { items, total: items.length, urgent_remaining: 0 },
};
export const CompleteAgenda: Story = {
  args: {
    firstName: "Dana",
    now: new Date("2026-09-03T08:00:00Z"),
    day,
    week: undefined,
    view: "morning",
  },
};
export const CompleteAgendaDark: Story = {
  ...CompleteAgenda,
  globals: { theme: "dark" },
};
export const PartialAgenda: Story = {
  args: {
    ...CompleteAgenda.args,
    day: {
      ...day,
      sources_unavailable: [{ source: "task", reason: "failed" }],
    },
  },
};
export const MoreThanTheVisibleCards: Story = {
  args: {
    ...CompleteAgenda.args,
    day: { ...day, focus: { ...day.focus, total: 9 } },
  },
};

const noop = () => undefined;

// The facts line as the page draws it: the date, the agenda and the badge,
// with the view dial at its far end.
export const Briefed: Story = {
  args: {
    ...CompleteAgenda.args,
    briefedAt: "2026-09-03T05:30:00Z",
    controls: <BriefDials address={{ view: "morning" }} onChange={noop} />,
  },
};
export const BriefedPhone: Story = { ...Briefed, tags: ["uat-phone"] };

const categories: WorklistItem["category"][] = [
  "customer_waiting",
  "leads",
  "deals_at_risk",
  "meetings",
  "tasks",
  "decisions",
  "relationships",
];
const crowded = categories.flatMap((category) =>
  [1, 2].map((n) => ({
    ...taskRow(`${category}-${n}`, "Send revised pricing"),
    category,
  })),
);

// Every lane in the agenda: the line wraps and the dial keeps its place.
export const LongAgenda: Story = {
  args: {
    ...Briefed.args,
    day: {
      ...readingsDay({}, crowded),
      focus: { items: crowded, total: crowded.length, urgent_remaining: 0 },
    },
  },
};
export const LongAgendaPhone: Story = { ...LongAgenda, tags: ["uat-phone"] };

// The weekly's head: the report's week over its days, then when it was frozen
// and what the counts say.
export const Weekly: Story = {
  args: {
    ...CompleteAgenda.args,
    view: "weekly",
    week: firstWeek,
    controls: <BriefDials address={{ view: "weekly" }} onChange={noop} />,
  },
};
export const WeeklyPhone: Story = { ...Weekly, tags: ["uat-phone"] };
