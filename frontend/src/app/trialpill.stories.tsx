// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { meFixture } from "../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { TrialPill } from "./trialpill";

const DAY = 86_400_000;

// The pill under the rail's orb, at each stage of a 14-day trial.
function story(daysLeft: number, ended = false) {
  return () => {
    const ends = Date.now() + daysLeft * DAY - 60_000;
    installFetchStub({
      "GET /me": () =>
        jsonResponse(meFixture({ allow: { license: ["read"] } })),
      "GET /installation/license": () =>
        jsonResponse({
          state: "absent",
          seats_used: 12,
          over_limit: false,
          checked_at: new Date().toISOString(),
          trial: {
            started_at: new Date(ends - 14 * DAY).toISOString(),
            ends_at: new Date(ends).toISOString(),
            ended,
          },
        }),
    });
    return (
      <StoryProviders>
        <TrialPill />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof TrialPill> = {
  title: "Shell/Trial pill",
  component: TrialPill,
};
export default meta;
type Story = StoryObj<typeof TrialPill>;

export const Running: Story = { render: story(10) };
export const Closing: Story = { render: story(6) };
export const LastDay: Story = { render: story(1) };
export const Ended: Story = { render: story(0, true) };
