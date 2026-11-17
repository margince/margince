// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { type GrantSpec, meFixture } from "../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { TrialEndedScreen } from "./trialended";

const DAY = 86_400_000;

function story(
  allow: GrantSpec,
  roles: string[],
  reason: "trial" | "license" = "trial",
) {
  return () => {
    const ended = Date.now() - 2 * DAY;
    const lock =
      reason === "license" ? { license_lapsed: true } : { trial_ended: true };
    installFetchStub({
      "GET /me": () =>
        jsonResponse({ ...meFixture({ allow, roles }), ...lock }),
      "GET /installation/license": () =>
        jsonResponse({
          state: "absent",
          seats_used: 12,
          over_limit: false,
          checked_at: new Date().toISOString(),
          trial: {
            started_at: new Date(ended - 14 * DAY).toISOString(),
            ends_at: new Date(ended).toISOString(),
            ended: true,
          },
        }),
    });
    return (
      <StoryProviders>
        <TrialEndedScreen reason={reason} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof TrialEndedScreen> = {
  title: "Shell/Trial ended",
  component: TrialEndedScreen,
};
export default meta;
type Story = StoryObj<typeof TrialEndedScreen>;

// An admin is told where the licence goes once the team sends it. One who may
// change the installation can also check the licence again here.
export const Admin: Story = {
  render: story({ license: ["read"], installation_settings: ["update"] }, [
    "admin",
  ]),
};
// Every other seat is pointed at its admin, and can write to the team too.
export const Member: Story = { render: story({}, ["rep"]) };
// A license that expired or was refused locks the same way, and says so.
export const LicenseLapsed: Story = {
  render: story({ license: ["read"] }, ["admin"], "license"),
};
