// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { RecordHistoryTab } from "./history";
import {
  emptyPage,
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// RecordHistoryTab (B-EP09.x) reads through two endpoints depending on the
// SegmentedControl toggle — GET /records/{entity_type}/{id}/history (Changes,
// the default tab on mount) and GET /field-history (Field history) — both
// resolving to a static pathname for the kind="deal" id="d1" every story
// below hardcodes, so each route is stubbed explicitly by its own key. A
// blanket fallback would answer the Changes-tab mount with field-history-
// shaped fixtures (`changed_at`, no `occurred_at`), which crashes
// formatDateTime on an Invalid time value — this keeps each endpoint's
// response shaped for the schema it actually is.
const meta: Meta = {
  title: "Records/Record history",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function seedWorkspace() {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
}

const created = {
  id: "h1",
  actor_type: "human",
  actor_id: "u1",
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
};

export const Changes: Story = {
  render: () => {
    seedWorkspace();
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /records/deal/d1/history": () =>
        jsonResponse({
          data: [created, updated],
          page: { next_cursor: null, has_more: false },
        }),
      "GET /field-history": () => jsonResponse(emptyPage),
    });
    return (
      <StoryProviders>
        <RecordHistoryTab kind="deal" id="d1" />
      </StoryProviders>
    );
  },
};

const fhCreated = {
  id: "f0",
  entity_type: "deal",
  entity_id: "d1",
  field: "name",
  old_value: null,
  new_value: "Globex Renewal",
  changed_at: "2026-07-13T10:00:00Z",
  actor_type: "human",
  actor_id: "u1",
};
const fhUpdated = {
  id: "f1",
  entity_type: "deal",
  entity_id: "d1",
  field: "name",
  old_value: "Globex Renewal",
  new_value: "Globex Renewal (updated)",
  changed_at: "2026-07-14T10:00:00Z",
  actor_type: "agent",
  actor_id: "sdr",
  passport_id: "psp_7Q3fa91",
  evidence: { snippet: "renewal signed", source: "email#42" },
};

// The record-history fixture is a single unrelated "name field touched"
// entry — the story mounts on the Changes tab first (no initialTab prop on
// the component), then the reviewer clicks "Field history" to see the
// old→new diff grouping this story is actually named for.
export const FieldDiffs: Story = {
  render: () => {
    seedWorkspace();
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /records/deal/d1/history": () =>
        jsonResponse({
          data: [updated],
          page: { next_cursor: null, has_more: false },
        }),
      "GET /field-history": () =>
        jsonResponse({
          data: [fhUpdated, fhCreated],
          page: { next_cursor: null, has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <RecordHistoryTab kind="deal" id="d1" />
      </StoryProviders>
    );
  },
};

export const Empty: Story = {
  render: () => {
    seedWorkspace();
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /records/deal/d1/history": () => jsonResponse(emptyPage),
      "GET /field-history": () => jsonResponse(emptyPage),
    });
    return (
      <StoryProviders>
        <RecordHistoryTab kind="deal" id="d1" />
      </StoryProviders>
    );
  },
};

export const ErrorState: Story = {
  render: () => {
    seedWorkspace();
    // The Changes tab is what's shown on mount, so that's the route the
    // error has to come back on for the story to demonstrate the error
    // state; field-history stays healthy in case the reviewer switches tabs.
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /records/deal/d1/history": () =>
        jsonResponse({ title: "boom" }, 500),
      "GET /field-history": () => jsonResponse(emptyPage),
    });
    return (
      <StoryProviders>
        <RecordHistoryTab kind="deal" id="d1" />
      </StoryProviders>
    );
  },
};

