// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { ReadingsFloor } from "./readingsfloor";

// The caveat on its own. It is a sentence about the read rather than about any
// figure. It draws quieter than the readings it sits under and never competes
// with them.
const meta: Meta<typeof ReadingsFloor> = {
  title: "Components/Text and data display/Readings floor",
  component: ReadingsFloor,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof ReadingsFloor>;

export const TheFloorAlone: Story = {
  render: () => (
    <ReadingsFloor>
      Read to the page limit, so these are floors, not totals.
    </ReadingsFloor>
  ),
};
