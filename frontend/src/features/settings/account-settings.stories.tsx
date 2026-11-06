// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import { PageSaveBar, SavePageProvider } from "../../design-system/savepage";
import { meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "../../testing/story-utils";
import { AccountSettings } from "./account-settings";

type User = components["schemas"]["User"];

export function accountStory(
  roles: string[],
  routes: RouteMap = {},
  user: Partial<User> = {},
) {
  return () => {
    const me = meFixture({ roles });
    installFetchStub({
      "GET /me": () => jsonResponse({ ...me, user: { ...me.user, ...user } }),
      ...routes,
    });
    return (
      <StoryProviders>
        <div className="settings-stack">
          <SavePageProvider>
            <AccountSettings />
            <PageSaveBar />
          </SavePageProvider>
        </div>
      </StoryProviders>
    );
  };
}

const meta = {
  title: "Settings/You/Account/Account settings",
  component: AccountSettings,
  parameters: { layout: "padded" },
  excludeStories: ["accountStory"],
} satisfies Meta<typeof AccountSettings>;
export default meta;
type Story = StoryObj<typeof meta>;

// Profile, sign-in, language, this browser and scheduled emails, each under its own title.
export const Admin: Story = { render: accountStory(["admin"]) };

// A rep sees the same panels: every row here is the reader's own.
export const Rep: Story = { render: accountStory(["rep"]) };

export const AdminDark: Story = {
  globals: { theme: "dark" },
  render: accountStory(["admin"]),
};

export const RepPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: accountStory(["rep"]),
};

// A refused save keeps the page in the current language and says so on the save bar.
export const LanguageRefused: Story = {
  render: accountStory(["rep"], {
    "PUT /me/locale": () =>
      new Response(
        JSON.stringify({ status: 503, code: "service_unavailable" }),
        {
          status: 503,
          headers: { "content-type": "application/problem+json" },
        },
      ),
  }),
  play: async ({ canvasElement }) => {
    const page = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      await page.findByRole("combobox", { name: "Language" }),
    );
    await userEvent.click(await page.findByRole("option", { name: "Deutsch" }));
    await userEvent.click(await page.findByRole("button", { name: "Save" }));
    await page.findAllByRole("alert");
  },
};

export const LanguageRefusedPhone: Story = {
  ...LanguageRefused,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
