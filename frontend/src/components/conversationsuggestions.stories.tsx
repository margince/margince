// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { ConversationSuggestionsCard } from "./conversationsuggestions";

function story(type: "contact" | "lead", limited = false, readOnly = false) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({
        contact: readOnly ? ["read"] : ["read", "create"],
        lead: ["read"],
        activity: ["read"],
      }),
      [`GET /${type}s/anchor/conversation-suggestions`]: () =>
        jsonResponse({
          data: [
            {
              address: "jana@beispiel.test",
              display_name: "Jana Becker",
              activity_id: "activity-1",
            },
            {
              address: "minh.nguyen@buyer.test",
              display_name: "Minh Nguyen",
              activity_id: "activity-2",
            },
          ],
          scan_limited: limited,
        }),
    });
    return (
      <StoryProviders>
        <ConversationSuggestionsCard record={{ type, id: "anchor" }} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof ConversationSuggestionsCard> = {
  title: "Records/Record 360/Conversation suggestions",
  component: ConversationSuggestionsCard,
};
export default meta;
type Story = StoryObj<typeof ConversationSuggestionsCard>;
export const OnAContact: Story = { render: story("contact") };
export const OnALead: Story = { render: story("lead") };
export const ScanLimitReached: Story = { render: story("contact", true) };
export const WithoutContactCreation: Story = {
  render: story("contact", false, true),
};
export const OnAContactDark: Story = {
  globals: { theme: "dark" },
  render: story("contact"),
};
export const OnALeadDark: Story = {
  globals: { theme: "dark" },
  render: story("lead"),
};
export const ScanLimitReachedDark: Story = {
  globals: { theme: "dark" },
  render: story("contact", true),
};
export const WithoutContactCreationDark: Story = {
  globals: { theme: "dark" },
  render: story("contact", false, true),
};
