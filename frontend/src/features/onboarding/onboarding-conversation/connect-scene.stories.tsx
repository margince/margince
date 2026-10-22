// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { GrantSpec } from "../../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../../testing/story-utils";
import { ConnectScene, type ProviderAvailability } from "./connect-scene";
import type { MailProvider } from "./connect-vendor";
import "./conversation.css";

// The connect act's work surface on its own, without the act that owns its
// state. The act's stories (`Onboarding/Conversation/Connect act`) walk the
// route and consent outcomes. These hold the ask in view. They show the
// account picker and the inline Google ask with mail and calendar both ticked.
// They also show IMAP's form, an account whose company app waits on an
// administrator, the connected account and a roster read that failed.

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

// A Google account as the roster reports it once consent has returned.
const SEND = "https://www.googleapis.com/auth/gmail.send";
const BOOK = "https://www.googleapis.com/auth/calendar.events";
const connection = (provider: string, scopes: string[]) => ({
  id: provider,
  provider,
  status: "connected",
  scopes,
  account_label: "lars@example.org",
  backfill: { state: "none" },
});
const GOOGLE_LIVE = [
  connection("gmail", ["read", SEND]),
  connection("gcal", [BOOK]),
];

function scene(
  vendor: MailProvider | null,
  providers: ProviderAvailability[] = ALL_READY,
  allow: GrantSpec = READER,
  roster: () => Response = () => jsonResponse({ data: [], providers }),
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /connectors": roster,
      "POST /connectors/gmail/backfill/preview": () =>
        jsonResponse({
          window: "6m",
          estimated_messages: 900,
          computed_at: "2026-07-31T10:00:00Z",
        }),
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

/** The LinkedIn ask, opened: the profile address and why it is wanted. */
export const LinkedinAsk: Story = {
  render: NothingConnected.render,
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      await body.findByRole("button", { name: "Add profile" }),
    );
    await body.findByRole("dialog");
  },
};

/** The mailbox is live: the account as one row with what it brings in, the
 *  posture and the import. */
export const Connected: Story = {
  render: scene(null, ALL_READY, READER, () =>
    jsonResponse({ data: GOOGLE_LIVE, providers: ALL_READY }),
  ),
};

export const ConnectedDark: Story = {
  ...Connected,
  globals: { theme: "dark" },
};

/** The calendar was granted for reading only: the account needs attention,
 *  says why once, and offers one primary Reconnect. */
export const ConnectedNeedsReconnect: Story = {
  render: scene(null, ALL_READY, READER, () =>
    jsonResponse({
      data: [connection("gmail", ["read", SEND]), connection("gcal", ["read"])],
      providers: ALL_READY,
    }),
  ),
};

/** Mail without its calendar: the calendar line offers its own connect. */
export const ConnectedCalendarMissing: Story = {
  render: scene(null, ALL_READY, READER, () =>
    jsonResponse({
      data: [connection("gmail", ["read", SEND])],
      providers: ALL_READY,
    }),
  ),
};

/** The roster read failed: said in red, with the one recovery. */
export const RosterFailed: Story = {
  render: scene(null, ALL_READY, READER, () =>
    jsonResponse({ code: "internal" }, 500),
  ),
};
