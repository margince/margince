// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen } from "storybook/test";
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
      "GET /me": meRoute({ company: ["read", "update"], partner: ["update"] }),
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
    await screen.findByRole("textbox", { name: "Served segments" });
  },
};
