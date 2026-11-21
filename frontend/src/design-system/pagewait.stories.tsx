// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { type ReactNode, useState } from "react";
import { Button } from "./atoms";
import { PageWait, usePageWait } from "./pagewait";

// The line hangs from the foot of the bar above it, over the page it covers.
const meta = {
  title: "Components/Loading/Page wait",
  component: PageWait,
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => (
      <div style={{ background: "var(--bgPage)", minBlockSize: 160 }}>
        <div
          style={{
            blockSize: 48,
            borderBlockEnd: "1px solid var(--borderSubtle)",
          }}
        />
        <Story />
        <p style={{ padding: "var(--space-4)" }}>The page still on screen.</p>
      </div>
    ),
  ],
} satisfies Meta<typeof PageWait>;
export default meta;

type Story = StoryObj<typeof meta>;

/** A move under 200 ms: the status region is there and empty, and no line shows. */
export const Hidden: Story = {
  args: { waiting: false, label: "Loading Account…" },
};

export const Showing: Story = {
  args: { waiting: true, label: "Loading Account…" },
};

/** Under reduced motion the line stands still and whole. */
export const ReducedMotion: Story = {
  args: { waiting: true, label: "Loading Account…", reducedMotion: true },
};

function DelayedWait({ label }: Readonly<{ label: ReactNode }>) {
  const [pending, setPending] = useState(false);
  const waiting = usePageWait(pending);
  return (
    <>
      <PageWait waiting={waiting} label={label} />
      <div style={{ padding: "var(--space-4)" }}>
        <Button onClick={() => setPending((now) => !now)}>
          {pending ? "Land the page" : "Start a move"}
        </Button>
      </div>
    </>
  );
}

/** Press the button: nothing shows for 200 ms, then the line until the press ends. */
export const AfterTheDelay: Story = {
  args: { waiting: false, label: "Loading Account…" },
  render: (args) => <DelayedWait label={args.label} />,
};
