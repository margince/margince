// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { meFixture } from "../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { TrialContactDialog } from "./trialcontact";

const DAY = 86_400_000;

function story(daysLeft: number | undefined) {
  return () => {
    installFetchStub({ "GET /me": () => jsonResponse(meFixture({})) });
    const standing =
      daysLeft === undefined
        ? undefined
        : {
            endsAt: new Date(Date.now() + daysLeft * DAY).toISOString(),
            daysLeft: Math.max(daysLeft, 0),
            ended: daysLeft <= 0,
            closing: daysLeft <= 7,
          };
    return (
      <StoryProviders>
        <TrialContactDialog
          standing={standing}
          seatsUsed={12}
          onClose={() => undefined}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof TrialContactDialog> = {
  title: "Components/Trial contact dialog",
  component: TrialContactDialog,
};
export default meta;
type Story = StoryObj<typeof TrialContactDialog>;

export const WhileTheTrialRuns: Story = { render: story(5) };
export const AfterTheTrialEnded: Story = { render: story(-3) };
// A seat without license:read writes from the lock screen with no date to name.
export const WithoutTheTrialDay: Story = { render: story(undefined) };
