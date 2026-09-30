// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Panel, PanelBody } from "../design-system/panel";
import { BUSY_NIGHT } from "./magic.fixtures";
import { MagicGlance } from "./magic.glance";
import { StoryProviders } from "./story-utils";

const meta: Meta<typeof MagicGlance> = {
  title: "Shell/Home/Receipt at a glance",
  component: MagicGlance,
};
export default meta;
type Story = StoryObj<typeof MagicGlance>;

/** The night's work by kind, in records, largest first. */
export const OneNight: Story = {
  render: () => (
    <StoryProviders>
      <Panel title="Since your last brief">
        <PanelBody>
          <MagicGlance done={BUSY_NIGHT.done} />
        </PanelBody>
      </Panel>
    </StoryProviders>
  ),
};
