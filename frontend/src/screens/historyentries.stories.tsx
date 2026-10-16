// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { type Locale, translate } from "../i18n";
import {
  emptyPage,
  expectNameAndValueApart,
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { RecordHistory } from "./historyentries";

// The plain-language change list (B-EP09.x), on its own rather than behind
// RecordHistoryTab's SegmentedControl: history.stories.tsx already covers
// the tab's toggle and the field-diff half; this is the smaller surface
// historyentries.tsx exports directly, so its own story reads GET
// /records/{entity_type}/{id}/history the same way every screen story stubs
// an api route rather than mounting the field-history tab this panel never
// reaches.

function seedWorkspace() {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
}

const created = {
  id: "h1",
  actor_type: "human",
  actor_id: "u1",
  actor_name: "Demo Admin",
  action: "create",
  occurred_at: "2026-07-13T10:00:00Z",
  summary: "Demo Admin created the record",
};
const updated = {
  id: "h2",
  actor_type: "agent",
  actor_id: "sdr",
  on_behalf_of_name: "Anna Weber",
  action: "update",
  occurred_at: "2026-07-14T10:00:00Z",
  summary: "Overnight agent updated the record",
  before: { amount_minor: 2500000 },
  after: { amount_minor: 4150000 },
};
const viaClient = {
  id: "h3",
  actor_type: "agent",
  actor_id: "agent:p1",
  passport_id: "0199a000-0000-7000-8000-000000000001",
  on_behalf_of_name: "Ada Authority",
  agent_client: "Claude",
  action: "archive",
  occurred_at: "2026-07-15T10:00:00Z",
  summary: "Ada Authority, via Claude, archived the record",
};
const connector = {
  id: "h4",
  actor_type: "connector",
  actor_id: "connector:gmail",
  action: "update",
  occurred_at: "2026-07-15T11:00:00Z",
  summary: "Connector updated the record",
};
const system = {
  id: "h5",
  actor_type: "system",
  actor_id: "system",
  action: "restrict",
  occurred_at: "2026-07-15T12:00:00Z",
  summary: "System withheld the record",
};
const unrecorded = {
  id: "h6",
  actor_type: "agent",
  actor_id: "agent:p2",
  passport_id: "0199a000-0000-7000-8000-000000000002",
  action: "send_email",
  occurred_at: "2026-07-15T13:00:00Z",
  summary:
    "A machine with no recorded human authority sent an email for the record",
};
const everyActor = [unrecorded, system, connector, viaClient, updated, created];

function Panel({ locale = "en" }: Readonly<{ locale?: Locale }>) {
  return (
    <StoryProviders locale={locale}>
      <div style={{ maxWidth: 720 }}>
        <RecordHistory kind="deal" id="d1" currency="EUR" />
      </div>
    </StoryProviders>
  );
}

const meta: Meta<typeof Panel> = {
  title: "Records/Record 360/Record history/Changes list",
  component: Panel,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof Panel>;

// The timeline-plain layout this panel is named for: full-width sentences,
// ruled rows, each carrying its own date inline rather than in a gutter.
export const Entries: Story = {
  render: () => {
    seedWorkspace();
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /records/deal/d1/history": () =>
        jsonResponse({
          data: everyActor,
          page: { next_cursor: null, has_more: false },
        }),
    });
    return <Panel />;
  },
  play: async ({ canvasElement }) => {
    await expectViaClientLine(canvasElement, "en");
    await expectNameAndValueApart(canvasElement);
  },
};

// The same change list in dark: the agent-actor line reads "on behalf of"
// through the same indigo provenance token as light, so this is the story
// that shows whether that tint still passes against the dark panel.
export const EntriesDark: Story = {
  ...Entries,
  globals: { theme: "dark" },
};

// The sentence and the fields it puts back read as one block.
export const UndoConfirm: Story = {
  render: () => {
    seedWorkspace();
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /records/deal/d1/history": () =>
        jsonResponse({
          data: [
            {
              ...updated,
              before: { amount_minor: 2500000, name: "Globex" },
              after: { amount_minor: 4150000, name: "Globex Renewal" },
              undoable: { undoable: true },
            },
            created,
          ],
          page: { next_cursor: null, has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <RecordHistory
          kind="deal"
          id="d1"
          currency="EUR"
          restore={{ version: 7, onRestored: () => {} }}
        />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", { name: "Undo" }),
    );
    await within(canvasElement.ownerDocument.body).findByRole("dialog");
  },
};

function stubEveryActor() {
  seedWorkspace();
  installFetchStub({
    "GET /me": meRoute({}),
    "GET /records/deal/d1/history": () =>
      jsonResponse({
        data: everyActor,
        page: { next_cursor: null, has_more: false },
      }),
  });
}

function viaClientLine(locale: Locale): string {
  const subject = translate(locale, "history.subject.viaClient", {
    name: viaClient.on_behalf_of_name,
    client: viaClient.agent_client,
  });
  return translate(locale, "history.verb.archive", { subject });
}

async function expectViaClientLine(canvasElement: HTMLElement, locale: Locale) {
  const line = await within(canvasElement).findByText(viaClientLine(locale));
  await expect(line).toBeVisible();
  await expect(within(canvasElement).queryByText(viaClient.summary)).toBeNull();
}

export const GermanReader: Story = {
  render: () => {
    stubEveryActor();
    return <Panel locale="de" />;
  },
  play: ({ canvasElement }) => expectViaClientLine(canvasElement, "de"),
};

export const VietnameseReader: Story = {
  render: () => {
    stubEveryActor();
    return <Panel locale="vi" />;
  },
  play: ({ canvasElement }) => expectViaClientLine(canvasElement, "vi"),
};

// German runs longest, so the phone is where a sentence would overflow.
export const GermanReaderPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => {
    stubEveryActor();
    return <Panel locale="de" />;
  },
  play: ({ canvasElement }) => expectViaClientLine(canvasElement, "de"),
};

// Nothing recorded for this record yet: the honest empty state, not a list
// with no rows in it.
export const Empty: Story = {
  render: () => {
    seedWorkspace();
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /records/deal/d1/history": () => jsonResponse(emptyPage),
    });
    return <Panel />;
  },
};
