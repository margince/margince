// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ConnectFlow } from "./connect-flow";

// The connect dialog's three steps for each account, the refusals step 1 can
// show, and the vendor that answers "not set up here". `screen` in every play,
// because the dialog is portalled out of the story canvas.

const meta: Meta<typeof ConnectFlow> = {
  title: "Settings/You/Connections/Connect flow",
  component: ConnectFlow,
  args: { open: true, onClose: () => {}, onImap: () => {} },
  render: (args) => {
    installFetchStub({
      "POST /connectors/graph/connect": () =>
        jsonResponse({ code: "not_implemented", detail: "not wired" }, 501),
    });
    return (
      <StoryProviders>
        <ConnectFlow {...args} />
      </StoryProviders>
    );
  },
};
export default meta;
type Story = StoryObj<typeof ConnectFlow>;

async function next() {
  await userEvent.click(screen.getByRole("button", { name: "Continue" }));
}

export const ChooseAccount: Story = {};

export const ChooseAccountDark: Story = { globals: { theme: "dark" } };

// Microsoft has no company app here, and the IMAP mailbox is already on the
// roster: each refusal says why on its own plate.
export const AccountsRefused: Story = {
  args: {
    availability: [
      { provider: "gmail", reason: "ready" },
      { provider: "graph", reason: "app_missing" },
      { provider: "imap", reason: "ready" },
    ],
    connected: ["imap"],
  },
};

export const GoogleBringIn: Story = { args: { startFrom: "google" } };

export const GoogleBringInDark: Story = {
  args: { startFrom: "google" },
  globals: { theme: "dark" },
};

// The mailbox is connected already, so only its calendar is left to add.
export const GoogleCalendarOnly: Story = {
  args: { startFrom: "google", connected: ["gmail"] },
};

export const GoogleSignIn: Story = {
  args: { startFrom: "google" },
  play: next,
};

export const GoogleSignInDark: Story = {
  args: { startFrom: "google" },
  globals: { theme: "dark" },
  play: next,
};

export const MicrosoftSignIn: Story = {
  args: { startFrom: "microsoft" },
  play: next,
};

export const MicrosoftNotConfigured: Story = {
  args: { startFrom: "microsoft" },
  play: async () => {
    await next();
    await userEvent.click(
      screen.getByRole("button", { name: "Continue to Microsoft 365" }),
    );
    await screen.findByText(/not configured on this installation/);
  },
};

export const OtherMailHost: Story = { args: { startFrom: "imap" } };

export const OtherMailHostDark: Story = {
  args: { startFrom: "imap" },
  globals: { theme: "dark" },
};
