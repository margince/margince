// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleProvider } from "../i18n";
import { type SelectAll, SelectAllControls } from "./listselection";
import { SelectionBar } from "./selectionbar";

// The end of a list's bulk bar: Select all and Clear selection. While Select
// all reads the pages it does not hold yet, the bar says how far it got.
const meta: Meta<typeof SelectAllControls> = {
  title: "Components/Text and data display/List selection",
  component: SelectAllControls,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <SelectionBar>
          <span className="t-caption">1 selected</span>
          <Story />
        </SelectionBar>
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof SelectAllControls>;

const resting: SelectAll = {
  page: { shown: true, checked: false, mixed: true, toggle: () => undefined },
  complete: false,
  atLimit: false,
  capped: false,
  pending: false,
  progress: undefined,
  run: () => undefined,
  clear: () => undefined,
  limit: 500,
};

export const Resting: Story = { args: { all: resting } };

// Select all is reading: the bar says how many rows it holds of the limit.
export const Reading: Story = {
  args: {
    all: { ...resting, pending: true, progress: { held: 150, of: 500 } },
  },
};

// The server sent no count, so the bar counts the rows without a target.
export const ReadingUncounted: Story = {
  args: {
    all: {
      ...resting,
      pending: true,
      progress: { held: 150, of: undefined },
    },
  },
};

// The limit is reached and more rows match: the bar says why it stopped.
export const AtTheLimit: Story = {
  args: { all: { ...resting, atLimit: true, capped: true } },
};