// A field-history entry carrying passport_id + evidence — the agent-
// attribution surface (PassportChip + EvidenceChip) on the field-diff view.
export const AgentAttribution: Story = {
  render: () => {
    seedWorkspace();
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /records/deal/d1/history": () =>
        jsonResponse({
          data: [updated],
          page: { next_cursor: null, has_more: false },
        }),
      "GET /field-history": () =>
        jsonResponse({
          data: [fhUpdated],
          page: { next_cursor: null, has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <RecordHistoryTab kind="deal" id="d1" />
      </StoryProviders>
    );
  },
};

// The verb and its two refusals, on the tab a reader lands on. `restore`
// carries the version the write pins against and the record re-read that
// follows, so the button is offered; without it the panel is read-only.
const RESTORE = { version: 7, onRestored: () => {} };

const repriced = {
  id: "h3",
  actor_type: "human",
  actor_id: "u1",
  actor_name: "Demo Admin",
  action: "update",
  occurred_at: "2026-07-15T10:00:00Z",
  summary: "Demo Admin repriced the deal",
  before: { amount_minor: 2500000, name: "Globex" },
  after: { amount_minor: 4150000, name: "Globex Renewal" },
};

export const CanBePutBack: Story = {
  render: () => {
    seedWorkspace();
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /records/deal/d1/history": () =>
        jsonResponse({
          data: [
            { ...repriced, undoable: { undoable: true } },
            { ...created, undoable: { undoable: true } },
          ],
          page: { next_cursor: null, has_more: false },
        }),
      "GET /field-history": () => jsonResponse(emptyPage),
    });
    return (
      <StoryProviders>
        <RecordHistoryTab
          kind="deal"
          id="d1"
          currency="EUR"
          restore={RESTORE}
        />
      </StoryProviders>
    );
  },
};

// The state this feature exists for: a refused verb that SAYS why, in the same
// words the server answers a refused press with.
export const RefusedWithItsReason: Story = {
  render: () => {
    seedWorkspace();
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /records/deal/d1/history": () =>
        jsonResponse({
          data: [
            {
              ...repriced,
              undoable: {
                undoable: false,
                reason: "superseded",
                detail: "amount_minor",
              },
            },
            {
              ...created,
              undoable: { undoable: false, reason: "not_a_replayable_verb" },
            },
          ],
          page: { next_cursor: null, has_more: false },
        }),
      "GET /field-history": () => jsonResponse(emptyPage),
    });
    return (
      <StoryProviders>
        <RecordHistoryTab
          kind="deal"
          id="d1"
          currency="EUR"
          restore={RESTORE}
        />
      </StoryProviders>
    );
  },
};

// A reversal and the change it reversed, as ONE line the reader opens.
//
// The complaint this answers was the COUNT: an undo writes an ordinary update,
// so three changes rendered as four, the fourth saying "restored the record"
// without naming what it restored. Collapsed by default, and a disclosure
// rather than a deletion — both audit rows are one press away.
const titleMoved = {
  id: "h9",
  actor_type: "human",
  actor_id: "human:u-sam",
  actor_name: "Sam Okafor",
  action: "update",
  occurred_at: "2026-07-14T12:00:00Z",
  summary: "Sam Okafor updated the record",
  before: { name: "Globex" },
  after: { name: "Globex Renewal" },
  undoable: { undoable: false, reason: "already_undone" },
};
const putBackAgain = {
  id: "h10",
  actor_type: "human",
  actor_id: "human:u-tin",
  actor_name: "Tin Nguyen",
  action: "restore",
  occurred_at: "2026-07-14T12:04:00Z",
  summary: "Tin Nguyen restored the record",
  undid_audit_log_id: "h9",
  before: { name: "Globex Renewal" },
  after: { name: "Globex" },
  undoable: { undoable: true },
};

export const AReversalCollapsedWithWhatItUndid: Story = {
  render: () => {
    seedWorkspace();
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /records/deal/d1/history": () =>
        jsonResponse({
          data: [putBackAgain, titleMoved, created],
          page: { next_cursor: null, has_more: false },
        }),
      "GET /field-history": () => jsonResponse(emptyPage),
    });
    return (
      <StoryProviders>
        <RecordHistoryTab
          kind="deal"
          id="d1"
          currency="EUR"
          restore={RESTORE}
        />
      </StoryProviders>
    );
  },
};

// The state one word separates from a lie: a restore that put back only SOME
// of what moved. The headline says "partly", and the residual is on the face —
// a row claiming nothing changed while a field still holds a new value is the
// worst outcome this shape can produce.
export const AReversalThatOnlyPartlyWentBack: Story = {
  render: () => {
    seedWorkspace();
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /records/deal/d1/history": () =>
        jsonResponse({
          data: [
            putBackAgain,
            {
              ...titleMoved,
              before: { name: "Globex", amount_minor: 2500000 },
              after: { name: "Globex Renewal", amount_minor: 4150000 },
            },
            created,
          ],
          page: { next_cursor: null, has_more: false },
        }),
      "GET /field-history": () => jsonResponse(emptyPage),
    });
    return (
      <StoryProviders>
        <RecordHistoryTab
          kind="deal"
          id="d1"
          currency="EUR"
          restore={RESTORE}
        />
      </StoryProviders>
    );
  },
};
