// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { screen, userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { OAuthAppsCard } from "./oauth-apps-card";

// The card in each state an installation reaches. Either no app from any
// source, one stored, or one the installation's configuration supplies. Also a
// reader who may see the apps but not change them.

const meta: Meta<typeof OAuthAppsCard> = {
  title: "Settings/Company/Sign-in and apps/Company apps",
  component: OAuthAppsCard,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof OAuthAppsCard>;

type ConnectorApp = components["schemas"]["ConnectorApp"];
type RedirectUri = ConnectorApp["redirect_uris"][number];

const GOOGLE_URIS: RedirectUri[] = [
  {
    purpose: "sign_in",
    url: "https://api.brandt.example/v1/auth/oidc/google/callback",
  },
  {
    purpose: "mailbox_connect",
    url: "https://api.brandt.example/v1/connectors/gmail/callback",
  },
  {
    purpose: "calendar_connect",
    url: "https://api.brandt.example/v1/connectors/gcal/callback",
  },
];
const MICROSOFT_URIS: RedirectUri[] = [
  {
    purpose: "mailbox_connect",
    url: "https://api.brandt.example/v1/connectors/graph/callback",
  },
];

function absent(provider: ConnectorApp["provider"]): ConnectorApp {
  return {
    provider,
    configured: false,
    client_id: "",
    source: "none",
    redirect_uris: provider === "google" ? GOOGLE_URIS : MICROSOFT_URIS,
  };
}

const STORED_GOOGLE: ConnectorApp = {
  provider: "google",
  configured: true,
  client_id: "804215663017-brandtcrm.apps.googleusercontent.com",
  source: "stored",
  redirect_uris: GOOGLE_URIS,
};

const ENVIRONMENT_MICROSOFT: ConnectorApp = {
  provider: "microsoft",
  configured: true,
  client_id: "6d1e9b52-4c7a-4e0f-9a31-2b8f5c0d7e14",
  tenant: "9b0c3a71-58e2-4d16-b7a4-0f3e6c21d985",
  source: "environment",
  locks: [
    {
      key: "capture.microsoft_app",
      origin: { source: "env", name: "MARGINCE_GRAPH_CLIENT_ID" },
    },
  ],
  redirect_uris: MICROSOFT_URIS,
};

const ADMIN: GrantSpec = { capture_settings: ["read", "update"] };

function Served({
  google,
  microsoft,
  allow = ADMIN,
  children,
}: Readonly<{
  google: ConnectorApp;
  microsoft: ConnectorApp;
  allow?: GrantSpec;
  children: ReactNode;
}>) {
  installFetchStub({
    "GET /installation/oauth-apps/google": () => jsonResponse(google),
    "GET /installation/oauth-apps/microsoft": () => jsonResponse(microsoft),
    "GET /me": () => jsonResponse(meFixture({ allow })),
  });
  return <StoryProviders>{children}</StoryProviders>;
}

/** Neither vendor has an app, so nobody can connect mail or a calendar yet. */
export const NoneSetUp: Story = {
  render: () => (
    <Served google={absent("google")} microsoft={absent("microsoft")}>
      <OAuthAppsCard />
    </Served>
  ),
};

export const NoneSetUpDark: Story = {
  ...NoneSetUp,
  globals: { theme: "dark" },
};

/** One stored app is enough for the card to read Ready. */
export const OneReady: Story = {
  render: () => (
    <Served google={STORED_GOOGLE} microsoft={absent("microsoft")}>
      <OAuthAppsCard />
    </Served>
  ),
};

/** A stored Google app beside a Microsoft app this installation's
 * configuration supplies, pinned to one directory. */
export const BothReady: Story = {
  render: () => (
    <Served google={STORED_GOOGLE} microsoft={ENVIRONMENT_MICROSOFT}>
      <OAuthAppsCard />
    </Served>
  ),
};

export const BothReadyDark: Story = {
  ...BothReady,
  globals: { theme: "dark" },
};

/** The stored app's overflow menu, open: its verbs are words, not glyphs. */
export const StoredAppMenu: Story = {
  ...BothReady,
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    await user.click(
      await within(canvasElement).findByRole("button", {
        name: "More actions for the Google app",
      }),
    );
    await screen.findByRole("button", { name: "Remove app" });
  },
};

/** A reader who may see the apps but not change them: every verb is there and
 * refused, with the one sentence that says why. */
export const ReadOnly: Story = {
  render: () => (
    <Served
      google={absent("google")}
      microsoft={ENVIRONMENT_MICROSOFT}
      allow={{ capture_settings: ["read"] }}
    >
      <OAuthAppsCard />
    </Served>
  ),
};

export const ReadOnlyDark: Story = {
  ...ReadOnly,
  globals: { theme: "dark" },
};
