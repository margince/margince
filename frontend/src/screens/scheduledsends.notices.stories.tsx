// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { en } from "../i18n/en";
import { WriteRefused } from "./common";
import { QueueSkewNotice } from "./scheduledsends.notices";
import { StoryProviders } from "./story-utils";

// What the scheduled queue says about itself. Read side by side because the
// pair is the point: the milder notice is the one that carries a verb, and the
// louder one has nothing for the reader to press.

const meta: Meta = {
  title: "Patterns/Scheduled queue notices",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

/** The list moved under the reader, and re-reading is the way out. */
export const VersionSkew: Story = {
  render: () => (
    <StoryProviders>
      <QueueSkewNotice message={en["sched.skew"]} onReload={() => {}} />
    </StoryProviders>
  ),
};

/** A move the server refused, in the server's own words. */
export const Refused: Story = {
  render: () => (
    <StoryProviders>
      <WriteRefused
        titleKey="sched.writeFailed"
        message="That message had already gone out."
      />
    </StoryProviders>
  ),
};
