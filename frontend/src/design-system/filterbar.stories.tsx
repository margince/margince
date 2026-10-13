// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button, Field, SegmentedControl } from "./atoms";
import { FilterBar } from "./filterbar";
import { Select } from "./select";

// A page's dials on one card: filters leading, the page's verbs trailing, and
// the line saying what the cut covers underneath.

const meta: Meta<typeof FilterBar> = {
  title: "Components/Layout and structure/Filter bar",
  component: FilterBar,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof FilterBar>;

const period = (
  <Field label="Date range">
    {(field) => (
      <Select
        {...field}
        value="this_month"
        options={[
          { value: "this_month", label: "This month" },
          { value: "last_month", label: "Last month" },
        ]}
        onChange={() => {}}
      />
    )}
  </Field>
);

export const WithActionsAndCaption: Story = {
  render: () => (
    <FilterBar
      label="Filters"
      actions={
        <>
          <Button>Export CSV</Button>
          <Button>Save report</Button>
        </>
      }
      caption="Results through 22 Sept 2026, 14:00"
    >
      <SegmentedControl
        label="View"
        options={["sales", "sdr"]}
        value="sales"
        labels={{ sales: "Sales", sdr: "SDR outcomes" }}
        onChange={() => {}}
      />
      {period}
    </FilterBar>
  ),
};

// Dials alone: no verbs, no caption, and no empty slots left for them.
export const DialsOnly: Story = {
  render: () => <FilterBar label="Filters">{period}</FilterBar>,
};

export const Phone: Story = {
  ...WithActionsAndCaption,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
