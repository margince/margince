// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { LocaleProvider } from "../i18n";
import { installFetchStub, jsonResponse } from "../testing/story-utils";
import {
  OAuthAppDrawer,
  RedirectAddressesDialog,
  RemoveOAuthAppDialog,
} from "./oauth-app-drawer";

// The setup drawer and the two dialogs a stored app's menu opens. Google and
// Microsoft are both drawn because they differ in what the drawer asks: only
// Microsoft has a directory to pin.

const meta: Meta<typeof OAuthAppDrawer> = {
  title: "Settings/Company/Sign-in and apps/Connector app setup",
  component: OAuthAppDrawer,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof OAuthAppDrawer>;

const CLIENT_ID = "111-abc.apps.googleusercontent.com";
const URIS = [
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

function Served({
  app,
  children,
}: Readonly<{ app: unknown; children: ReactNode }>) {
  installFetchStub({
    // The resolved path, not the template: installFetchStub matches a route key
    // against the request's pathname by exact string. api.GET has already
    // substituted the provider, so a key still carrying `{provider}` matches
    // nothing and every story renders the fallback.
    "GET /installation/oauth-apps/google": () => jsonResponse(app),
    "GET /installation/oauth-apps/microsoft": () => jsonResponse(app),
    "GET /me": () =>
      jsonResponse({
        user: { email: "admin@brandt.example" },
        workspace_name: "Brandt Automotive GmbH",
        authorization: { capture_settings: ["read", "update"] },
      }),
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
}

const ENTRA_ID = "11111111-2222-3333-4444-555555555555";
const GRAPH_URIS = [
  {
    purpose: "sign_in",
    url: "https://api.brandt.example/v1/auth/oidc/microsoft/callback",
  },
  {
    purpose: "mailbox_connect",
    url: "https://api.brandt.example/v1/connectors/graph/callback",
  },
  {
    purpose: "calendar_connect",
    url: "https://api.brandt.example/v1/connectors/graphcal/callback",
  },
];

const noop = () => undefined;

/** The setup drawer on an installation with no Google app: three steps, the
 * redirect table, and Save waiting for both halves of the pair. */
export const SetupDrawerGoogle: Story = {
  render: () => (
    <Served
      app={{
        configured: false,
        client_id: "",
        source: "none",
        redirect_uris: URIS,
      }}
    >
      <OAuthAppDrawer provider="google" open onClose={noop} />
    </Served>
  ),
};

export const SetupDrawerGoogleDark: Story = {
  ...SetupDrawerGoogle,
  globals: { theme: "dark" },
};

/** Replacing a stored Microsoft app: the same drawer, opened with the client
 * id and directory in place, so a secret rotation is one field. */
export const ReplaceDrawerMicrosoft: Story = {
  render: () => (
    <Served
      app={{
        configured: true,
        client_id: ENTRA_ID,
        tenant: "99999999-8888-7777-6666-555555555555",
        source: "stored",
        redirect_uris: GRAPH_URIS,
      }}
    >
      <OAuthAppDrawer provider="microsoft" open onClose={noop} />
    </Served>
  ),
};

export const ReplaceDrawerMicrosoftDark: Story = {
  ...ReplaceDrawerMicrosoft,
  globals: { theme: "dark" },
};

const STORED_GOOGLE = {
  configured: true,
  client_id: CLIENT_ID,
  source: "stored",
  redirect_uris: URIS,
};

/** Removing a stored app: the impact list, then the typed phrase that unlocks
 * Remove app. */
export const RemoveGoogle: Story = {
  render: () => (
    <Served app={STORED_GOOGLE}>
      <RemoveOAuthAppDialog provider="google" open onClose={noop} />
    </Served>
  ),
};

export const RemoveGoogleDark: Story = {
  ...RemoveGoogle,
  globals: { theme: "dark" },
};

/** A stored app's redirect addresses on their own, from its menu. */
export const RedirectAddressesGoogle: Story = {
  render: () => (
    <Served app={STORED_GOOGLE}>
      <RedirectAddressesDialog provider="google" open onClose={noop} />
    </Served>
  ),
};

export const RedirectAddressesGoogleDark: Story = {
  ...RedirectAddressesGoogle,
  globals: { theme: "dark" },
};
