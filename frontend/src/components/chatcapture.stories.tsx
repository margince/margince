// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { ChatCapture } from "./chatcapture";

const meta: Meta<typeof ChatCapture> = {
  title: "Records/Capture chat",
  component: ChatCapture,
};
export default meta;
type Story = StoryObj<typeof ChatCapture>;
export const ChooseProvider: Story = {
  render: () => {
    installFetchStub({
      "GET /channel-providers": () =>
        jsonResponse({
          data: [
            { provider: "whatsapp", label: "WhatsApp" },
            { provider: "linkedin", label: "LinkedIn" },
          ],
        }),
    });
    return (
      <StoryProviders>
        <ChatCapture
          record={{ type: "contact", id: "p1", name: "Ada Weber" }}
          onClose={() => undefined}
        />
      </StoryProviders>
    );
  },
};
export const ChooseProviderDark: Story = {
  ...ChooseProvider,
  globals: { theme: "dark" },
};
