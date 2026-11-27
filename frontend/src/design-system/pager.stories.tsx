// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleProvider } from "../i18n";
import { Pager } from "./pager";
import "./listtable.css";

// The footer of a list: what the strip of page numbers says in each state of
// what the caller knows about the list's length.
const meta: Meta<typeof Pager> = {
  title: "Components/Text and data display/Pager",
  component: Pager,
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

type Story = StoryObj<typeof Pager>;

const shared = {
  perPage: 25,
  onGoto: () => undefined,
  onPerPage: () => undefined,
};

// The server counted the list, so the last page is numbered though only the
// first two are in hand.
export const Counted: Story = {
  args: { ...shared, current: 1, lastPage: 175, hasMore: true, endKnown: true },
};

export const CountedMiddle: Story = {
  args: {
    ...shared,
    current: 90,
    lastPage: 175,
    hasMore: true,
    endKnown: true,
  },
};

// The reader pressed the last page; its number waits while the rows are read.
export const AskedPage: Story = {
  args: {
    ...shared,
    current: 6,
    lastPage: 175,
    hasMore: true,
    endKnown: true,
    asked: 175,
  },
};

// No count: Previous and Next alone, since the pages in hand are not the list.
export const NotCounted: Story = {
  args: { ...shared, current: 2, lastPage: 2, hasMore: true },
};

// Every row is in hand, so the pages are all there are.
export const EveryRowInHand: Story = {
  args: { ...shared, current: 4, lastPage: 9, hasMore: false },
};
