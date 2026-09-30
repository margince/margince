// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../screens/story-utils";
import { ConnectivityNotice } from "./connectivitybanner";

// The notice, not the connected banner: a story cannot take the browser offline,
// and an outage reported to the live store would probe Storybook's own server.
const meta = {
  title: "Shell/Connectivity banner",
  component: ConnectivityNotice,
  render: (args) => (
    <StoryProviders>
      <ConnectivityNotice {...args} />
    </StoryProviders>
  ),
} satisfies Meta<typeof ConnectivityNotice>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Offline: Story = { args: { state: "offline" } };

export const Unreachable: Story = { args: { state: "unreachable" } };

// `uat-phone` makes the capture gate draw this at 390px, where the sentence
// has to wrap inside the notice rather than push the page sideways.
export const Phone: Story = {
  name: "phone — offline",
  args: { state: "offline" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
