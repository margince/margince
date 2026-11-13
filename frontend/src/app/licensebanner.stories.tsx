// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { type GrantSpec, meFixture } from "../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { LicenseBanner } from "./licensebanner";

type LicenseState = components["schemas"]["LicenseEntitlement"]["state"];
type LicenseHolder = components["schemas"]["LicenseHolder"];

const LICENSE_READER: GrantSpec = { license: ["read"] };

const IN_GRACE: LicenseHolder = {
  id: "lic-1",
  subject: "acme-prod",
  expiry: "2026-07-01T00:00:00Z",
  in_grace: true,
  renewal_due: true,
};

const DAY = 86_400_000;

// A trial that ends `days` from whenever the story is opened, so it reads the
// same on any day.
function trialEnding(days: number) {
  const ends = Date.now() + days * DAY;
  return {
    started_at: new Date(ends - 14 * DAY).toISOString(),
    ends_at: new Date(ends).toISOString(),
    ended: false,
  };
}

function story(
  state: LicenseState,
  allow: GrantSpec = LICENSE_READER,
  license?: LicenseHolder,
  trial?: ReturnType<typeof trialEnding>,
) {
  return () => {
    installFetchStub({
      "GET /me": () => jsonResponse(meFixture({ allow })),
      "GET /installation/license": () =>
        jsonResponse({
          state,
          license,
          trial,
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
export const InGrace: Story = {
  render: story("valid", LICENSE_READER, IN_GRACE),
};

// A production installation with no licence runs on its trial. While more than
// a week is left the orb's ring carries it, and this draws nothing. In the last
// week it warns. It may be put away until tomorrow, except on the last day.
export const TrialRunning: Story = {
  render: story("absent", LICENSE_READER, undefined, trialEnding(10)),
};
export const TrialClosing: Story = {
  render: story("absent", LICENSE_READER, undefined, trialEnding(3)),
};
export const TrialLastDay: Story = {
  render: story("absent", LICENSE_READER, undefined, trialEnding(0.5)),
};

// No licence is every dev and demo stack's state, so it draws nothing.
export const Absent: Story = { render: story("absent") };
export const Valid: Story = { render: story("valid") };

// The entitlement read is `license:read`; a seat without it never asks.
export const HiddenWithoutLicenseRead: Story = {
  render: story("rejected", {}),
};
