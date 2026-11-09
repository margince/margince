// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { useT } from "../../i18n";
import { StoryProviders } from "../../testing/story-utils";
import { DispositionButton } from "./worklist.verbbutton";

// One judgement as a button. A company question's Done defers the row, so
// its button names that: Not now, with a hint that new mail brings it back.

function Pair() {
  const t = useT();
  return (
    <div className="worklist-row-acts">
      <DispositionButton
        disposition="handled"
        source="task"
        writing={undefined}
        onPress={() => {}}
        t={t}
      />
      <DispositionButton
        disposition="handled"
        source="domain_question"
        writing={undefined}
        onPress={() => {}}
        t={t}
      />
    </div>
  );
}

const meta: Meta<typeof DispositionButton> = {
  title: "Records/Worklist/Disposition button",
  component: DispositionButton,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof DispositionButton>;

export const DoneAndNotNow: Story = {
  render: () => (
    <StoryProviders>
      <Pair />
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("button", { name: "Done" })).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Not now" })).toBeVisible();
  },
};
export const DoneAndNotNowDark: Story = {
  ...DoneAndNotNow,
  globals: { theme: "dark" },
};
