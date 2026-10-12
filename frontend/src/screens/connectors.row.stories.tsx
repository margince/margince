// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { Panel } from "../design-system/panel";
import { SettingList } from "../design-system/settingrow";
import { ConnectorRow } from "./connectors.row";
import { ConnectionDrawer } from "./connectors.settings";
import { jsonResponse, StoryProviders, stubWithSession } from "./story-utils";
import "./connectors.css";

// One roster row per state. A healthy row carries no badge; a broken one says
// how and offers Reconnect. Every row keeps its menu and chevron at one x.

type CaptureConnection = components["schemas"]["CaptureConnection"];

const gmail: CaptureConnection = {
  id: "018f3a1b-0000-7000-8000-0000000000c1",
  provider: "gmail",
  status: "connected",
  scopes: [
    "https://www.googleapis.com/auth/gmail.readonly",
    "https://www.googleapis.com/auth/gmail.send",
  ],
  account_label: "lars.vandermeulen-schoenberg@example-holding.de",
  last_synced_at: "2026-07-23T09:30:00Z",
  next_sync_due_at: "2026-07-23T09:35:00Z",
  watch_expires_at: "2026-08-01T00:00:00Z",
  mail_posture: "classified",
  backfill: { state: "done" },
};

const gcalNoLabel: CaptureConnection = {
  id: "018f3a1b-0000-7000-8000-0000000000c2",
  provider: "gcal",
  status: "connected",
  scopes: ["https://www.googleapis.com/auth/calendar.events"],
  last_synced_at: "2026-07-23T09:10:00Z",
};

const graphReauth: CaptureConnection = {
  id: "018f3a1b-0000-7000-8000-0000000000c3",
  provider: "graph",
  status: "reauth_required",
  scopes: ["Mail.Read"],
  account_label: "lars@example.de",
  last_synced_at: "2026-07-20T08:00:00Z",
  last_sync_error_class: "auth",
};

const imapError: CaptureConnection = {
  id: "018f3a1b-0000-7000-8000-0000000000c4",
  provider: "imap",
  status: "error",
  scopes: [],
  account_label: "sales@example.org",
  last_synced_at: "2026-07-18T12:00:00Z",
  last_sync_error_class: "unreachable",
};

const ROSTER = [gmail, gcalNoLabel, graphReauth, imapError];

function roster(rows: readonly CaptureConnection[]) {
  stubWithSession(
    {
      "GET /capture/settings": () =>
        jsonResponse({ shared_posture_allowed: false, signature_enrich: true }),
      "GET /tags": () => jsonResponse({ tags: [] }),
    },
    {},
  );
  return (
    <StoryProviders>
      <Panel title="Connected mailboxes and calendars">
        <SettingList bleed="records">
          {rows.map((conn) => (
            <ConnectorRow
              key={conn.id}
              conn={conn}
              connectPending={false}
              connectError={null}
              onReconnect={() => undefined}
              onDisconnect={() => undefined}
            />
          ))}
        </SettingList>
      </Panel>
    </StoryProviders>
  );
}

const meta: Meta<typeof ConnectorRow> = {
  title: "Settings/You/Connections/Connector row",
  component: ConnectorRow,
};
export default meta;
type Story = StoryObj<typeof ConnectorRow>;

export const EveryState: Story = { render: () => roster(ROSTER) };

export const EveryStateDark: Story = {
  globals: { theme: "dark" },
  render: () => roster(ROSTER),
};

// At 390 each row keeps its menu and chevron at the end of its first line.
export const EveryStatePhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => roster(ROSTER),
};

// The row's verbs: the import and the disconnect, in its one menu.
export const RowMenu: Story = {
  render: () => roster([gmail]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: /^Actions for Gmail/ }),
    );
    // The menu's items are portalled out of the canvas.
    await within(canvasElement.ownerDocument.body).findByRole("button", {
      name: "Import old emails",
    });
  },
};

// The chevron opens the mailbox's sync facts and its standing decisions.
export const MailboxSettings: Story = {
  render: () => roster([gmail]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const page = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      await canvas.findByRole("button", { name: /^Open Gmail/ }),
    );
    await page.findByRole("dialog", { name: /Gmail/ });
  },
};

// A calendar's drawer has the sync facts and the tag, and no mail settings.
export const CalendarSettings: Story = {
  render: () => {
    stubWithSession({ "GET /tags": () => jsonResponse({ tags: [] }) }, {});
    return (
      <StoryProviders>
        <ConnectionDrawer conn={gcalNoLabel} open onClose={() => undefined} />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    await within(canvasElement.ownerDocument.body).findByRole("dialog", {
      name: "Google Calendar",
    });
  },
};
