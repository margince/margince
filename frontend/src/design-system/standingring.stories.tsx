// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { HealthRing, ScoreRing, StrengthRing } from "./standingring";

const meta: Meta<typeof HealthRing> = {
  title: "Components/Status indicators/Reading ring",
  component: HealthRing,
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

type Story = StoryObj<typeof HealthRing>;

/** A company with one of its three dimensions rated, and that one amber. */
export const HealthOneRated: Story = {
  render: () => (
    <HealthRing
      word="Watch"
      tone="warning"
      arcs={[
        { key: "relationship", label: "Relationship", rating: "good" },
        { key: "commercial", label: "Commercial" },
        { key: "payment", label: "Payment" },
      ]}
    />
  ),
  play: async ({ canvasElement }) => {
    const ring = within(canvasElement).getByRole("img");
    await expect(ring).toHaveAccessibleName("Watch. 1 of 3 rated");
  },
};

/** All three rated: the ring shows which one holds the verdict down. */
export const HealthAllRated: Story = {
  render: () => (
    <HealthRing
      word="At risk"
      tone="danger"
      arcs={[
        { key: "relationship", label: "Relationship", rating: "strong" },
        { key: "commercial", label: "Commercial", rating: "good" },
        { key: "payment", label: "Payment", rating: "at_risk" },
      ]}
    />
  ),
};

/** A new company: nothing rated, bare track, and the centre says so. */
export const HealthNotRated: Story = {
  render: () => (
    <HealthRing
      word="Not rated"
      tone="neutral"
      arcs={[
        { key: "relationship", label: "Relationship" },
        { key: "commercial", label: "Commercial" },
        { key: "payment", label: "Payment" },
      ]}
    />
  ),
};

/** A contact's relationship strength, in its band's colour. */
export const StrengthModerate: Story = {
  render: () => (
    <StrengthRing
      score={41}
      band="moderate"
      bandLabel="moderate"
      detail="Last contact 19 Oct"
      explain="Recency 80 %, frequency 40 %."
    />
  ),
};

export const StrengthStrong: Story = {
  render: () => (
    <StrengthRing
      score={72}
      band="strong"
      bandLabel="strong"
      detail="Last contact 19 Oct"
      explain="Recency 80 %, frequency 40 %."
    />
  ),
};

/** Never an interaction: no zero, the words instead. */
export const StrengthNoInteractions: Story = {
  render: () => (
    <StrengthRing
      score={0}
      band="none"
      bandLabel="none"
      detail="Last contact 19 Oct"
      explain="Recency 80 %, frequency 40 %."
    />
  ),
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).getByRole("img")).toHaveAccessibleName(
      "No interactions yet",
    );
  },
};

/** A lead's score: the number itself, because points are the scale. */
export const ScoreHigh: Story = {
  render: () => (
    <ScoreRing
      score={82}
      tone="success"
      label="Score 82 of 100"
      caption="of 100"
    />
  ),
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).getByRole("img")).toHaveAccessibleName(
      "Score 82 of 100",
    );
  },
};

/** A share of a target, compact beside a lead figure, with the expected
    finish as a lighter arc ahead of it. */
export const ScoreTargetShare: Story = {
  render: () => (
    <ScoreRing
      size="compact"
      score={53}
      reach={95}
      display="53%"
      tone="warning"
      label="Target reached: €420k won of €800k"
      caption="Target"
    />
  ),
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).getByRole("img")).toHaveTextContent(
      "53%Target",
    );
  },
};

export const ScoreTargetShareDark: Story = {
  ...ScoreTargetShare,
  globals: { theme: "dark" },
};
