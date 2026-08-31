// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { LocaleProvider } from "../i18n";
import { SignInMethodsCard } from "./sign-in-methods";
import { installFetchStub, jsonResponse } from "./story-utils";

// Password is present in every state and flippable in none: there is no value
// of the setting that removes it, so the row shows a control that exists and
// refuses rather than one that is missing. The three states differ only in what
// the DEPLOYMENT composed and what the admin chose of it.

const meta: Meta<typeof SignInMethodsCard> = {
  title: "Settings/Admin settings/General/Sign-in methods",
  component: SignInMethodsCard,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof SignInMethodsCard>;

// The card reads the shared installation-settings query, so the story answers
// that route rather than seeding a cache: data written with setQueryData is
// stale on arrival and the mount's background refetch would reach the network.
function Served({
  providers,
  children,
}: Readonly<{
  providers: { key: string; label: string; enabled: boolean }[];
  children: ReactNode;
}>) {
  installFetchStub({
    "GET /installation/settings": () =>
      jsonResponse({
        name: "Brandt Automotive",
        // Not a zone name: this card never reads the field, and zone literals
        // are reserved to the module that owns them.
        timezone: "",
        base_currency: "EUR",
        base_language: "de",
        base_currency_locked: false,
        max_upload_bytes: 25_000_000,
        sign_in_providers: providers,
      }),
    "GET /me": () =>
      jsonResponse({
        user: { email: "admin@brandt.example" },
        authorization: { installation_settings: ["read", "update"] },
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

/** A provider the deployment configured and the admin offers. */
export const ProviderEnabled: Story = {
  render: () => (
    <Served providers={[{ key: "google", label: "Google", enabled: true }]}>
      <SignInMethodsCard />
    </Served>
  ),
};

/** The same provider switched off. It stays LISTED — the credentials are still
 * there, and an admin who cannot see it cannot put it back. */
export const ProviderDisabled: Story = {
  render: () => (
    <Served providers={[{ key: "google", label: "Google", enabled: false }]}>
      <SignInMethodsCard />
    </Served>
  ),
};

/** No external provider configured, so password is the whole answer. */
export const PasswordOnly: Story = {
  render: () => (
    <Served providers={[]}>
      <SignInMethodsCard />
    </Served>
  ),
};
