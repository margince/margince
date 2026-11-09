// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent } from "storybook/test";
import { isLocale } from "../../i18n";
import { StoryProviders, stubWithSession } from "../../testing/story-utils";
import { RecordConversationFact } from "./composecontextfact";

const meta: Meta<typeof RecordConversationFact> = {
  title: "Patterns/Compose mail/Record a request",
  component: RecordConversationFact,
  beforeEach: () => stubWithSession({}, { contact: ["read", "update"] }),
  decorators: [
    (Story, { parameters }) => (
      <StoryProviders
        locale={
          typeof parameters.locale === "string" && isLocale(parameters.locale)
            ? parameters.locale
            : undefined
        }
      >
        <Story />
      </StoryProviders>
    ),
  ],
  args: {
    target: {
      contactId: "c-1",
      address: "anna@example.test",
      category: "requested_followup",
    },
  },
};
export default meta;
type Story = StoryObj<typeof meta>;

export const Empty: Story = {
  play: async () => {
    await expect(
      await screen.findByRole("button", { name: "Record what happened" }),
    ).toBeInTheDocument();
  },
};

export const Open: Story = {
  play: async () => {
    await userEvent.click(
      await screen.findByRole("button", { name: "Record what happened" }),
    );
    await expect(
      await screen.findByRole("dialog", {
        name: "Record the exchange with anna@example.test",
      }),
    ).toBeInTheDocument();
  },
};

export const RequestedQuote: Story = {
  args: {
    target: {
      contactId: "c-1",
      address: "anna@example.test",
      category: "precontract_quote",
    },
    chosen: "precontract_quote",
  },
  play: Open.play,
};

export const Narrow: Story = { tags: ["uat-phone"], play: Open.play };

export const German: Story = {
  parameters: { locale: "de" },
  play: async () => {
    await userEvent.click(
      await screen.findByRole("button", { name: "Gespräch festhalten" }),
    );
    await expect(
      await screen.findByRole("dialog", {
        name: "Gespräch mit anna@example.test festhalten",
      }),
    ).toBeInTheDocument();
  },
};

export const Vietnamese: Story = {
  parameters: { locale: "vi" },
  play: async () => {
    await userEvent.click(
      await screen.findByRole("button", { name: "Ghi lại sự việc" }),
    );
    await expect(
      await screen.findByRole("dialog", {
        name: "Ghi lại trao đổi với anna@example.test",
      }),
    ).toBeInTheDocument();
  },
};
