// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { MailSharingCard } from "./mail-sharing";

// The workspace mail-sharing posture. ON is the quiet default; a posture with a
// cost (sharing off, or seats allowed to share) states it under the switches.
// Each switch writes on flip, so the only other state is a refused write.
function story(
  mail_sharing: boolean,
  allow: Parameters<typeof meRoute>[0],
  shared_posture_allowed = false,
  refuse = false,
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /capture/settings": () =>
        jsonResponse({
          auto_enrich: true,
          mail_sharing,
          shared_posture_allowed,
        }),
      ...(refuse
        ? {
            "PATCH /capture/settings": () =>
              jsonResponse(
                {
                  title: "Forbidden",
                  detail: "The posture could not be saved.",
                },
                403,
              ),
          }
        : {}),
    });
    return (
      <StoryProviders>
        <MailSharingCard />
      </StoryProviders>
    );
  };
}

const MANAGER = { capture_settings: ["read", "update"] } as const;
const READER = { capture_settings: ["read"] } as const;

const meta: Meta<typeof MailSharingCard> = {
  title: "Settings/Data/Capture rules/Email sharing",
  component: MailSharingCard,
};
export default meta;
type Story = StoryObj<typeof MailSharingCard>;

export const SharingOn: Story = { render: story(true, MANAGER) };

// The stored posture IS off: the danger callout stands without any
// interaction, so an admin landing here later still sees the cost stated.
export const SharingOff: Story = { render: story(false, MANAGER) };

export const CannotChange: Story = { render: story(true, READER) };

// Seats may put their mailbox in the shared posture: the works-council warning
// stands under the switches for as long as that is allowed.
export const SharedPostureAllowed: Story = {
  render: story(true, MANAGER, true),
};

// A flip the server refused: one notice for the card, and the switch shows the
// stored posture again.
export const SaveRefused: Story = {
  render: story(true, MANAGER, false, true),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByTestId("mail-sharing-toggle"));
    await canvas.findByText("Setting not saved");
  },
};
