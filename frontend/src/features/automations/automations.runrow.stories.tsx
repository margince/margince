// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { StoryProviders } from "../../testing/story-utils";
import { TRACED_RUNS } from "./automations.fixtures";
import { RunRow } from "./automations.runrow";

// One run of the history, in each shape a server sends it.

const meta: Meta<typeof RunRow> = {
  title: "Settings/Data/Automations/Run",
  component: RunRow,
};
export default meta;
type Story = StoryObj<typeof RunRow>;

function runs(locale: "en" | "de" = "en") {
  return (
    <StoryProviders locale={locale}>
      <ul className="auto-run-list">
        {TRACED_RUNS.map((run) => (
          <RunRow key={run.id} run={run} />
        ))}
      </ul>
    </StoryProviders>
  );
}

export const EveryShape: Story = {
  render: () => runs(),
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    const canvas = within(canvasElement);
    await expect(
      canvas.getByText(/Waiting for approval: a draft email on/),
    ).toBeVisible();
    await expect(
      canvas.getByText("Sent a notification about a deal"),
    ).toBeVisible();
    const [details] = canvas.getAllByText("Technical details");
    if (!details) throw new Error("the traced run drew no technical details");
    await user.click(details);
    await expect(
      await canvas.findByText("01a1e5f1-7a5c-7000-8000-0000000000e1"),
    ).toBeVisible();
  },
};

export const EveryShapeDark: Story = {
  globals: { theme: "dark" },
  render: () => runs(),
};

export const EveryShapeGerman: Story = { render: () => runs("de") };
