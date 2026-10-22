// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import type { GrantSpec } from "../../testing/mefixture";
import {
  jsonResponse,
  type RouteMap,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { ConnectorsCard } from "./connectors";

// ConnectorsCard stories for the fe-uat render gate. They cover a healthy
// connection, a reauth-needed one (the reconnect affordance), a sync-error one,
// the empty state, and a load failure. All use the same GET /connectors shape
// the unit tests (connectors.test.tsx) exercise.
//
// One ProviderRow per account, with its services listed under it. The
// workspace's Telegram bot has its own stories on Integrations.

type CaptureConnection = components["schemas"]["CaptureConnection"];

// The sharing plate under the mail card reads the company's capture posture.
function stubAsMember(routes: RouteMap, allow: GrantSpec = {}) {
  stubWithSession(
    {
      "GET /capture/settings": () =>
        jsonResponse({
          auto_enrich: true,
          mail_sharing: true,
          shared_posture_allowed: false,
          signature_enrich: true,
        }),
      ...routes,
    },
    allow,
  );
}

const gmailConnected: CaptureConnection = {
  id: "018f3a1b-0000-7000-8000-0000000000c1",
  provider: "gmail",
  can_backfill: true,
  status: "connected",
  scopes: [
    "https://www.googleapis.com/auth/gmail.readonly",
    "https://www.googleapis.com/auth/gmail.send",
  ],
  account_label: "lars@example.de",
  last_synced_at: "2026-07-23T09:30:00Z",
  next_sync_due_at: "2026-07-23T09:35:00Z",
  watch_expires_at: "2026-08-01T00:00:00Z",
  // A finished import, so the Past email service draws its figures with no
  // request against a route this story never stubs.
  backfill: {
    state: "done",
    counts: { captured: 842, contacts_created: 96, companies_created: 21 },
  },
};

// The mailbox connected before Margince asked for the send scope: capturing
// happily, and permanently unable to send until it is reconnected.
const gmailNoSendGrant: CaptureConnection = {
  ...gmailConnected,
  scopes: ["https://www.googleapis.com/auth/gmail.readonly"],
};

const gcalReauth: CaptureConnection = {
  id: "018f3a1b-0000-7000-8000-0000000000c2",
  provider: "gcal",
  can_backfill: false,
  status: "reauth_required",
  scopes: ["read"],
  account_label: "lars@example.de",
  last_synced_at: "2026-07-20T08:00:00Z",
  last_sync_error_class: "auth",
};

const imapError: CaptureConnection = {
  id: "018f3a1b-0000-7000-8000-0000000000c3",
  provider: "imap",
  can_backfill: false,
  status: "error",
  scopes: [],
  last_synced_at: "2026-07-18T12:00:00Z",
  last_sync_error_class: "unreachable",
};

// IMAP is poll-only, so watch_expires_at is always null for it.
const imapPolled: CaptureConnection = {
  id: "018f3a1b-0000-7000-8000-0000000000c4",
  provider: "imap",
  can_backfill: false,
  status: "connected",
  scopes: [],
  account_label: "sales@example.org",
  last_synced_at: "2026-07-23T09:00:00Z",
  next_sync_due_at: "2026-07-23T09:15:00Z",
  watch_expires_at: null,
  backfill: { state: "none" },
};

function cardStory(
  connections: CaptureConnection[],
  providers: components["schemas"]["CaptureProviderAvailability"][] = [],
  allow: GrantSpec = {},
) {
  return () => {
    stubAsMember(
      {
        "GET /connectors": () => jsonResponse({ data: connections, providers }),
      },
      allow,
    );
    return (
      <StoryProviders>
        <ConnectorsCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof ConnectorsCard> = {
  title: "Settings/You/Connections/Connectors",
  component: ConnectorsCard,
};
export default meta;
type Story = StoryObj<typeof ConnectorsCard>;

export const Connected: Story = {
  render: cardStory([gmailConnected]),
};

export const NeedsReconnect: Story = {
  render: cardStory([gcalReauth]),
};

export const CannotSend: Story = {
  render: cardStory([gmailNoSendGrant]),
};

export const SyncError: Story = {
  render: cardStory([imapError]),
};

export const MixedRows: Story = {
  render: cardStory([gmailConnected, gcalReauth, imapError]),
};

// The statuses in dark, side by side: the tones that separate "connected",
// "needs reconnecting" and "erroring" must hold apart in dark too.
export const MixedRowsDark: Story = {
  globals: { theme: "dark" },
  render: cardStory([gmailConnected, gcalReauth, imapError]),
};

// The roster at 390px: each service's verbs drop under its sentence.
export const MixedRowsPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: cardStory([gmailConnected, gcalReauth, imapError]),
};

export const ImapPolled: Story = {
  render: cardStory([imapPolled]),
};

// No mailbox at all. The card leads with the steps once on top, then a tile per
// vendor with its own Connect, the other-host row and the sharing plate.
export const Empty: Story = {
  render: cardStory([]),
};

export const EmptyDark: Story = {
  globals: { theme: "dark" },
  render: cardStory([]),
};

const MICROSOFT_UNWIRED: components["schemas"]["CaptureProviderAvailability"][] =
  [
    { provider: "gmail", reason: "ready" },
    { provider: "graph", reason: "app_missing" },
    { provider: "imap", reason: "ready" },
  ];

// Microsoft's company app is not registered here. Its tile keeps its place,
// says Not set up and names who turns it on, with no Connect to press.
export const EmptyOneVendorUnwired: Story = {
  render: cardStory([], MICROSOFT_UNWIRED),
};

export const EmptyOneVendorUnwiredDark: Story = {
  globals: { theme: "dark" },
  render: EmptyOneVendorUnwired.render,
};

// The same tile for someone who can register the app: the way to Sign-in and
// apps stands where Connect would.
export const EmptyOneVendorUnwiredAdmin: Story = {
  render: cardStory([], MICROSOFT_UNWIRED, { capture_settings: ["update"] }),
};

export const EmptyOneVendorUnwiredAdminDark: Story = {
  globals: { theme: "dark" },
  render: EmptyOneVendorUnwiredAdmin.render,
};

/** Open the card's header verb, so the picture is the dialog behind it. */
async function openAddDialog(canvasElement: HTMLElement) {
  const body = within(canvasElement.ownerDocument.body);
  await userEvent.click(
    await body.findByRole("button", { name: "Add account" }),
  );
  await body.findByRole("dialog", { name: "Connect mail and calendar" });
}

// "Add account" opens the connect dialog on its first step: the account, as
// three plates. The dialog's own steps have their stories beside it
// (connect-flow.stories.tsx); these show it over the card it starts from.
export const AddAccountDialog: Story = {
  render: cardStory([imapPolled]),
  play: async ({ canvasElement }) => {
    await openAddDialog(canvasElement);
  },
};

// Gmail is on the roster and its calendar is not, so Google stays a pick: its
// calendar is what is left to add. In dark, where the muted description under
// each plate is the ink most likely to collapse into the dialog's ground.
export const AddAccountDialogDark: Story = {
  globals: { theme: "dark" },
  render: cardStory([gmailConnected]),
  play: async ({ canvasElement }) => {
    await openAddDialog(canvasElement);
  },
};

// One mailbox connected, dialog closed: the header carries the verb and the
// body carries only the roster.
export const OneConnectedWithHeaderVerb: Story = {
  render: cardStory([gmailConnected]),
};

// A refused connect reports itself where the press happened, not in a band of
// its own further down the card. That is the connect dialog, and a roster row's
// Reconnect.
function connectFailureStory(connections: CaptureConnection[], path: string) {
  return () => {
    stubAsMember({
      "GET /connectors": () => jsonResponse({ data: connections }),
      [`POST /connectors/${path}/connect`]: () =>
        jsonResponse(
          { title: "Bad Gateway", detail: "The provider did not answer." },
          502,
        ),
    });
    return (
      <StoryProviders>
        <ConnectorsCard />
      </StoryProviders>
    );
  };
}

export const AddAccountFailed: Story = {
  render: connectFailureStory([gmailConnected], "graph"),
  play: async ({ canvasElement }) => {
    await openAddDialog(canvasElement);
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(body.getByRole("radio", { name: /^Microsoft 365/ }));
    await userEvent.click(body.getByRole("button", { name: "Continue" }));
    await userEvent.click(body.getByRole("button", { name: "Continue" }));
    await userEvent.click(
      body.getByRole("button", { name: "Continue to Microsoft 365" }),
    );
    await body.findByRole("alert");
  },
};

export const ReconnectFailed: Story = {
  render: connectFailureStory([gcalReauth], "gcal"),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: /Reconnect/ }),
    );
    await canvas.findByRole("alert");
  },
};

