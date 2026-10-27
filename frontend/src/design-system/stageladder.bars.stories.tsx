// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import type { StageStep } from "./stageladder";
import { LadderBars } from "./stageladder.bars";
import "./stageladder.css";

// The bars shape of the stage ladder alone, as How it stands draws it.
const meta: Meta<typeof LadderBars> = {
  title: "Components/Forms and input/Stage ladder bars",
  component: LadderBars,
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

type Story = StoryObj<typeof LadderBars>;

const noop = () => undefined;

const steps: StageStep[] = [
  ...[
    "Potential Deal",
    "Deal Qualification",
    "Solution Discovery",
    "Conversion Strategy",
    "Proposal Sent",
    "Contract Negotiation",
    "Contract Review",
  ].map((label, index) => ({
    key: label,
    label,
    done: index < 3,
    current: index === 3,
    onPick: noop,
  })),
  { key: "won", label: "Closed Won", terminal: true, onPick: noop },
  { key: "lost", label: "Closed Lost", terminal: true, onPick: noop },
];

/**
 * Seven open stages in a card too narrow for their names. No bar gives up
 * its name; the run scrolls, with a button at each end that hides a bar.
 */
export const LongRunInANarrowCard: Story = {
  render: () => (
    <div style={{ inlineSize: 420 }}>
      <LadderBars label="Stage" steps={steps} tone="neutral" />
    </div>
  ),
  play: async ({ canvasElement }) => {
    const ladder = within(
      await within(canvasElement).findByRole("group", { name: "Stage" }),
    );
    await expect(
      await ladder.findByRole("button", { name: "Show later stages" }),
    ).toBeVisible();
    for (const name of canvasElement.querySelectorAll<HTMLElement>(
      ".stage-ladder-bars .stage-ladder-label",
    )) {
      await expect(name.scrollWidth).toBeLessThanOrEqual(name.clientWidth);
    }
  },
};
