// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Panel } from "../../../design-system/panel";
import { StoryProviders } from "../../../testing/story-utils";
import { SignalStrip } from "./verdict";

// The findings a reader scans before deciding to read: each states the number
// that tripped it, or its rule's own words where it has none.

const meta: Meta<typeof SignalStrip> = {
  title: "Records/Record 360/Signal strip",
  component: SignalStrip,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof SignalStrip>;

function Strip() {
  return (
    <StoryProviders>
      <div style={{ maxWidth: 860 }}>
        <Panel title="Brandt Automotive GmbH · 360">
          <SignalStrip
            signals={[
              {
                key: "cold",
                label: "Going cold",
                figure: "84 days",
                tone: "warning",
              },
              {
                key: "promise",
                label: "Commitment overdue",
                figure: "19 days",
                tone: "danger",
              },
              { key: "reach", label: "One route in", tone: "accent" },
            ]}
          />
        </Panel>
      </div>
    </StoryProviders>
  );
}

export const Signals: Story = { render: () => <Strip /> };

export const SignalsDark: Story = {
  ...Signals,
  globals: { theme: "dark" },
};
