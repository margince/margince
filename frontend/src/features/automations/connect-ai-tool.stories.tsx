// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ConnectAiToolCard } from "./connect-ai-tool";

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
        <ConnectAiToolCard />
      </StoryProviders>
    );
  };
}

function choosing(...names: string[]) {
  return async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    for (const name of names) {
      await userEvent.click(await canvas.findByRole("button", { name }));
    }
  };
}

const meta: Meta<typeof ConnectAiToolCard> = {
  title: "Settings/You/AI access/Connect your AI tool",
  component: ConnectAiToolCard,
};
export default meta;
type Story = StoryObj<typeof ConnectAiToolCard>;

// Claude through the company: the admin goes first, and nobody pastes anything.
export const CompanyPlan: Story = { render: story() };

// A plan the reader pays for: the address sits inside the step that asks for it.
export const OwnPlanWithAddress: Story = {
  render: story(),
  play: choosing("My own"),
};

export const Unavailable: Story = {
  render: story(),
  play: choosing("Microsoft Copilot", "My own"),
};

export const ConnectorOff: Story = { render: story(false) };

export const OwnPlanDark: Story = {
  globals: { theme: "dark" },
  render: story(),
  play: choosing("Gemini", "My own"),
};

export const Phone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(),
  play: choosing("ChatGPT", "My own"),
};
