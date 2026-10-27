// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import { Panel, PanelBody } from "../../design-system/panel";
import { ProviderList } from "../../design-system/providerrow";
import { PageSaveBar, SavePageProvider } from "../../design-system/savepage";
import { accountsOf } from "../../format/connectorprovider";
import type { ImportWindow } from "../../lib/connect-account";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "../../testing/story-utils";
import { ConnectionAccountRow } from "./connection-account";

// One account on Connections in each state its services can be in. Syncing with
// the import reading, up to date, and a service needing a reconnect. Then a
// grant too narrow to send, a held import, and a mailbox without its calendar.

type CaptureConnection = components["schemas"]["CaptureConnection"];
type BackfillStatus = components["schemas"]["BackfillStatus"];

const gmail: CaptureConnection = {
  id: "018f3a1b-0000-7000-8000-0000000000c1",
  provider: "gmail",
  can_backfill: true,
  status: "connected",
  account_label: "admin@demo.test",
  scopes: [
    "https://www.googleapis.com/auth/gmail.readonly",
    "https://www.googleapis.com/auth/gmail.send",
  ],
  last_synced_at: "2026-07-23T09:52:00Z",
  last_success_at: "2026-07-23T09:52:00Z",
  created_at: "2026-07-23T08:38:00Z",
  backfill: {
    state: "done",
    window: "6m",
    counts: { captured: 842, contacts_created: 96, companies_created: 21 },
  },
};

const gcal: CaptureConnection = {
  ...gmail,
  id: "018f3a1b-0000-7000-8000-0000000000ca",
  provider: "gcal",
  can_backfill: false,
  scopes: ["https://www.googleapis.com/auth/calendar.events.owned"],
  backfill: undefined,
};

const reading: BackfillStatus = {
  state: "running",
  window: "6m",
  estimated_messages: 3560,
  counts: {
    messages_scanned: 1240,
    captured: 412,
    contacts_created: 38,
    companies_created: 9,
  },
  // Kept fresh, or a story opened minutes later draws the stalled state.
  updated_at: new Date().toISOString(),
};

const imap: CaptureConnection = {
  id: "018f3a1b-0000-7000-8000-0000000000c3",
  provider: "imap",
  can_backfill: false,
  status: "connected",
  account_label: "sales@example.org",
  scopes: [],
  last_synced_at: "2026-07-23T09:00:00Z",
  last_success_at: "2026-07-23T09:00:00Z",
  backfill: { state: "none" },
};

function accountStory(
  connections: CaptureConnection[],
  heldWindow: ImportWindow | null = null,
  routes: RouteMap = {},
) {
  return () => {
    installFetchStub(routes);
    return (
      <StoryProviders>
        {/* A changed setting waits on the page's one save bar, as on Settings. */}
        <SavePageProvider>
          <Panel title="Mail and calendar">
            <PanelBody>
              <ProviderList>
                {accountsOf(connections).map((account) => (
                  <ConnectionAccountRow
                    key={account.key}
                    account={account}
                    heldWindow={heldWindow}
                    onImapReconnect={() => undefined}
                  />
                ))}
              </ProviderList>
            </PanelBody>
          </Panel>
          <PageSaveBar />
        </SavePageProvider>
      </StoryProviders>
    );
  };
}

const syncingRoutes: RouteMap = {
  "GET /connectors/gmail/backfill": () => jsonResponse(reading),
};

const heldRoutes: RouteMap = {
  "POST /connectors/gmail/backfill/preview": () =>
    jsonResponse({
      window: "12m",
      estimated_messages: 8200,
      estimated_cost_minor: 320,
      currency: "USD",
      computed_at: "2026-07-23T10:00:00Z",
    }),
};

const meta: Meta<typeof ConnectionAccountRow> = {
  title: "Settings/You/Connections/Account",
  component: ConnectionAccountRow,
};
export default meta;
type Story = StoryObj<typeof ConnectionAccountRow>;

export const Syncing: Story = {
  render: accountStory(
    [{ ...gmail, backfill: reading }, gcal],
    null,
    syncingRoutes,
  ),
};

export const SyncingDark: Story = {
  globals: { theme: "dark" },
  render: accountStory(
    [{ ...gmail, backfill: reading }, gcal],
    null,
    syncingRoutes,
  ),
};

