// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { en as englishCopy } from "../../i18n/en";
import {
  installFetchStub,
  permissionDenied,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactImpersonationWarning } from "./contactimpersonation";
import { populated } from "./contactpage.fixtures";

// The warning the contact page draws over a sender that showed a sign of
// impersonation. One story for each reason, and one for a refused clear.

const DETECTED_AT = "2026-08-10T07:30:00Z";

const meta: Meta<typeof ContactImpersonationWarning> = {
  title: "Records/Contact/Impersonation warning",
  component: ContactImpersonationWarning,
  args: {
    contact: {
      ...populated.contact,
      impersonation_warning: {
        reason: "punycode_domain",
        detected_at: DETECTED_AT,
      },
    },
  },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof ContactImpersonationWarning>;

export const PunycodeDomain: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByText(englishCopy["contact.impersonation.title"]),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Looks fine" }),
    ).toBeVisible();
  },
};

export const DisplayNameOtherDomain: Story = {
  args: {
    contact: {
      ...populated.contact,
      impersonation_warning: {
        reason: "display_name_other_domain",
        detected_at: DETECTED_AT,
      },
    },
  },
};

export const ReasonNotRecorded: Story = {
  args: {
    contact: {
      ...populated.contact,
      impersonation_warning: { detected_at: DETECTED_AT },
    },
  },
};

export const ClearRefused: Story = {
  beforeEach: () =>
    installFetchStub({
      "POST /contacts/p-1/impersonation-warning/clear": permissionDenied,
    }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Looks fine" }));
    await expect(await canvas.findByRole("alert")).toBeVisible();
  },
};

export const PunycodeDomainDark: Story = {
  ...PunycodeDomain,
  globals: { theme: "dark" },
};
