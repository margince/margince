// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { CommandLineToolsCard } from "./commandline-tools";

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
        <CommandLineToolsCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof CommandLineToolsCard> = {
  title: "Settings/You/AI access/Command-line tools",
  component: CommandLineToolsCard,
};
export default meta;
type Story = StoryObj<typeof CommandLineToolsCard>;

export const Commands: Story = { render: story() };

export const ConnectorOff: Story = { render: story(false) };

// At 390px a long command wraps inside the card instead of widening it.
export const Phone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(),
};
