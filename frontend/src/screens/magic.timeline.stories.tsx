// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Panel, PanelBody } from "../design-system/panel";
import { viewerZone } from "../format/timezone";
import { BUSY_NIGHT } from "./magic.fixtures";
import type { MagicReceipt } from "./magic.queries";
import { MagicTimeline } from "./magic.timeline";
import { StoryProviders } from "./story-utils";

const meta: Meta<typeof MagicTimeline> = {
  title: "Shell/Home/Receipt timeline",
  component: MagicTimeline,
};
export default meta;
type Story = StoryObj<typeof MagicTimeline>;

function strip(receipt: MagicReceipt) {
  return (
    <StoryProviders>
      <Panel title="Since your last brief">
        <PanelBody>
          <MagicTimeline receipt={receipt} zone={viewerZone()} />
        </PanelBody>
      </Panel>
    </StoryProviders>
  );
}

/** Evening to morning: a bulk sync after midnight, agents towards morning. */
export const OneNight: Story = { render: () => strip(BUSY_NIGHT) };

/** The same night in dark, where every mark's colour is a theme token. */
export const OneNightDark: Story = {
  globals: { theme: "dark" },
  render: () => strip(BUSY_NIGHT),
};

/** The same lines over a week: the axis names days rather than hours. */
export const AWeek: Story = {
  render: () => strip({ ...BUSY_NIGHT, since: "2026-09-06T08:05:00Z" }),
};
