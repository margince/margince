// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { PartnerTab } from "./partners.form";

const meta: Meta = {
  title: "Records/Partner setup form",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

export const Setup: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /companies/o-1/partner": () =>
        jsonResponse({ title: "Not found", detail: "no partner" }, 404),
    });
    return (
      <StoryProviders>
        <PartnerTab companyId="o-1" />
      </StoryProviders>
    );
  },
  play: async () => {
    await screen.findByRole("textbox", { name: "Next step due" });
  },
};

/** The calendar behind "Next step due", open on today with nothing chosen. */
export const PickingTheNextStepDay: Story = {
  ...Setup,
  play: async () => {
    await screen.findByRole("textbox", { name: "Next step due" });
    await userEvent.click(screen.getByRole("button", { name: /^Choose date/ }));
    await screen.findByRole("dialog", { name: /^Choose date/ });
  },
};
