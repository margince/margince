// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Panel, PanelBody } from "../design-system/panel";
import { BUSY_NIGHT } from "./magic.fixtures";
import { MagicGlance } from "./magic.glance";
import type { MagicLine } from "./magic.queries";
import { StoryProviders } from "./story-utils";

const meta: Meta<typeof MagicGlance> = {
  title: "Shell/Home/Receipt at a glance",
  component: MagicGlance,
};
export default meta;
type Story = StoryObj<typeof MagicGlance>;

function glance(done: readonly MagicLine[]) {
  return (
    <StoryProviders>
      <Panel title="Since your last brief">
        <PanelBody>
          <MagicGlance done={done} />
        </PanelBody>
      </Panel>
    </StoryProviders>
  );
}

/** The night's work by kind, in records, largest first. */
export const OneNight: Story = { render: () => glance(BUSY_NIGHT.done) };

/** The same night in dark: the cards' grounds are `color-mix()`es of theme
 * tokens, and can be right in light and wrong in dark. */
export const OneNightDark: Story = {
  globals: { theme: "dark" },
  render: () => glance(BUSY_NIGHT.done),
};

/** A filing run whose read was cut short: its tile, and the total it joins,
 * read as a minimum. */
export const CutShort: Story = {
  render: () =>
    glance(
      BUSY_NIGHT.done.map((line) =>
        line.summary.key === "magic.action.mail_filed"
          ? { ...line, count_is_floor: true }
          : line,
      ),
    ),
};
