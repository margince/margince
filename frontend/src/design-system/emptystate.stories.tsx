// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";
import { Button, EmptyState } from "./atoms";

const meta: Meta<typeof EmptyState> = {
  title: "Components/Messaging/Empty state",
  component: EmptyState,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof EmptyState>;

const stack: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "1rem",
};

export const ThreeFaces: Story = {
  render: () => (
    <div style={stack}>
      <EmptyState>No deals match these filters yet.</EmptyState>
      <EmptyState
        title="No projects yet"
        action={<Button variant="primary">New project</Button>}
      >
        <p>
          A project is the body of work a deal is about. It starts during the
          deal, in the initiative phase, and outlives close-won: delivery is
          tracked here after the pipeline has let go.
        </p>
      </EmptyState>
      {/* Dashed because the space is waiting rather than broken; its verb
          lives in the group's head. */}
      <EmptyState plate title="No open deals">
        A deal is a sale in progress on this account, with its stage and its
        expected close.
      </EmptyState>
    </div>
  ),
};