export const UpToDate: Story = {
  render: accountStory([gmail, gcal]),
};

export const UpToDateDark: Story = {
  globals: { theme: "dark" },
  render: accountStory([gmail, gcal]),
};

export const NeedsReconnect: Story = {
  render: accountStory([
    { ...gmail, status: "reauth_required", last_sync_error_class: "auth" },
    gcal,
  ]),
};

export const NeedsReconnectDark: Story = {
  globals: { theme: "dark" },
  render: accountStory([
    { ...gmail, status: "reauth_required", last_sync_error_class: "auth" },
    gcal,
  ]),
};

// A mailbox granted before sending shipped: the service names what it cannot
// do, and the account carries the reason and the one Reconnect.
const gmailNoSend: CaptureConnection = {
  ...gmail,
  scopes: ["https://www.googleapis.com/auth/gmail.readonly"],
};

export const CannotSend: Story = {
  render: accountStory([gmailNoSend, gcal]),
};

export const CannotSendDark: Story = {
  globals: { theme: "dark" },
  render: accountStory([gmailNoSend, gcal]),
};

// A failure streak the sweep has not given up on: amber with its reason, and
// no Reconnect, because a new consent would not mend it.
export const SyncTrouble: Story = {
  render: accountStory([
    { ...gmail, last_sync_error_class: "rate_limited" },
    gcal,
  ]),
};

// The connect asked for a year and the server held it above its cap: the
// row counts the year and waits for one press.
export const HeldImport: Story = {
  render: accountStory(
    [{ ...gmail, backfill: { state: "none" } }, gcal],
    "12m",
    heldRoutes,
  ),
};

export const HeldImportDark: Story = {
  globals: { theme: "dark" },
  render: accountStory(
    [{ ...gmail, backfill: { state: "none" } }, gcal],
    "12m",
    heldRoutes,
  ),
};

// A calendar granted for reading only: the row asks for a reconnect, and the
// account badge agrees with it rather than reading Connected.
const readOnlyGcal: CaptureConnection = {
  ...gcal,
  scopes: ["https://www.googleapis.com/auth/calendar.readonly"],
};

export const ReadOnlyCalendar: Story = {
  render: accountStory([gmail, readOnlyGcal]),
};

export const ReadOnlyCalendarDark: Story = {
  globals: { theme: "dark" },
  render: accountStory([gmail, readOnlyGcal]),
};

export const CalendarMissing: Story = {
  render: accountStory([{ ...gmail, backfill: { state: "none" } }]),
};

export const CalendarMissingDark: Story = {
  globals: { theme: "dark" },
  render: accountStory([{ ...gmail, backfill: { state: "none" } }]),
};

export const ImapMailbox: Story = {
  render: accountStory([imap]),
};

export const MenuOpen: Story = {
  render: accountStory([gmail, gcal]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "More actions for Google" }),
    );
  },
};

export const MenuOpenDark: Story = {
  globals: { theme: "dark" },
  render: accountStory([gmail, gcal]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "More actions for Google" }),
    );
  },
};

async function openBringsIn(canvasElement: HTMLElement) {
  const canvas = within(canvasElement);
  await userEvent.click(
    await canvas.findByRole("button", { name: "More actions for Google" }),
  );
  await userEvent.click(
    await within(canvasElement.ownerDocument.body).findByRole("button", {
      name: "Change what it brings in",
    }),
  );
}

export const BringsInDialog: Story = {
  render: accountStory([gmail]),
  play: async ({ canvasElement }) => openBringsIn(canvasElement),
};

export const BringsInDialogDark: Story = {
  globals: { theme: "dark" },
  render: accountStory([gmail]),
  play: async ({ canvasElement }) => openBringsIn(canvasElement),
};

// A narrowing waits on the bar, and asks beside the choice whether mail already
// captured narrows with it.
export const NarrowingPosture: Story = {
  render: accountStory([gmail]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("combobox", { name: "Mail visibility" }),
    );
    await userEvent.click(
      await within(canvasElement.ownerDocument.body).findByRole("option", {
        name: "Always held",
      }),
    );
  },
};

export const Phone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: accountStory(
    [{ ...gmail, backfill: reading }, gcal],
    null,
    syncingRoutes,
  ),
};
