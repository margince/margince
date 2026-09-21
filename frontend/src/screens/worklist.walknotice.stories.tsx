// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "./story-utils";
import { WalkNotice } from "./worklist.walknotice";

// What has happened to a walk since the reader started it. An OFFER, not an
// error: the day on screen is still correct, it is simply no longer complete,
// and the remedy is a refresh when the reader is ready. A walk that has not
// moved draws nothing at all, which is why it has no story — a line saying
// "0 new" would be noise on every page a reader turns.

const meta: Meta<typeof WalkNotice> = {
  title: "Records/Worklist/Walk notice",
  component: WalkNotice,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof WalkNotice>;

function walk(over: Partial<{ new_available: number }> = {}) {
  return {
    as_of: "2026-09-05T09:00:00Z",
    changed_since_snapshot: 0,
    new_available: 0,
    ...over,
  };
}

/** Work arrived behind the reader, and the refresh is what brings it in. */
export const WorkArrived: Story = {
  render: () => (
    <StoryProviders>
      <WalkNotice
        walk={walk({ new_available: 3 })}
        onRefresh={() => undefined}
      />
    </StoryProviders>
  ),
};

/**
 * Work LEFT — dealt with, deleted, or no longer theirs to see. No refresh is
 * offered: refreshing does not bring these back, so the verb would point at
 * the wrong remedy.
 */
export const WorkLeft: Story = {
  render: () => (
    <StoryProviders>
      <WalkNotice
        walk={{ ...walk(), changed_since_snapshot: 2 }}
        onRefresh={() => undefined}
      />
    </StoryProviders>
  ),
};

/** Both, reported separately: one is a reason to refresh, the other explains
 * why a count fell, and a net figure would hide both. */
export const ArrivedAndLeft: Story = {
  render: () => (
    <StoryProviders>
      <WalkNotice
        walk={{ ...walk({ new_available: 3 }), changed_since_snapshot: 2 }}
        onRefresh={() => undefined}
      />
    </StoryProviders>
  ),
};
