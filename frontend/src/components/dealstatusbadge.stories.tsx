// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { type Locale, LocaleProvider } from "../i18n";
import { DealStatusBadge } from "./dealstatusbadge";

const meta = {
  title: "Records/Deals/Status badge",
  component: DealStatusBadge,
  parameters: { layout: "padded" },
  args: { status: "open" },
} satisfies Meta<typeof DealStatusBadge>;
export default meta;

type Story = StoryObj<typeof meta>;

function Statuses({ locale }: Readonly<{ locale: Locale }>) {
  return (
    <LocaleProvider initial={locale}>
      <span style={{ display: "flex", gap: "var(--space-2)" }}>
        <DealStatusBadge status="open" />
        <DealStatusBadge status="won" />
        <DealStatusBadge status="lost" />
      </span>
    </LocaleProvider>
  );
}

export const AllThree: Story = {
  render: () => <Statuses locale="en" />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByText("Open")).toBeVisible();
    await expect(canvas.getByText("Won")).toBeVisible();
    await expect(canvas.getByText("Lost")).toBeVisible();
  },
};

// A Vietnamese reader meets the catalog's words, never the stored value.
export const Vietnamese: Story = {
  render: () => <Statuses locale="vi" />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByText("Đang mở")).toBeVisible();
    await expect(canvas.getByText("Thắng")).toBeVisible();
    await expect(canvas.getByText("Thua")).toBeVisible();
    await expect(canvas.queryByText("open")).toBeNull();
  },
};
