// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { meFixture } from "../testing/mefixture";
import { installFetchStub, jsonResponse } from "../testing/story-utils";
import { SEEDED_ASSIGNABLE_ROLES } from "./roles.testkit";
import { SignInMethodsCard } from "./sign-in-methods";

// Password is present in every state and flippable in none: there is no value
// of the setting that removes it, so the row shows a control that exists and
// refuses rather than one that is missing. The three states differ only in what
// the DEPLOYMENT composed and what the admin chose of it.

const meta: Meta<typeof SignInMethodsCard> = {
  title: "Settings/Company/Sign-in and apps/Sign-in methods",
  component: SignInMethodsCard,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof SignInMethodsCard>;

// The card reads the narrow authentication-policy projection, so the story
// answers that route rather than seeding a cache: data written with
// setQueryData is stale on arrival and the mount's background refetch would
// reach the network.
function Served({
  providers,
  groupRoleMap = {},
  requireMfa = false,
  ownMfaConfirmed = true,
  appSource = "stored",
  children,
}: Readonly<{
  providers: { key: string; label: string; enabled: boolean }[];
  groupRoleMap?: Record<string, string>;
  requireMfa?: boolean;
  ownMfaConfirmed?: boolean;
  appSource?: "stored" | "none";
  children: ReactNode;
}>) {
  const app = (provider: string) => () =>
    jsonResponse({
      provider,
      configured: appSource === "stored",
      source: appSource,
      client_id: appSource === "stored" ? "client-1" : "",
      redirect_uris: [],
    });
  installFetchStub({
    "GET /installation/oauth-apps/google": app("google"),
    "GET /installation/oauth-apps/microsoft": app("microsoft"),
    "GET /installation/authentication-policy": () =>
      jsonResponse({
        sign_in_providers: providers,
        require_sso: false,
        require_mfa: requireMfa,
        oidc_group_role_map: groupRoleMap,
      }),
    "GET /me": () =>
      jsonResponse(
        meFixture({
          allow: {
            installation_settings: ["read", "update"],
            authentication_policy: ["read", "update"],
            capture_settings: ["read"],
          },
        }),
      ),
    "GET /me/mfa": () =>
      jsonResponse({
        enrolled: ownMfaConfirmed,
        confirmed: ownMfaConfirmed,
        recovery_codes_left: ownMfaConfirmed ? 10 : 0,
      }),
    "GET /users/assignable-roles": () =>
      jsonResponse({ roles: SEEDED_ASSIGNABLE_ROLES }),
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

/** No OAuth app exists for either vendor yet. Only the provider switched on
 * says the sign-in screen does not show it; the one switched off keeps its hint. */
export const ProviderWithoutApp: Story = {
  render: () => (
    <Served
      providers={[
        { key: "google", label: "Continue with Google", enabled: true },
        { key: "microsoft", label: "Continue with Microsoft", enabled: false },
      ]}
      appSource="none"
    >
      <SignInMethodsCard />
    </Served>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText(/not on the sign-in screen yet/i);
    await expect(
      canvas.getAllByText(/not on the sign-in screen yet/i),
    ).toHaveLength(1);
  },
};

/** A drafted mapping: the grant-only warning appears with the first row, and
 * Save stands only while the draft differs from what is stored. */
export const GroupDraft: Story = {
  render: () => (
    <Served providers={[{ key: "google", label: "Google", enabled: true }]}>
      <SignInMethodsCard />
    </Served>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const add = await canvas.findByRole("button", { name: /add group/i });
    await waitFor(() => expect(add).toBeEnabled());
    await userEvent.click(add);
    await canvas.findByText(/grants the mapped roles and never removes any/i);
    await canvas.findByRole("button", { name: /save group grants/i });
  },
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

/** IdP groups mapped onto roles, with the grant-only warning standing beside
 * the rows. The empty-map states above show the "nothing extra is granted"
 * line instead. */
export const GroupsGrantRoles: Story = {
  render: () => (
    <Served
      providers={[{ key: "google", label: "Google", enabled: true }]}
      groupRoleMap={{ "crm-admins": "admin", "field-sales": "rep" }}
    >
      <SignInMethodsCard />
    </Served>
  ),
};

/** On a phone each grant folds: the group over its role, the remove verb at
 * the end of the group's line. */
export const GroupsGrantRolesPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => (
    <Served
      providers={[{ key: "google", label: "Google", enabled: true }]}
      groupRoleMap={{
        "crm-admins": "admin",
        "sales-dach-benelux-regional-field-and-channel-partner-enablement":
          "rep",
      }}
    >
      <SignInMethodsCard />
    </Served>
  ),
};

/** A second factor required of every member. Turning it off saves at once. */
export const SecondFactorRequired: Story = {
  render: () => (
    <Served
      providers={[{ key: "google", label: "Google", enabled: true }]}
      requireMfa
    >
      <SignInMethodsCard />
    </Served>
  ),
};

/** Turning the requirement on asks first. This admin holds no factor of their
 * own, so the dialog says they are asked to set one up too. */
export const SecondFactorConfirm: Story = {
  render: () => (
    <Served
      providers={[{ key: "google", label: "Google", enabled: true }]}
      ownMfaConfirmed={false}
    >
      <SignInMethodsCard />
    </Served>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const control = await canvas.findByRole("switch", {
      name: /require second factor/i,
    });
    // The switch draws before the session grants arrive, and a disabled one
    // ignores the click.
    await waitFor(() => expect(control).toBeEnabled());
    await userEvent.click(control);
    // The dialog portals to the body, outside the canvas.
    await within(canvasElement.ownerDocument.body).findByText(
      /you have not set up two-step sign-in/i,
    );
  },
};
