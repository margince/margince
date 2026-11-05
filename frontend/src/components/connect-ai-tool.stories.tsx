// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
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

// Picks the AI, then where it is used; the listbox opens in the page body.
function picking(ai: string, place?: string, ...then: string[]) {
  return async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const body = within(canvasElement.ownerDocument.body);
    const answers: [string, string | undefined][] = [
      ["Which AI do you use?", ai],
      ["Where do you use it?", place],
    ];
    for (const [question, answer] of answers) {
      if (answer !== undefined) {
        await userEvent.click(
          await canvas.findByRole("combobox", { name: question }),
        );
        await userEvent.click(
          await body.findByRole("option", { name: answer }),
        );
      }
    }
    await choosing(...then)({ canvasElement });
  };
}

const meta: Meta<typeof ConnectAiToolCard> = {
  title: "Settings/You/AI access/Connect your AI tool",
  component: ConnectAiToolCard,
};
export default meta;
type Story = StoryObj<typeof ConnectAiToolCard>;

// Claude's app through the company: the admin goes first, nobody pastes anything.
export const CompanyPlan: Story = { render: story() };

// A plan the reader pays for: the address sits inside the step that asks for it.
export const OwnPlanWithAddress: Story = {
  render: story(),
  play: choosing("My own"),
};

export const Unavailable: Story = {
  render: story(),
  play: picking("Copilot", undefined, "My own"),
};

// The "where" list open: one AI, several places to use it.
export const PlacesOpen: Story = {
  render: story(),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("combobox", {
        name: "Where do you use it?",
      }),
    );
  },
};

// Claude Code in an editor: the one command, and where to type it.
export const ClaudeCodeInVisualStudio: Story = {
  render: story(),
  play: picking("Claude", "Claude Code in Visual Studio"),
};

export const CopilotInVsCode: Story = {
  render: story(),
  play: picking("Copilot", "GitHub Copilot in VS Code"),
};

// Cursor is used in one place, so the card does not ask where.
export const Cursor: Story = {
  render: story(),
  play: picking("Cursor"),
};

export const AnotherTool: Story = {
  render: story(),
  play: picking("Another AI tool"),
};

export const ConnectorOff: Story = { render: story(false) };

export const OwnPlanDark: Story = {
  globals: { theme: "dark" },
  render: story(),
  play: picking("Gemini", undefined, "My own"),
};

export const LocalToolDark: Story = {
  globals: { theme: "dark" },
  render: story(),
  play: picking("ChatGPT / Codex", "Codex in a terminal or editor"),
};

export const Phone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(),
  play: picking("ChatGPT / Codex", undefined, "My own"),
};
