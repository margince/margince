// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { screen, userEvent, within } from "storybook/test";
import { meFixture } from "../app/mefixture";
import { SEEDED_ASSIGNABLE_ROLES } from "./roles.testkit";
import { SignInMethodsCard } from "./sign-in-methods";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// Password is always on and says so. Each provider says whether its button is
// on the sign-in screen right now, and why not when it is missing: the verdict
// is the server's sign_in_screen, the reason the vendor app's state.

const meta: Meta<typeof SignInMethodsCard> = {
  title: "Settings/Company/Sign-in and apps/Sign-in methods",
  component: SignInMethodsCard,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof SignInMethodsCard>;

type Provider = { key: string; label: string; enabled: boolean };

const GOOGLE: Provider = {
  key: "google",
  label: "Continue with Google",
  enabled: true,
};
const MICROSOFT: Provider = {
  key: "microsoft",
  label: "Continue with Microsoft",
  enabled: true,
};

function app(provider: string, source: "stored" | "none", tenant?: string) {
  return {
    provider,
    configured: source === "stored",
    client_id: source === "stored" ? "client-id" : "",
    ...(tenant === undefined ? {} : { tenant }),
    source,
    redirect_uris: [],
  };
}

// The card reads the narrow authentication-policy projection and each vendor's
// app, so the story answers those routes rather than seeding a cache.
function Served({
  providers,
  onScreen,
  apps = "stored",
  children,
}: Readonly<{
  providers: Provider[];
  onScreen: string[];
  apps?: "stored" | "none";
  children: ReactNode;
}>) {
  installFetchStub({
    "GET /installation/authentication-policy": () =>
      jsonResponse({
        sign_in_providers: providers,
        sign_in_screen: onScreen,
        require_sso: false,
        require_mfa: false,
        oidc_group_role_map: {},
      }),
    "GET /installation/oauth-apps/google": () =>
      jsonResponse(app("google", apps)),
    "GET /installation/oauth-apps/microsoft": () =>
      jsonResponse(app("microsoft", apps, "9b0c3a71")),
    "GET /me": () =>
      jsonResponse(
        meFixture({
          allow: {
            installation_settings: ["read", "update"],
            authentication_policy: ["read", "update"],
          },
        }),
      ),
    "GET /users/assignable-roles": () =>
      jsonResponse({ roles: SEEDED_ASSIGNABLE_ROLES }),
  });
  return <StoryProviders>{children}</StoryProviders>;
}

/** Both apps set up, both buttons on the sign-in screen. */
export const OnScreen: Story = {
  render: () => (
    <Served providers={[GOOGLE, MICROSOFT]} onScreen={["google", "microsoft"]}>
      <SignInMethodsCard />
    </Served>
  ),
};

export const OnScreenDark: Story = {
  ...OnScreen,
  globals: { theme: "dark" },
};

/** Chosen but not drawn: neither app is set up yet, and each row says so. */
export const OffScreenNoApp: Story = {
  render: () => (
    <Served providers={[GOOGLE, MICROSOFT]} onScreen={[]} apps="none">
      <SignInMethodsCard />
    </Served>
  ),
};

export const OffScreenNoAppDark: Story = {
  ...OffScreenNoApp,
  globals: { theme: "dark" },
};

/** One provider switched off. It stays listed, so an admin can put it back. */
export const ProviderTurnedOff: Story = {
  render: () => (
    <Served
      providers={[{ ...GOOGLE, enabled: false }, MICROSOFT]}
      onScreen={["microsoft"]}
    >
      <SignInMethodsCard />
    </Served>
  ),
};

/** Turning a provider off asks for the typed phrase first. */
export const TurnOffConfirm: Story = {
  ...OnScreen,
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    await user.click(
      await within(canvasElement).findByRole("switch", {
        name: "Continue with Google",
      }),
    );
    await screen.findByRole("dialog");
  },
};

export const TurnOffConfirmDark: Story = {
  ...TurnOffConfirm,
  globals: { theme: "dark" },
};

/** No external provider configured, so password is the whole answer. */
export const PasswordOnly: Story = {
  render: () => (
    <Served providers={[]} onScreen={[]}>
      <SignInMethodsCard />
    </Served>
  ),
};
