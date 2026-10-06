// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import { ComposeModal } from "./compose";
import { ConversationFold } from "./composeconversation";
import { SelectedReply, selectedReplyRoutes } from "./composethread.stories";
import { installFetchStub, StoryProviders } from "./story-utils";

const meta: Meta = {
  title: "Patterns/Compose mail/Conversation fold",
  component: ConversationFold,
};
export default meta;

/** A reading drawer wide enough for both: the thread beside the draft. */
export const Beside: StoryObj = {
  parameters: {
    viewport: {
      options: {
        desktop: {
          name: "Desktop",
          styles: { width: "1024px", height: "720px" },
        },
      },
    },
  },
  globals: { viewport: { value: "desktop" } },
  render: SelectedReply.render,
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await dialog.findByRole("heading", { name: "This thread" });
    await expect(
      dialog.queryByRole("button", { name: "Show this thread" }),
    ).toBeNull();
  },
};

/** A phone: the thread waits behind its toggle above the draft, opened here. */
export const Folded: StoryObj = {
  render: SelectedReply.render,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await userEvent.click(
      await dialog.findByRole("button", { name: "Show this thread" }),
    );
    await dialog.findByRole("heading", { name: "This thread" });
    await dialog.findByRole("button", { name: "Hide this thread" });
  },
};

/** A fresh mail on a record with history: the earlier threads fold the same way. */
export const FoldedChoices: StoryObj = {
  render: () => {
    installFetchStub(selectedReplyRoutes());
    return (
      <StoryProviders>
        <ComposeModal
          entityType="contact"
          entityId="ada"
          open
          onClose={() => {}}
        />
      </StoryProviders>
    );
  },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await userEvent.click(
      await dialog.findByRole("button", { name: "Show earlier threads" }),
    );
    await dialog.findByRole("heading", { name: "Continue thread?" });
    await dialog.findByRole("button", { name: "Hide earlier threads" });
  },
};
