// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { CalendarDays } from "lucide-react";
import { ChipFace, chipClass } from "./chipface";
import { Chip } from "./readings";

// Check: a button drawn with the chip's classes and face matches the `Chip`
// beside it, edge, height and mark alike.

const meta: Meta = {
  title: "Components/Text and data display/Chip face",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

export const BesideAChip: Story = {
  render: () => (
    <div style={{ display: "flex", gap: "var(--space-2)" }}>
      <Chip dense icon={CalendarDays}>
        Due 01/09/2027
      </Chip>
      <button
        type="button"
        className={`${chipClass({ "chip-dense": true })} chip-link`}
      >
        <ChipFace mark={<CalendarDays size={12} aria-hidden="true" />}>
          Due 01/09/2027
        </ChipFace>
      </button>
    </div>
  ),
};
