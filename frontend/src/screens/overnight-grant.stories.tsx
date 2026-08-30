// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { OvernightGrantCard, OvernightGrantChoice } from "./overnight-grant";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The rep's standing overnight authority, in both places it is asked.
//
// The states worth rendering are the ones a screenshot can tell apart and a
// reader has to act on differently: ticked (nothing to warn about), cleared
// (the red notice naming what goes empty), and — settings only — the renewal
// case, where the rep already said yes and the credential behind it has died.

const meta: Meta<typeof OvernightGrantCard> = {
  title: "Shell/Overnight grant",
  component: OvernightGrantCard,
};
export default meta;
type Story = StoryObj<typeof OvernightGrantCard>;

function grantsResponse(
  state: "granted" | "declined" | "never_asked",
  credentialUsable = true,
  // Defaulted, because the field's ABSENCE from the response reads as false
  // and put the scope-renewal notice on every granted story in this file. A
  // story about anything else shows a covered credential.
  credentialFundsAgent = true,
) {
  return jsonResponse({
    data: [
      {
        spec: "morning_brief",
        state,
        credential_usable: credentialUsable,
        credential_funds_agent: credentialFundsAgent,
        decided_at: "2026-08-20T06:00:00Z",
      },
    ],
  });
}

/** Onboarding, as it arrives: preselected, no warning. */
export const OnboardingPreselected: Story = {
  render: () => {
    installFetchStub({});
    return (
      <StoryProviders>
        <ChoiceHarness initial={true} />
      </StoryProviders>
    );
  },
};

/** Onboarding, cleared — the red notice naming the screens that go empty. */
export const OnboardingCleared: Story = {
  render: () => {
    installFetchStub({});
    return (
      <StoryProviders>
        <ChoiceHarness initial={false} />
      </StoryProviders>
    );
  },
};

/** Settings, granted: the ordinary resting state. */
export const SettingsGranted: Story = {
  render: () => {
    installFetchStub({
      "GET /me/agent-grants": () => grantsResponse("granted"),
    });
    return (
      <StoryProviders>
        <OvernightGrantCard />
      </StoryProviders>
    );
  },
};

/** Settings, withdrawn: the same red notice as onboarding, same words. */
export const SettingsDeclined: Story = {
  render: () => {
    installFetchStub({
      "GET /me/agent-grants": () => grantsResponse("declined"),
    });
    return (
      <StoryProviders>
        <OvernightGrantCard />
      </StoryProviders>
    );
  },
};

/** The rep agreed and their credential has since died. Reported as its own
 * state — showing it as a decline would put a settled question back to them. */
export const SettingsNeedsRenewal: Story = {
  render: () => {
    installFetchStub({
      "GET /me/agent-grants": () => grantsResponse("granted", false),
    });
    return (
      <StoryProviders>
        <OvernightGrantCard />
      </StoryProviders>
    );
  },
};

/** The rep agreed, their credential still works, and the agent has since
 * learned to do more than it covers. Its own notice: nothing expired, and
 * telling them it did sends them looking for a lapse that never happened. */
export const SettingsNeedsWiderAuthority: Story = {
  render: () => {
    installFetchStub({
      "GET /me/agent-grants": () => grantsResponse("granted", true, false),
    });
    return (
      <StoryProviders>
        <OvernightGrantCard />
      </StoryProviders>
    );
  },
};

/** The checkbox owns no state of its own — the onboarding act does — so a
 * story has to hold it for the box to be tickable at all. */
function ChoiceHarness({ initial }: Readonly<{ initial: boolean }>) {
  const [checked, setChecked] = useState(initial);
  return <OvernightGrantChoice checked={checked} onChange={setChecked} />;
}
