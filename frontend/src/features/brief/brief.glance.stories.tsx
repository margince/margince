// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  readingsDay,
  taskRow,
  waitingEmailRow,
} from "../../testing/brief.fixtures";
import { StoryProviders } from "../../testing/story-utils";
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
