// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";
import { STATUS_KINDS, StatusBadge, type StatusKind } from "./statusbadge";

const meta: Meta<typeof StatusBadge> = {
  title: "Components/Labels/Status badge",
  component: StatusBadge,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof StatusBadge>;

const row: CSSProperties = {
  display: "flex",
  gap: "0.75rem",
  alignItems: "center",
  flexWrap: "wrap",
};
const stack: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "1rem",
};

// The labels screens put on each kind today, so a reader sees where each one goes.
const EXAMPLES: Readonly<Record<StatusKind, readonly string[]>> = {
  done: ["Connected", "Imported", "Fulfilled"],
  neutral: ["Waiting", "Invited", "Deactivated", "Lost", "Rejected"],
  attention: ["Needs attention", "Needs a key", "Stopped"],
  failure: ["Failed", "Sync broken"],
};

const docs = (story: string) => ({ docs: { description: { story } } });

export const AllKinds: Story = {
  parameters: docs(`A status is always this badge, in one of four kinds.
- **done** (green): it worked, nothing to do.
- **neutral** (grey): waiting, invited, deactivated, and results like Lost or
  Rejected. A result is not a failure.
- **attention** (amber): someone must act.
- **failure** (red): something broke.
- Blue never means status.`),
  render: () => (
    <div style={stack}>
      {STATUS_KINDS.map((kind) => (
        <div key={kind} style={row}>
          {EXAMPLES[kind].map((label) => (
            <StatusBadge key={label} kind={kind} label={label} />
          ))}
        </div>
      ))}
    </div>
  ),
};

export const AllKindsDark: Story = { ...AllKinds, globals: { theme: "dark" } };

export const Live: Story = {
  parameters: docs(
    "Running or syncing: neutral with the live dot, never blue. The dot stops under reduced motion.",
  ),
  render: () => (
    <div style={row}>
      <StatusBadge kind="neutral" label="Syncing" live />
      <StatusBadge kind="neutral" label="Running" live />
    </div>
  ),
};
