// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { LocaleProvider } from "../i18n";
import { ChipValueList } from "./listfiltervalues";
import { type ListChip, Menu } from "./listsurface";

// THE VALUE STEP of one filter, in the two shapes an attribute can take. It is
// the body of a menu and nothing else — the Filter button opens it as its
// second step, and an applied row's value segment opens the same list — so
// every story here draws it inside the `Menu` it lives in rather than loose on
// the page, where it would have no box and no name.

const meta: Meta = {
  title: "Design System/Filter values",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        {/* The toolbar's own anchor. Its height is room for the panel, which
            hangs out of the flow and would otherwise run off the canvas. */}
        <span className="lt-menu-wrap" style={{ blockSize: "20rem" }}>
          <Story />
        </span>
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj;

/** An attribute small enough to list whole: every value is on screen. */
const industry: ListChip = {
  key: "industry",
  label: "Industry",
  allLabel: "All industries",
  options: [
    { value: "manufacturing", label: "Manufacturing" },
    { value: "logistics", label: "Logistics" },
    { value: "healthcare", label: "Healthcare" },
    { value: "saas", label: "SaaS" },
  ],
};

const COMPANIES = [
  { value: "c-1", label: "Acme Industrie GmbH" },
  { value: "c-2", label: "Brandt Automotive GmbH" },
  { value: "c-3", label: "Nordwind Logistik AG" },
];

/**
 * A relation too large to list whole, so the step searches instead.
 *
 * Resolved from a local set: a story has no server, and a `search` that waited
 * on one would document a spinner rather than the list. `options` stays
 * declared because the "all" entry still clears the filter.
 */
const company: ListChip = {
  key: "company_id",
  label: "Company",
  allLabel: "Any company",
  options: [],
  search: (query) =>
    Promise.resolve(
      COMPANIES.filter((candidate) =>
        candidate.label.toLowerCase().includes(query.toLowerCase()),
      ),
    ),
};

/**
 * The step with the state a menu would hold around it, so picking a value
 * moves the tick rather than leaving a list nobody can answer.
 */
function Values({
  chip,
  picked = "",
}: Readonly<{ chip: ListChip; picked?: string }>) {
  const [value, setValue] = useState(picked);
  return (
    <Menu open head={chip.label}>
      <ChipValueList chip={chip} value={value} onPick={setValue} />
    </Menu>
  );
}

// One value per filter, so the entries are RADIOS: the list endpoints take one
// value per param, and a column of boxes would promise a combination the API
// cannot answer.
export const Picked: Story = {
  render: () => <Values chip={industry} picked="logistics" />,
};

// Nothing chosen, which is not an empty state: the "all" entry is what the
// filter reads as when it is off, and it is also how a reader clears one.
export const NothingPicked: Story = {
  render: () => <Values chip={industry} />,
};

// The searched variant, as a reader meets it. A workspace has more companies
// than a menu can hold, so the fixed options give way to a box — and the line
// under it says which of the four states the search is in rather than showing
// an empty list, which would read as a confident "there are none".
export const Searched: Story = {
  render: () => <Values chip={company} />,
};
