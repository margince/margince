// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { ConnectorsCard } from "./connectors";
import {
  jsonResponse,
  type RouteMap,
  StoryProviders,
  stubWithSession,
} from "./story-utils";

// Every story draws both panels, mail and the workspace's Telegram bot,
// because the card does.

type CaptureConnection = components["schemas"]["CaptureConnection"];

// Every story but the read-only one draws an administrator's view, so the
// Telegram panel shows the verbs its layout is judged with.
function stubAsBotManager(routes: RouteMap) {
  stubWithSession(routes, {
    channel_connection: ["read", "create", "update", "delete"],
  });
}

const gmailConnected: CaptureConnection = {
  id: "018f3a1b-0000-7000-8000-0000000000c1",
  provider: "gmail",
  status: "connected",
  scopes: [
    "https://www.googleapis.com/auth/gmail.readonly",
    "https://www.googleapis.com/auth/gmail.send",
  ],
  account_label: "lars@example.de",
  last_synced_at: "2026-07-23T09:30:00Z",
  next_sync_due_at: "2026-07-23T09:35:00Z",
  watch_expires_at: "2026-08-01T00:00:00Z",
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
  status: "reauth_required",
  scopes: ["read"],
  account_label: "lars@example.de",
  last_synced_at: "2026-07-20T08:00:00Z",
  last_sync_error_class: "auth",
};

const imapError: CaptureConnection = {
  id: "018f3a1b-0000-7000-8000-0000000000c3",
  provider: "imap",
  status: "error",
  scopes: [],
  last_synced_at: "2026-07-18T12:00:00Z",
  last_sync_error_class: "unreachable",
};

// IMAP is poll-only — there is no push subscription to renew, so
// watch_expires_at is always null for this provider. The card must read
// that null as "polled", never as an expired push renewal.
const imapPolled: CaptureConnection = {
  id: "018f3a1b-0000-7000-8000-0000000000c4",
  provider: "imap",
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
  publicOrigin?: { origin: string; reachable?: boolean | null },
) {
  return () => {
    stubAsBotManager({
      "GET /connectors": () =>
        jsonResponse({ data: connections, public_origin: publicOrigin }),
    });
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
  render: cardStory([gmailConnected, gcalReauth, imapError, imapPolled]),
};

// The badges are what tell a broken row from a healthy one, so their tones
// must hold apart in dark too.
export const MixedRowsDark: Story = {
  globals: { theme: "dark" },
  render: cardStory([gmailConnected, gcalReauth, imapError, imapPolled]),
};

// At 390 each row keeps its verbs at the end of its first line.
export const MixedRowsPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: cardStory([gmailConnected, gcalReauth, imapError, imapPolled]),
};

export const ImapPolled: Story = {
  render: cardStory([imapPolled]),
};

export const Empty: Story = {
  render: cardStory([]),
};

// Only an address that did not answer earns a badge.
export const OriginUnreachable: Story = {
  render: cardStory([gmailConnected], {
    origin: "http://localhost:8080",
    reachable: false,
  }),
};

// The import lives in a dialog, so the roster spends no preview on load.
export const ImportOldEmails: Story = {
  render: cardStory([{ ...gmailConnected, backfill: { state: "done" } }]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: /^Actions for Gmail/ }),
    );
    await userEvent.click(
      await canvas.findByRole("button", { name: "Import old emails" }),
    );
    await within(canvasElement.ownerDocument.body).findByRole("dialog");
  },
};

/** Open the card's header verb, so the picture is the dialog behind it. */
async function openAddDialog(canvasElement: HTMLElement) {
  const body = within(canvasElement.ownerDocument.body);
  await userEvent.click(
    await body.findByRole("button", { name: "Add connector" }),
  );
  await body.findByRole("dialog", { name: "Add connector" });
}

// The "Add connector" affordance (Task 1): ONE verb in the card's header,
// and the picks in the dialog it opens. What to check is that four providers
// each carrying a sentence read as four rows of one list — the shape they
// replaced was a strip of four buttons squeezed against a wrapping description,
// with Gmail drawn primary on a card that exists to report a roster rather than
// to push one mailbox.
export const AddConnectionDialogAllProviders: Story = {
  render: cardStory([]),
  play: async ({ canvasElement }) => {
    await openAddDialog(canvasElement);
  },
};

// The same dialog once Gmail is on the roster: three picks, not four. And in
// dark, where the muted description under each provider name is the ink most
// likely to collapse into the dialog's own ground under the accent lift.
export const AddConnectionDialogDark: Story = {
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
// its own further down the card. One `connect` mutation serves two surfaces, so
// both placements are worth seeing: the add dialog's provider picks, and a
// roster row's Reconnect.
function connectFailureStory(connections: CaptureConnection[], path: string) {
  return () => {
    stubAsBotManager({
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

export const AddConnectionFailed: Story = {
  render: connectFailureStory([gmailConnected], "graph"),
  play: async ({ canvasElement }) => {
    await openAddDialog(canvasElement);
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      // "Connect Outlook": the button's accessible name is
      // `connectors.connectProvider` filled with `connectors.provGraph`, and
      // that provider's word moved from Microsoft to Outlook.
      await body.findByRole("button", { name: "Connect Outlook" }),
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
    stubAsBotManager({
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

// A deployment that never wired mail capture answers 501 code:not_implemented
// (httperr.NotImplemented) — a calm, documented feature-off state, never an
// error card.
export const NotConfigured: Story = {
  render: () => {
    stubAsBotManager({
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
    stubAsBotManager({
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
