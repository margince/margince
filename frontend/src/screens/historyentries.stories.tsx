// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { RecordHistory } from "./historyentries";
import {
  emptyPage,
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

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

function Panel() {
  return (
    <StoryProviders>
      <div style={{ maxWidth: 720 }}>
        <RecordHistory kind="deal" id="d1" currency="EUR" />
      </div>
    </StoryProviders>
  );
}

const meta: Meta<typeof Panel> = {
  title: "Records/Record history/Changes list",
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
          data: [updated, created],
          page: { next_cursor: null, has_more: false },
        }),
    });
    return <Panel />;
  },
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
