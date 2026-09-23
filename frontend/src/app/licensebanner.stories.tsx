// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../screens/story-utils";
import { LicenseBanner } from "./licensebanner";
import { type GrantSpec, meFixture } from "./mefixture";

type LicenseState = components["schemas"]["LicenseEntitlement"]["state"];

const LICENSE_READER: GrantSpec = { license: ["read"] };

function story(state: LicenseState, allow: GrantSpec = LICENSE_READER) {
  return () => {
    installFetchStub({
      "GET /me": () => jsonResponse(meFixture({ allow })),
      "GET /installation/license": () =>
        jsonResponse({
          state,
          seats_used: 12,
          over_limit: false,
          checked_at: "2026-08-01T09:00:00Z",
        }),
    });
    return (
      <StoryProviders>
        <LicenseBanner />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof LicenseBanner> = {
  title: "Shell/License banner",
  component: LicenseBanner,
};
export default meta;
type Story = StoryObj<typeof LicenseBanner>;

export const Refused: Story = { render: story("rejected") };

// No licence is every dev and demo stack's state, so it draws nothing.
export const Absent: Story = { render: story("absent") };
export const Valid: Story = { render: story("valid") };

// The entitlement read is `license:read`; a seat without it never asks.
export const HiddenWithoutLicenseRead: Story = {
  render: story("rejected", {}),
};
