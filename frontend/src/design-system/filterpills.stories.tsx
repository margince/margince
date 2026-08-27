// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { LocaleProvider } from "../i18n";
import { FilterPills } from "./filterpills";

// FilterPills: cuts THROUGH a list, each pill its own outlined object rather
// than one joined control, because each is a separate question about the list
// rather than a setting with one answer. The case worth seeing beside the
// ordinary counted row is the one the type makes possible on purpose: a pill
// whose count is absent, which is not a zero — it is a cut nobody has finished
// counting yet.
const meta: Meta<typeof FilterPills> = {
  title: "Design System/FilterPills",
  component: FilterPills,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof FilterPills>;

type Cut = "all" | "conversations" | "changes";

// Live, so the pressed pill can be judged as the reader's own choice rather
// than a fixed prop — `onChange` is the point of the row.
function Live(props: Readonly<{ initial: Cut }>) {
  const [value, setValue] = useState<Cut>(props.initial);
  return (
    <FilterPills
      label="Timeline"
      value={value}
      onChange={setValue}
      pills={[
        { value: "all", label: "All", count: 42 },
        { value: "conversations", label: "Conversations", count: 18 },
        // A paged read that hit `has_more`: the server knows the cut holds AT
        // LEAST this many, not exactly this many, so no figure is drawn.
        { value: "changes", label: "Changes" },
      ]}
    />
  );
}

// Every pill counted, the first cut pressed — the ordinary row.
export const Counted: Story = {
  render: () => <Live initial="all" />,
};

// The pressed pill is the one whose count is absent: the row still reads —
// nothing about being current depends on carrying a figure.
export const PressedPillHasNoCount: Story = {
  render: () => <Live initial="changes" />,
};
