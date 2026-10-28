// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { QuickCaptureAction } from "./quickcapture";

// Quick capture with the Company box New contact shares.

const meta: Meta = {
  title: "Records/Contacts/Quick capture",
  render: () => {
    installFetchStub({ "GET /me": meRoute({ contact: ["read", "create"] }) });
    return (
      <StoryProviders>
        <QuickCaptureAction />
      </StoryProviders>
    );
  },
};
export default meta;

type Story = StoryObj;

export const Open: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByTestId("quick-capture"),
    );
  },
};

export const OpenPhone: Story = {
  ...Open,
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
