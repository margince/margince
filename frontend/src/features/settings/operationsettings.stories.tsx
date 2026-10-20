// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ComponentType } from "react";
import { expect, userEvent, within } from "storybook/test";
import { defaultOperations } from "../../testing/operationsettings.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { BackgroundSchedulesCard, SendPacingCard } from "./operationsettings";

// The two operating-value cards, editable and refused. The refused seat holds
// the read and not the write, so it sees the numbers and the sentence saying
// why they are fixed for it.
function story(Card: ComponentType, allow: Parameters<typeof meRoute>[0]) {
  return () => {
    let operations = { ...defaultOperations };
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /installation/settings": () => jsonResponse({ operations }),
      "PATCH /installation/settings": (body) => {
        operations = { ...operations, ...(body as object) };
        return jsonResponse({ operations });
      },
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

export const SchedulesAtPhoneWidth: Story = {
  ...Schedules,
  tags: ["uat-phone"],
};

// Opened, with a value under the row's floor typed in: the range shows only
// now, in the row's own unit.
export const SchedulesOpenRefused: Story = {
  render: story(BackgroundSchedulesCard, MANAGER),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByText("All schedules"));
    const box = await canvas.findByTestId(
      "operation-close_date_sweep_interval_seconds",
    );
    await userEvent.clear(box);
    await userEvent.type(box, "59{Enter}");
    await expect(box).toHaveAttribute("aria-invalid", "true");
  },
};
