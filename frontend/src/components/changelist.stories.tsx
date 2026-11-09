// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { StoryProviders } from "../testing/story-utils";
import { ChangeList } from "./changelist";

// The fields one change moved, each value read the way the server typed it.
const meta: Meta<typeof ChangeList> = {
  title: "Patterns/Change list",
  component: ChangeList,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof ChangeList>;

const STAGE_ID = "0c000000-0000-4000-8000-000000000001";

/** Money in its currency, a stage by its name, a day as a date. */
export const TypedValues: Story = {
  args: {
    changes: [
      {
        field: "amount_minor",
        type: "money",
        currency: "EUR",
        before: 1_250_000,
        after: 3_000_000,
      },
      {
        field: "stage_id",
        type: "stage",
        before: STAGE_ID,
        after: "0c000000-0000-4000-8000-000000000002",
        before_label: "Qualified",
        after_label: "Closed lost",
      },
      {
        field: "expected_close_date",
        type: "date",
        precision: "day",
        before: "2026-09-30",
        after: "2026-10-31",
      },
    ],
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Stage")).toBeVisible();
    await expect(canvas.getByText("Qualified → Closed lost")).toBeVisible();
    const text = canvasElement.textContent ?? "";
    await expect(text).toContain("€12,500.00");
    await expect(text).toContain("30/09/2026");
    await expect(text).not.toContain("1250000");
  },
};

export const TypedValuesDark: Story = {
  ...TypedValues,
  globals: { theme: "dark" },
};

/**
 * Values the reader may not see, or that only an id or minor units could
 * name, read as "hidden"; an absent one reads as "empty".
 */
export const Hidden: Story = {
  args: {
    changes: [
      { field: "owner_id", type: "member", before: STAGE_ID, after: null },
      { field: "lost_reason", type: "enum", before: null, after: "silent" },
      { field: "amount_minor", before: 100, after: 200 },
    ],
  },
  play: async ({ canvasElement }) => {
    const text = canvasElement.textContent ?? "";
    await expect(text).toContain("hidden");
    await expect(text).toContain("empty");
    await expect(text).not.toContain(STAGE_ID);
    await expect(text).not.toContain("silent");
    await expect(text).not.toMatch(/\b(100|200)\b/);
  },
};

export const HiddenDark: Story = { ...Hidden, globals: { theme: "dark" } };
