// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { InfoTip } from "./infotip";
import { ProbabilityRing } from "./probabilityring";

const meta: Meta<typeof InfoTip> = {
  title: "Components/Overlays and layering/Info tip",
  component: InfoTip,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof InfoTip>;

const REASON =
  "Margince lowered this from the 50% that the stage Proposal gives.";

function render() {
  return (
    <p>
      <ProbabilityRing percent={35} />{" "}
      <InfoTip label="Why this probability" text={REASON} />
    </p>
  );
}

/** Beside a deal's probability, raised from the keyboard. */
export const Focused: Story = {
  render,
  play: async ({ canvasElement }) => {
    await userEvent.tab();
    const mark = within(canvasElement).getByRole("button", {
      name: "Why this probability",
    });
    await expect(mark).toHaveFocus();
    await expect(
      await within(document.body).findByRole("tooltip"),
    ).toHaveTextContent(REASON);
  },
};

/** The same in the dark theme. */
export const FocusedDark: Story = { ...Focused, globals: { theme: "dark" } };
