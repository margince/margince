// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import { PageSaveBar, SavePageProvider } from "../../design-system/savepage";
import { en } from "../../i18n/en";
import { setSingleKeyShortcutsOn } from "../../lib/shortcuts";
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
      "GET /me/sessions": () => jsonResponse({ sessions: [] }),
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

// The reader turned single-key shortcuts off for this browser.
export const SingleKeysOff: Story = {
  render: () => {
    setSingleKeyShortcutsOn(false);
    return accountStory(["rep"])();
  },
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
        JSON.stringify({
          status: 503,
          code: "service_unavailable",
          detail: "The language could not be saved.",
        }),
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
    const bar = await page.findByRole("region", {
      name: en["saveBar.unsaved"],
    });
    await expect(await within(bar).findByRole("alert")).toHaveTextContent(
      "The language could not be saved.",
    );
  },
};

export const LanguageRefusedPhone: Story = {
  ...LanguageRefused,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

const PHONE_AGENT =
  "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1";
const LAPTOP_AGENT =
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36";

// This browser is marked and has no button; the phone can be signed out.
export const SignedInDevices: Story = {
  render: accountStory(["rep"], {
    "GET /me/sessions": () =>
      jsonResponse({
        sessions: [
          {
            id: "00000000-0000-4000-8000-0000000000a1",
            user_agent: LAPTOP_AGENT,
            signed_in_at: "2026-11-01T08:00:00Z",
            last_active_at: "2026-11-02T09:30:00Z",
            current: true,
          },
          {
            id: "00000000-0000-4000-8000-0000000000a2",
            user_agent: PHONE_AGENT,
            signed_in_at: "2026-10-20T18:10:00Z",
            last_active_at: "2026-10-30T07:45:00Z",
            current: false,
          },
        ],
      }),
  }),
};
