// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";

import { TAG_TONES, TagPill } from "./tagpill";

// A tag's colour is not a status. The tones exist so one word is tellable from
// another at a glance, which is why they ride as a dot rather than as a fill —
// a strip of filled pills reads as a stripe of blocks, and the words stop being
// the thing you read.

const meta: Meta<typeof TagPill> = {
  title: "Design System/TagPill",
  component: TagPill,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof TagPill>;

/** Every tone an admin picks from, side by side, which is how a reader actually
 * meets them: as a strip on one record. Driven off TAG_TONES rather than a list
 * typed out here, so a tone added to the palette cannot go missing from the one
 * view that exists to prove the tones are tellable apart. */
export const EveryTone: Story = {
  render: () => (
    <div style={{ display: "flex", gap: "var(--space-2)", flexWrap: "wrap" }}>
      {TAG_TONES.map((tone) => (
        <TagPill key={tone} name={tone} tone={tone} />
      ))}
    </div>
  ),
};

/** A tag with no colour is a tag. The dot is absent rather than drawn in some
 * default hue, which would claim a distinction the admin did not make. */
export const NoTone: Story = {
  render: () => <TagPill name="Uncoloured" />,
};

/**
 * An archived word stays ON the record it was applied to: retiring a tag stops
 * it being applied, it does not un-tag history. It draws quiet and says so,
 * because a reader who saw it plain would go looking for it in a picker that
 * no longer offers it.
 */
export const Archived: Story = {
  render: () => <TagPill name="Trade Fair 2025" tone="amber" archived />,
};

/**
 * A colour outside the palette means the server's list and this one have
 * drifted. The pill draws no dot rather than an unstyled one — a broken swatch
 * beside a name is a defect a reader has to interpret.
 */
export const ToneOutsideThePalette: Story = {
  render: () => <TagPill name="Drifted" tone="chartreuse" />,
};
