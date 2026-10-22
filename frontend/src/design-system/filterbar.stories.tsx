// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button, Field, SearchField, SegmentedControl } from "./atoms";
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

const dial = (label: string, value: string, choices: readonly string[]) => (
  <Field label={label}>
    {(field) => (
      <Select
        {...field}
        value={value}
        options={choices.map((choice) => ({ value: choice, label: choice }))}
        onChange={() => {}}
      />
    )}
  </Field>
);

// A log's bar in the 720-wide settings column. A search and four dials wrap
// onto a second line. Clear holds the trailing edge once a dial is set. The
// caption counts what the dials let through.
export const InASettingsColumn: Story = {
  render: () => (
    <div style={{ maxWidth: 720 }}>
      <FilterBar
        label="Filters"
        actions={<Button>Clear filters</Button>}
        caption="1 to 25 of 25 calls loaded"
      >
        <Field label="Search calls">
          {(field) => <SearchField {...field} value="" onChange={() => {}} />}
        </Field>
        {dial("Feature", "Message classification", [
          "All features",
          "Message classification",
        ])}
        {dial("When", "Last day", ["Any time", "Last day"])}
        {dial("Outcome", "Any outcome", ["Any outcome", "Failed"])}
        {dial("Model", "Any model", ["Any model", "gemini-2.5-flash"])}
      </FilterBar>
    </div>
  ),
};

export const InASettingsColumnPhone: Story = {
  ...InASettingsColumn,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
