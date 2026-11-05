// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { OvernightGrantChoice } from "../../components/overnight-grant-choice";
import { SettingList } from "../../design-system/settingrow";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { OvernightGrantCard } from "./overnight-grant";

// The rep's standing overnight authority, in both places it is asked.
//
// Onboarding: a Switch row, on (nothing to warn about) or off (the notice naming
// what goes empty). Settings: on, off (an Off badge and a quiet sentence, no
// warning), and the renewal cases where a yes no longer does the job.

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

/** Onboarding, turned off: the notice naming the screens that go empty. */
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

/** Settings, withdrawn: the approved Off badge and quiet sentence. */
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

export const SettingsDeclinedDark: Story = {
  ...SettingsDeclined,
  globals: { theme: "dark" },
};

export const OnboardingPreselectedDark: Story = {
  ...OnboardingPreselected,
  globals: { theme: "dark" },
};

/** Switched off here: the same notice as onboarding, in the same words. */
export const SettingsSwitchedOff: Story = {
  render: () => {
    let state: "granted" | "declined" = "granted";
    installFetchStub({
      "GET /me/agent-grants": () => grantsResponse(state),
      "PUT /me/agent-grants/morning_brief": () => {
        state = "declined";
        return jsonResponse({ spec: "morning_brief", state });
      },
    });
    return (
      <StoryProviders>
        <OvernightGrantCard />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByTestId("overnight-grant-toggle"));
    await waitFor(() =>
      expect(canvas.getByText("Overnight agent will not run")).toBeVisible(),
    );
  },
};

export const SettingsGrantedPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
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

/** A flip the server refused: said once for the card, under the row. */
export const SettingsWriteRefused: Story = {
  render: () => {
    installFetchStub({
      "GET /me/agent-grants": () => grantsResponse("granted"),
      "PUT /me/agent-grants/morning_brief": () =>
        jsonResponse(
          { title: "Unprocessable", detail: "The change could not be saved." },
          422,
        ),
    });
    return (
      <StoryProviders>
        <OvernightGrantCard />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByTestId("overnight-grant-toggle"));
    await canvas.findByText("Change was not saved");
  },
};

/** The switch owns no state (the onboarding act does), and it is a row for the
 * caller's list, so the story holds both. */
function ChoiceHarness({ initial }: Readonly<{ initial: boolean }>) {
  const [checked, setChecked] = useState(initial);
  return (
    <SettingList>
      <OvernightGrantChoice checked={checked} onChange={setChecked} />
    </SettingList>
  );
}
