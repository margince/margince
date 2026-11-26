// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, within } from "storybook/test";
import { Row } from "../../design-system/stack";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactCreateActions } from "./contactcreate";

function actions(startCreating: boolean) {
  return () => {
    installFetchStub({ "GET /me": meRoute({ contact: ["read", "create"] }) });
    return (
      <StoryProviders>
        <Row gap="2">
          <ContactCreateActions startCreating={startCreating} />
        </Row>
      </StoryProviders>
    );
  };
}

const meta: Meta = {
  title: "Records/Contact create/List actions",
  render: actions(false),
};
export default meta;

type Story = StoryObj;

/** New contact names `C`, the key that opens it, while single keys are on. */
export const QuickCaptureAndNewContact: Story = {
  play: async ({ canvasElement }) => {
    const button = await within(canvasElement).findByRole("button", {
      name: "New contact",
    });
    await expect(button).toHaveAttribute("aria-keyshortcuts", "c");
  },
};

/** Opened by its address, `#/contacts/new`. */
export const OpenedByItsAddress: Story = {
  render: actions(true),
  play: async () => {
    await expect(
      await screen.findByRole("dialog", { name: "New contact" }),
    ).toBeInTheDocument();
  },
};
