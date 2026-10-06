// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { RefreshCw } from "lucide-react";
import type { CSSProperties } from "react";
import { BusyMark, Button } from "./atoms";
import { Switch } from "./switch";

const meta: Meta<typeof BusyMark> = {
  title: "Components/Loading/Busy mark",
  component: BusyMark,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof BusyMark>;

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

// The mark takes no size of its own, so it is only ever seen in a host; the
// resting controls beside it show what the host looks like without it.
export const InItsHosts: Story = {
  render: () => (
    <div style={stack}>
      <div style={stack}>
        <span className="t-label">Button</span>
        <div style={row}>
          <Button variant="primary" pending>
            Save
          </Button>
          <Button variant="ghost" pending>
            Reconnect
          </Button>
          <Button variant="ghost" iconOnly pending aria-label="Reconnect">
            <RefreshCw aria-hidden />
          </Button>
          <Button variant="primary">Save</Button>
        </div>
      </div>
      <div style={stack}>
        <span className="t-label">Switch</span>
        <div style={{ ...stack, maxInlineSize: "22rem" }}>
          <Switch
            label="Capture this mailbox"
            checked
            pending
            onChange={() => undefined}
          />
          <Switch
            label="Capture this mailbox"
            checked
            onChange={() => undefined}
          />
        </div>
      </div>
    </div>
  ),
};
