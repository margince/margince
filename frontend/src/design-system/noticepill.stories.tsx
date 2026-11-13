// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { NoticePill } from "./noticepill";

const meta: Meta<typeof NoticePill> = {
  title: "Components/Labels/Notice pill",
  component: NoticePill,
  parameters: { layout: "padded" },
  args: { onClick: fn(), opensDialog: true },
};
export default meta;

type Story = StoryObj<typeof NoticePill>;

/** Calm: the status holds, and the press is there for whoever wants it. */
export const Calm: Story = {
  args: { tone: "success", children: "Trial · 10 days left" },
};

/** A status that needs acting on soon. */
export const Closing: Story = {
  args: { tone: "warning", children: "6 days left of your trial" },
};

/** A status past its limit. */
export const Ended: Story = {
  args: { tone: "danger", children: "Trial ended · Contact Margince" },
};
