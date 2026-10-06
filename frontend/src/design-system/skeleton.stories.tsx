// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Card, Skeleton } from "./atoms";

const meta: Meta<typeof Skeleton> = {
  title: "Components/Loading/Skeleton",
  component: Skeleton,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof Skeleton>;

export const InACard: Story = {
  render: () => (
    <Card>
      <div style={{ display: "flex", flexDirection: "column", gap: "0.6rem" }}>
        <Skeleton width="40%" height={18} />
        <Skeleton width="100%" />
        <Skeleton width="86%" />
        <Skeleton width={140} height={10} />
      </div>
    </Card>
  ),
};
