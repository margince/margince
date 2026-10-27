// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import type { StageStep } from "./stageladder";
import { LadderRun } from "./stageladder.run";
import "./stageladder.css";

// The scrolling run both ladder shapes draw, alone: where it comes to rest.
const meta: Meta<typeof LadderRun> = {
  title: "Components/Forms and input/Stage ladder run",
  component: LadderRun,
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

type Story = StoryObj<typeof LadderRun>;

const noop = () => undefined;

// Stage names have no length limit, so a current one can outgrow the run.
const LONG_NAME =
  "Commercial and technical due diligence with the buying committee";

const steps: StageStep[] = [
  "Qualified",
  "Discovery",
  "Proposal",
  LONG_NAME,
  "Negotiation",
  "Signature",
].map((label, index) => ({
  key: label,
  label,
  done: index < 3,
  current: index === 3,
  onPick: noop,
}));

/**
 * A current stage wider than the run. Centred, its name would start and end
 * under the two fades; the run rests on its start, just past the start fade.
 */
export const CurrentStageWiderThanTheRun: Story = {
  render: () => (
    <div style={{ inlineSize: 360 }}>
      <LadderRun label="Stage" steps={steps}>
        <ol className="stage-ladder-steps">
          {steps.map((step) => (
            <li
              key={step.key}
              className="stage-ladder-step"
              aria-current={step.current ? "step" : undefined}
            >
              <span className="stage-ladder-label">{step.label}</span>
            </li>
          ))}
        </ol>
      </LadderRun>
    </div>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const run = await canvas.findByRole("region", { name: "Stage" });
    await canvas.findByRole("button", { name: "Show earlier stages" });
    const here = canvas.getByText(LONG_NAME).closest("li");
    if (!here) {
      throw new Error("The current stage drew no rung.");
    }
    const room = Number.parseFloat(
      getComputedStyle(run).scrollPaddingInlineStart,
    );
    await expect(here.getBoundingClientRect().width).toBeGreaterThan(
      run.clientWidth,
    );
    await waitFor(() =>
      expect(
        Math.abs(
          here.getBoundingClientRect().left -
            (run.getBoundingClientRect().left + room),
        ),
      ).toBeLessThanOrEqual(1),
    );
  },
};
