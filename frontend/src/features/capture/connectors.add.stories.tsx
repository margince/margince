// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { within } from "storybook/test";
import { StoryProviders } from "../../testing/story-utils";
import { AddConnectionDialog, ALL_PROVIDERS } from "./connectors.add";

// The card's create verb: one row per provider not yet connected, each with
// the sentence its choice turns on.

function dialog(over: Partial<Parameters<typeof AddConnectionDialog>[0]> = {}) {
  return (
    <StoryProviders>
      <AddConnectionDialog
        open
        onClose={() => undefined}
        addable={ALL_PROVIDERS}
        pendingProvider={null}
        notConfigured501={null}
        connectError={null}
        onConnect={() => undefined}
        onImap={() => undefined}
        {...over}
      />
    </StoryProviders>
  );
}

const meta: Meta<typeof AddConnectionDialog> = {
  title: "Settings/You/Connections/Add connector",
  component: AddConnectionDialog,
};
export default meta;
type Story = StoryObj<typeof AddConnectionDialog>;

const opened: Story["play"] = async ({ canvasElement }) => {
  await within(canvasElement.ownerDocument.body).findByRole("dialog", {
    name: "Add connector",
  });
};

export const EveryProvider: Story = { render: () => dialog(), play: opened };

export const EveryProviderDark: Story = {
  globals: { theme: "dark" },
  render: () => dialog(),
  play: opened,
};

// A provider this installation never wired answers calmly, under the picks.
export const NotConfigured: Story = {
  render: () => dialog({ notConfigured501: "graph" }),
  play: opened,
};

export const ConnectRefused: Story = {
  render: () => dialog({ connectError: "The provider did not answer." }),
  play: opened,
};
