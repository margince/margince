// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "../design-system/atoms";
import { PageHeadAction } from "./pageheadaction";

// The verb on its own, where no shell head stands around it: it draws in
// place. "Shell/Navigation shell" shows it in a settings page's head, wide and
// on a phone.

const meta = {
  title: "Shell/Page head action",
  component: PageHeadAction,
  args: { children: <Button>New team</Button> },
} satisfies Meta<typeof PageHeadAction>;
export default meta;
type Story = StoryObj<typeof meta>;

export const InPlace: Story = {};

export const TwoVerbs: Story = {
  args: {
    children: (
      <>
        <Button>Import</Button>
        <Button>New team</Button>
      </>
    ),
  },
};
