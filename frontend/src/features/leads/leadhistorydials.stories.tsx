// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, userEvent, within } from "storybook/test";
import type { TimelineFilters } from "../../design-system/recordtimeline";
import type { TimelineFilter } from "../../lib/timelinefilter";
import { useLeadHistoryDials } from "./leadhistorydials";

function Dials({
  initial = "all",
  initialFilters = {},
}: Readonly<{ initial?: TimelineFilter; initialFilters?: TimelineFilters }>) {
  const [filter, onFilter] = useState<TimelineFilter>(initial);
  const [filters, onFilters] = useState<TimelineFilters>(initialFilters);
  const dials = useLeadHistoryDials({ filter, onFilter, filters, onFilters });
  return (
    <div>
      {dials.cuts}
      {dials.narrowing}
    </div>
  );
}

const meta: Meta<typeof Dials> = {
  title: "Records/Leads/History cuts",
  component: Dials,
};
export default meta;
type Story = StoryObj<typeof Dials>;

/** The whole record: the search toggle, then the cut the list shows. */
export const All: Story = {};

/** The cuts open: the whole record, one kind of exchange, or what changed. */
export const CutsOpen: Story = {
  play: async ({ canvasElement }) => {
    const cuts = await within(canvasElement).findByRole("combobox", {
      name: "Timeline filter",
    });
    await userEvent.click(cuts);
    const list = await screen.findByRole("listbox");
    await expect(within(list).getAllByRole("option")).toHaveLength(6);
    await expect(cuts).toHaveAttribute("aria-expanded", "true");
  },
};

/** Changes has no exchanges to narrow, so the search toggle stands down. */
export const Changes: Story = { args: { initial: "changes" } };

export const CallsDark: Story = {
  args: { initialFilters: { kind: "call" } },
  globals: { theme: "dark" },
};

export const Phone: Story = {
  args: { initialFilters: { kind: "meeting" } },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const PhoneDark: Story = {
  ...Phone,
  globals: { viewport: { value: "phone" }, theme: "dark" },
};
