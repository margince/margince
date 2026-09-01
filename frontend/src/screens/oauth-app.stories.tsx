// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { LocaleProvider } from "../i18n";
import { OAuthAppCard } from "./oauth-app";
import { installFetchStub, jsonResponse } from "./story-utils";

// The card has THREE states, and the middle one is why it was rewritten: an app
// the DEPLOYMENT supplies is neither a stored app nor the absence of one, and
// reporting it as absent told operators Gmail could not be connected on
// installations where it demonstrably could.

const meta: Meta<typeof OAuthAppCard> = {
  title: "Settings/Admin settings/General/Google app",
  component: OAuthAppCard,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof OAuthAppCard>;

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
    // The RESOLVED path, not the template: installFetchStub matches a route key
    // against the request's pathname by exact string, and api.GET has already
    // substituted the provider by the time it is called. A key still carrying
    // `{provider}` matches nothing and every story renders the fallback.
    "GET /installation/oauth-apps/google": () => jsonResponse(app),
    "GET /me": () =>
      jsonResponse({
        user: { email: "admin@brandt.example" },
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

/** An app saved through this surface, which wins over the deployment's. */
export const Stored: Story = {
  render: () => (
    <Served
      app={{
        configured: true,
        client_id: CLIENT_ID,
        source: "stored",
        redirect_uris: URIS,
      }}
    >
      <OAuthAppCard provider="google" />
    </Served>
  ),
};

/** The deployment's own credentials, used because nothing is stored. */
export const FromEnvironment: Story = {
  render: () => (
    <Served
      app={{
        configured: true,
        client_id: CLIENT_ID,
        source: "environment",
        redirect_uris: URIS,
      }}
    >
      <OAuthAppCard provider="google" />
    </Served>
  ),
};

/** Neither source can supply one, so neither flow can run — but the URIs are
 * still listed, because this is the state an operator is in while creating the
 * OAuth client that will end it. */
export const Absent: Story = {
  render: () => (
    <Served
      app={{
        configured: false,
        client_id: "",
        source: "none",
        redirect_uris: URIS,
      }}
    >
      <OAuthAppCard provider="google" />
    </Served>
  ),
};
