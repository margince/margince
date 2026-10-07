// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";
import { useState } from "react";
import { Button, Disclosure } from "./atoms";

const meta: Meta<typeof Disclosure> = {
  title: "Components/Layout and structure/Disclosure",
  component: Disclosure,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof Disclosure>;

const stack: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "1rem",
};

export const ClosedAndOpen: Story = {
  render: () => (
    <div style={stack}>
      <Disclosure summary="Matching rules">
        <p className="t-caption">
          Closed by default for details the reader rarely needs.
        </p>
      </Disclosure>
      <Disclosure summary="Import log" open>
        <p className="t-caption">Open for a run or a new result.</p>
      </Disclosure>
    </div>
  ),
};

function ControlledExample() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button onClick={() => setOpen(true)}>Open details</Button>
      <Disclosure summary="Editable details" open={open} onToggle={setOpen}>
        <p>The reader can close and reopen this section.</p>
      </Disclosure>
    </>
  );
}

export const Controlled: Story = {
  render: () => <ControlledExample />,
};
