// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { GrantSpec } from "../../app/mefixture";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../story-utils";
import { ConnectScene, type ProviderAvailability } from "./connect-scene";
import type { MailProvider } from "./connect-vendor";
import "./conversation.css";

// The connect act's work surface on its own, without the act that owns its
// state. The act's stories (`Onboarding/Conversation/Connect act`) walk the
// route and consent outcomes; these hold the ask in view: the account picker,
// the inline Google ask with mail and calendar both ticked, IMAP's form, and
// an account whose company app waits on an administrator.

const ALL_READY: ProviderAvailability[] = [
  { provider: "gmail", reason: "ready" },
  { provider: "graph", reason: "ready" },
  { provider: "imap", reason: "ready" },
];

const meta: Meta<typeof ConnectScene> = {
  title: "Onboarding/Conversation/Connect scene",
  component: ConnectScene,
};
export default meta;
type Story = StoryObj<typeof ConnectScene>;

// No company Google app: the account waits on an administrator, the others
// are open now.
const GOOGLE_MISSING: ProviderAvailability[] = [
  { provider: "gmail", reason: "app_missing" },
  { provider: "graph", reason: "ready" },
  { provider: "imap", reason: "ready" },
];

const READER: GrantSpec = { channel_connection: ["read", "create"] };

// The grant the app endpoints enforce, so the step offers the form in place.
const ADMIN: GrantSpec = { ...READER, capture_settings: ["read", "update"] };

function scene(
  vendor: MailProvider | null,
  providers: ProviderAvailability[] = ALL_READY,
  allow: GrantSpec = READER,
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /connectors": () => jsonResponse({ data: [], providers }),
      "GET /installation/oauth-apps/google": () =>
        jsonResponse({
          provider: "google",
          configured: false,
          client_id: "",
          source: "none",
          redirect_uris: [
            {
              purpose: "mailbox_connect",
              url: "https://crm.example.com/v1/connectors/google/callback",
            },
          ],
        }),
      "GET /installation/oauth-apps/microsoft": () =>
        jsonResponse({
          provider: "microsoft",
          configured: false,
          client_id: "",
          source: "none",
          redirect_uris: [
            {
              purpose: "mailbox_connect",
              url: "https://crm.example.com/v1/connectors/microsoft/callback",
            },
          ],
        }),
    });
    return (
      <StoryProviders>
        <ConnectScene
          vendor={vendor}
          onVendorChange={() => {}}
          askBusy={false}
          onAskBusyChange={() => {}}
          onImapDone={() => {}}
          returnPanel={null}
          onFinish={() => {}}
          finishing={false}
          wantsOvernight
          onWantsOvernightChange={() => {}}
          overnightFailed={false}
          linkedinStatus="pending"
          onLinkedinSave={() => {}}
          onLinkedinSkip={() => {}}
          linkedinPending={false}
          linkedinError={null}
        />
      </StoryProviders>
    );
  };
}

/** Nothing picked yet: the three accounts, and the network section below. */
export const NothingConnected: Story = { render: scene(null) };

/** Google picked: mail and calendar ticked, one press for both consents. */
export const GoogleAsk: Story = { render: scene("google") };

export const GoogleAskDark: Story = {
  render: scene("google"),
  globals: { theme: "dark" },
};

/** Another host: IMAP's form inline, mail only. */
export const ImapAsk: Story = { render: scene("imap") };

export const ImapAskDark: Story = {
  render: scene("imap"),
  globals: { theme: "dark" },
};

/** Google picked, no company app, a reader who cannot register one: the
 *  request to copy for the administrator, and the way to another provider. */
export const GoogleAppMissing: Story = {
  render: scene("google", GOOGLE_MISSING),
};

export const GoogleAppMissingDark: Story = {
  render: scene("google", GOOGLE_MISSING),
  globals: { theme: "dark" },
};

/** The same wait for an administrator: the app form opens in place, because
 *  the onboarding gate would bounce a link to Settings. */
export const GoogleAppMissingAdmin: Story = {
  render: scene("google", GOOGLE_MISSING, ADMIN),
};

export const GoogleAppMissingAdminDark: Story = {
  render: scene("google", GOOGLE_MISSING, ADMIN),
  globals: { theme: "dark" },
};

/** A stored Microsoft app whose client secret no longer opens: the request
 *  asks for a new secret, never a second app. */
export const MicrosoftAppUnusable: Story = {
  render: scene("microsoft", [
    { provider: "gmail", reason: "ready" },
    { provider: "graph", reason: "app_unusable" },
    { provider: "imap", reason: "ready" },
  ]),
};

export const MicrosoftAppUnusableDark: Story = {
  ...MicrosoftAppUnusable,
  globals: { theme: "dark" },
};

/** The unusable app for an administrator: replace its secret in place. */
export const MicrosoftAppUnusableAdmin: Story = {
  render: scene(
    "microsoft",
    [
      { provider: "gmail", reason: "ready" },
      { provider: "graph", reason: "app_unusable" },
      { provider: "imap", reason: "ready" },
    ],
    ADMIN,
  ),
};

export const MicrosoftAppUnusableAdminDark: Story = {
  ...MicrosoftAppUnusableAdmin,
  globals: { theme: "dark" },
};

const MICROSOFT_MISSING: ProviderAvailability[] = [
  { provider: "gmail", reason: "ready" },
  { provider: "graph", reason: "app_missing" },
  { provider: "imap", reason: "ready" },
];

/** Microsoft picked with no company app, for a member: the request to copy. */
export const MicrosoftAppMissing: Story = {
  render: scene("microsoft", MICROSOFT_MISSING),
};

export const MicrosoftAppMissingDark: Story = {
  ...MicrosoftAppMissing,
  globals: { theme: "dark" },
};

/** The same wait for an administrator: register the app in place. */
export const MicrosoftAppMissingAdmin: Story = {
  render: scene("microsoft", MICROSOFT_MISSING, ADMIN),
};

export const MicrosoftAppMissingAdminDark: Story = {
  ...MicrosoftAppMissingAdmin,
  globals: { theme: "dark" },
};
