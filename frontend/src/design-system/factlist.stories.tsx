// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { Badge, Card } from "./atoms";
import { FactList } from "./factlist";

// FactList's own node. It had a story, but it lived inside
// `callout.stories.tsx`, so the one place a reader looks for a primitive — the
// sidebar entry with its name on it — had nothing under F, and the README's ✅
// pointed at a page about a different component.
const meta: Meta<typeof FactList> = {
  title: "Components/Text and data display/Fact list",
  component: FactList,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <div style={{ maxWidth: "26rem" }}>
          <Story />
        </div>
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof FactList>;

export const Facts: Story = {
  args: {
    facts: [
      { key: "in", term: "Last inbound", value: "3 Feb 2026" },
      { key: "out", term: "Last outbound", value: "Never" },
      { key: "owner", term: "Owner", value: "Anna Brandt" },
    ],
  },
};

// `numeric` sets tabular figures, which is the difference between a column of
// amounts that lines up and one that shifts as digits change width.
export const NumericWithNotes: Story = {
  args: {
    numeric: true,
    facts: [
      { key: "arr", term: "Committed ARR", value: "€148,000.00" },
      {
        key: "spend",
        term: "Spend this month",
        value: "€1,204.00",
        note: "Partial — 12 of 28 days counted",
      },
      {
        key: "open",
        term: "Open deals",
        value: "3",
        note: "Weighted €61,400.00",
      },
    ],
  },
  // A note is a caption under its figure, so it must not read as a second value.
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const value = getComputedStyle(canvas.getByText("€1,204.00"));
    const note = getComputedStyle(
      canvas.getByText("Partial — 12 of 28 days counted"),
    );
    await expect(note.color).not.toBe(value.color);
    await expect(Number.parseFloat(note.fontSize)).toBeLessThan(
      Number.parseFloat(value.fontSize),
    );
  },
};

export const NumericWithNotesPhone: Story = {
  ...NumericWithNotes,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

// Both sides take nodes, which is what the real rows need: a status pill on the
// value side, and an unknown spelled out in words rather than left blank — an
// empty value claims we know the fact and it is nothing.
export const RichValues: Story = {
  args: {
    facts: [
      {
        key: "status",
        term: "Mailbox",
        value: <Badge tone="warning">Reconnect needed</Badge>,
      },
      { key: "size", term: "Employees", value: "Not recorded" },
      {
        key: "seat",
        term: "Seat",
        value: <Badge tone="accent">Full</Badge>,
        note: "Granted 4 March by Ops",
      },
    ],
  },
};

// In its usual home — inside a card, under a heading, as the block a reader
// scans before reading anything else on the page.
export const InACard: Story = {
  render: (args) => (
    <Card title="At a glance">
      <FactList {...args} />
    </Card>
  ),
  args: {
    facts: [
      { key: "hq", term: "Headquarters", value: "Munich" },
      { key: "since", term: "Customer since", value: "March 2023" },
      { key: "renewal", term: "Renewal", value: "11 days" },
    ],
  },
};

/** The list inside a record's details card: a fixed term column, no rules
 *  between rows, and a card-valued fact with its term above it. */
export const Flat: Story = {
  decorators: [
    (Story) => (
      <div style={{ maxWidth: "18.25rem" }}>
        <Story />
      </div>
    ),
  ],
  args: {
    flat: true,
    facts: [
      { key: "domain", term: "Domain", value: "brandt.example" },
      { key: "industry", term: "Industry", value: "Automotive" },
      {
        key: "way-in",
        term: "Way in",
        value: <Badge tone="success">Economic buyer</Badge>,
        block: true,
      },
      {
        key: "address",
        term: "Address",
        value: "Leopoldstraße 148, 80804 München, Germany",
      },
    ],
  },
};
