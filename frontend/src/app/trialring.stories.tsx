// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { MarginceCoreScene } from "../design-system/margince-core";
import { trialStandingOf } from "../lib/license-posture";
import { StoryProviders } from "../testing/story-utils";
import { TrialRing } from "./trialring";
import "./agentrail.css";
import "./shell.css";

const DAY = 86_400_000;

// The orb as the rail draws it, with the trial's ring at one stage of 14 days.
function Orb({
  daysLeft,
  folded = false,
}: Readonly<{ daysLeft: number; folded?: boolean }>) {
  const now = Date.now();
  const ends = now + daysLeft * DAY - 60_000;
  const standing = trialStandingOf(
    {
      started_at: new Date(ends - 14 * DAY).toISOString(),
      ends_at: new Date(ends).toISOString(),
      ended: false,
    },
    now,
  );
  return (
    <nav
      className={folded ? "rail collapsed" : "rail"}
      style={{
        width: folded ? 72 : 240,
        padding: "var(--space-5)",
        alignItems: "center",
      }}
    >
      <span className="artoggle">
        <MarginceCoreScene size="md" className="arorb" />
        <TrialRing standing={standing} />
      </span>
    </nav>
  );
}

const meta: Meta<typeof Orb> = {
  title: "Shell/Trial ring",
  component: Orb,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof Orb>;

/** Early on: most marks still lit, in the calm tone. */
export const Running: Story = { args: { daysLeft: 10 } };
/** The last week turns the ring amber, and today's dot breathes. */
export const LastWeek: Story = { args: { daysLeft: 6 } };
/** One mark left, just before twelve o'clock. */
export const LastDay: Story = { args: { daysLeft: 1 } };
/** The folded rail: one arc, and the count on the orb in the last week. */
export const FoldedLastWeek: Story = { args: { daysLeft: 6, folded: true } };
