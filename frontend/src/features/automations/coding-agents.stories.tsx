// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { CodingAgentsCard } from "./coding-agents";

function story(connectorEnabled = true) {
  return () => {
    installFetchStub({
      "GET /.well-known/oauth-protected-resource": () =>
        connectorEnabled
          ? jsonResponse({ resource: "https://margince.example/mcp" })
          : jsonResponse({ code: "not_found" }, 404),
    });
    return (
      <StoryProviders>
        <CodingAgentsCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof CodingAgentsCard> = {
  title: "Settings/You/AI access/Coding agents",
  component: CodingAgentsCard,
};
export default meta;
type Story = StoryObj<typeof CodingAgentsCard>;

export const Commands: Story = { render: story() };

export const ConnectorOff: Story = { render: story(false) };

// At 390px a long command wraps inside the card instead of widening it.
export const Phone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(),
};