export const LoadFailed: Story = {
  render: () => {
    stubAsMember({
      "GET /connectors": () =>
        jsonResponse({ title: "Internal Server Error", detail: "boom" }, 500),
    });
    return (
      <StoryProviders>
        <ConnectorsCard />
      </StoryProviders>
    );
  },
};

// The roster read in flight: the card holds the room its rows will take.
export const Loading: Story = {
  render: () => {
    stubAsMember({ "GET /connectors": () => new Promise(() => {}) });
    return (
      <StoryProviders>
        <ConnectorsCard />
      </StoryProviders>
    );
  },
};

// A deployment that never wired mail capture answers 501 code:not_implemented
// (httperr.NotImplemented) — a calm, documented feature-off state, never an
// error card.
export const NotConfigured: Story = {
  render: () => {
    stubAsMember({
      "GET /connectors": () => jsonResponse({ code: "not_implemented" }, 501),
    });
    return (
      <StoryProviders>
        <ConnectorsCard />
      </StoryProviders>
    );
  },
};

// The OAuth return outcome (Task 2): the backend lands the callback on
// #/settings/connections/{outcome}; the card reads id2 off the route and
// renders a dismissible inline note. Each story sets the hash before
// mounting, exactly like the fetch stub is wired before mount.
function outcomeStory(outcome: string, connections: CaptureConnection[]) {
  return () => {
    globalThis.location.hash = `#/settings/connections/${outcome}`;
    stubAsMember({
      "GET /connectors": () => jsonResponse({ data: connections }),
    });
    return (
      <StoryProviders>
        <ConnectorsCard />
      </StoryProviders>
    );
  };
}

export const OAuthDenied: Story = {
  render: outcomeStory("denied", []),
};

export const OAuthError: Story = {
  render: outcomeStory("error", []),
};

export const OAuthOk: Story = {
  render: outcomeStory("ok", [gmailConnected]),
};
