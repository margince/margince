// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import { en as englishCopy } from "../../i18n/en";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { EndPartnerProgramAction } from "./partners.end";
import { PartnerTab } from "./partners.tab";

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
      "GET /companies/o-1/partner": () => new Response(null, { status: 204 }),
    });
    return (
      <StoryProviders>
        <PartnerTab companyId="o-1" />
      </StoryProviders>
    );
  },
  play: async () => {
    await screen.findByRole("textbox", {
      name: englishCopy["partner.servedSegments"],
    });
  },
};

export const EndingProgram: Story = {
  render: () => (
    <StoryProviders>
      <EndPartnerProgramAction
        companyId="o-1"
        version={3}
        onEnded={() => undefined}
      />
    </StoryProviders>
  ),
  play: async () => {
    await userEvent.click(
      await screen.findByRole("button", {
        name: englishCopy["partner.endProgram"],
      }),
    );
  },
};
