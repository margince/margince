// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { meFixture } from "../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { AccountSettings } from "./account-settings";

function story(roles: string[]) {
  return () => {
    installFetchStub({
      "GET /me": () => jsonResponse(meFixture({ roles })),
    });
    return (
      <StoryProviders>
        <div className="settings-stack">
          <AccountSettings />
        </div>
      </StoryProviders>
    );
  };
}

const meta = {
  title: "Settings/You/Account/Account settings",
  component: AccountSettings,
  parameters: { layout: "padded" },
} satisfies Meta<typeof AccountSettings>;
export default meta;
type Story = StoryObj<typeof meta>;

// Profile, security, and language and appearance, each under its own title.
export const Admin: Story = { render: story(["admin"]) };

// A rep sees the same three panels: every row here is the reader's own.
export const Rep: Story = { render: story(["rep"]) };

export const AdminDark: Story = {
  globals: { theme: "dark" },
  render: story(["admin"]),
};

export const RepPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(["rep"]),
};
