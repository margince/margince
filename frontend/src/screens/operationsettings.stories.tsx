// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ComponentType } from "react";
import { BackgroundSchedulesCard, SendPacingCard } from "./operationsettings";
import { defaultOperations } from "./operationsettings.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The two operating-value cards, editable and refused. The refused seat holds
// the read and not the write, so it sees the numbers and the sentence saying
// why they are fixed for it.
function story(Card: ComponentType, allow: Parameters<typeof meRoute>[0]) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /installation/settings": () =>
        jsonResponse({ operations: defaultOperations }),
      "PATCH /installation/settings": () =>
        jsonResponse({ operations: defaultOperations }),
    });
    return (
      <StoryProviders>
        <Card />
      </StoryProviders>
    );
  };
}

const MANAGER = { installation_settings: ["read", "update"] } as const;
const READER = { installation_settings: ["read"] } as const;

const meta: Meta<typeof BackgroundSchedulesCard> = {
  title: "Settings/Governance/System health/Schedules and send pacing",
  component: BackgroundSchedulesCard,
};
export default meta;
type Story = StoryObj<typeof BackgroundSchedulesCard>;

export const Schedules: Story = {
  render: story(BackgroundSchedulesCard, MANAGER),
};

export const SchedulesCannotChange: Story = {
  render: story(BackgroundSchedulesCard, READER),
};

export const SendPacing: Story = { render: story(SendPacingCard, MANAGER) };

export const SendPacingCannotChangeDark: Story = {
  globals: { theme: "dark" },
  render: story(SendPacingCard, READER),
};
