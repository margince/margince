// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactChatCapture } from "./contactchatcapture";

const meta: Meta<typeof ContactChatCapture> = {
  title: "Records/Contact 360/Capture chat",
  component: ContactChatCapture,
};
export default meta;
type Story = StoryObj<typeof ContactChatCapture>;
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
        <ContactChatCapture
          contactId="p1"
          contactName="Ada Weber"
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
