// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { standardViews } from "../../components/recordlist";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { LeadBoard, StatusBadge } from "./leadpresentation";
import { LeadScreen } from "./leads";
import { LeadsScreen } from "./leads.list";
import { LeadManualSignals } from "./leadsignals";
import { SIGNAL_CATALOG } from "./leadsignals.fixture";

// LeadsScreen (list, accent-tinted "segregated" surface) and LeadScreen (its
// own 360 — never contact.html, per the §3.5 segregation gap) both read
// through the api client on mount; LeadScreen's lifecycle panel also reads
// GET /me (the session-principal probe every role-aware surface shares).
const meta: Meta = {
  title: "Records/Leads/Page",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const lead = {
  id: "l-1",
  full_name: "Jonas Petersen",
  email: "jonas@nordwind.example",
  company_name: "Nordwind Logistik",
  status: "contacted" as const,
  score: 72,
  source: "manual",
  captured_by: "human:u1",
  version: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

// The queue reads its dials off the address, so each story that wants a
// particular view names it as an address. Set before render, because the screen
// derives the view during its first one — and set on EVERY such story rather
// than only the ones that want a dial, so a story does not inherit whichever
// address the story before it left behind.
export const LeadsList: Story = {
  render: () => {
    installFetchStub({
      "GET /lead-signal-catalog": () => jsonResponse(SIGNAL_CATALOG),
      "GET /me": meRoute({ lead: ["read", "update"] }),
      "GET /leads": () =>
        jsonResponse({
          data: [lead],
          page: { next_cursor: null, has_more: false },
        }),
    });
    globalThis.location.hash = "#/leads";
    return (
      <StoryProviders>
        <LeadsScreen />
      </StoryProviders>
    );
  },
};

export const LeadOverview: Story = {
  render: () => {
    installFetchStub({
      "GET /lead-signal-catalog": () => jsonResponse(SIGNAL_CATALOG),
      "GET /leads/l-1": () => jsonResponse(lead),
      "GET /me": () =>
        jsonResponse({
          user: { id: "u-9", display_name: "Me" },
          roles: ["rep"],
          teams: [],
        }),
    });
    return (
      <StoryProviders>
        <LeadScreen id="l-1" />
      </StoryProviders>
    );
  },
};

// A lead that earns nothing, which is the common shape of a fresh one: the
// panel states the reasons rather than the score's storage history, so a 0
// stops reading as a bad prospect when it means an unassessed one
// (ADR-0108 §4).
export const LeadScoringZero: Story = {
  render: () => {
    installFetchStub({
      "GET /lead-signal-catalog": () => jsonResponse(SIGNAL_CATALOG),
      "GET /leads/l-1": () =>
        jsonResponse({ ...lead, score: 0, title: "Boss", source: "manual" }),
      "GET /leads/l-1/score": () =>
        jsonResponse({ score: 0, explained: false }),
      "GET /me": () =>
        jsonResponse({
          user: { id: "u-9", display_name: "Me" },
          roles: ["rep"],
          teams: [],
        }),
    });
    return (
      <StoryProviders>
        <LeadScreen id="l-1" />
      </StoryProviders>
    );
  },
};

// The score explained: factors with their points and the decay as arithmetic
// a reader can check, plus the line that reconciles them to the stored score.
export const LeadScoreExplained: Story = {
  render: () => {
    installFetchStub({
      "GET /lead-signal-catalog": () => jsonResponse(SIGNAL_CATALOG),
      "GET /leads/l-1": () => jsonResponse(lead),
      "GET /leads/l-1/score": () =>
        jsonResponse({
          score: 72,
          explained: true,
          current: {
            score: 72,
            score_computed: 72,
            raw_sum: 71.6,
            rounded_sum: 72,
            computed_at: "2026-06-04T00:00:00Z",
            factors: [
              { factor: "decision_maker_title", points: 15 },
              { factor: "high_intent_source", points: 8 },
              { factor: "reply", points: 22.6, base_points: 25 },
            ],
          },
        }),
      "GET /me": () =>
        jsonResponse({
          user: { id: "u-9", display_name: "Me" },
          roles: ["rep"],
          teams: [],
        }),
    });
    return (
      <StoryProviders>
        <LeadScreen id="l-1" />
      </StoryProviders>
    );
  },
};

// The activities behind a behavioral factor, with the card open.
//
// Three sources and two rows. The third is an activity this reader cannot
// discover, so the card says it is showing fewer than the count. One row is a
// conversation whose content belongs to its participants, and the row says so
// where a subject would go.
export const LeadScoreSourcesNamed: Story = {
  render: () => {
    installFetchStub({
      "GET /lead-signal-catalog": () => jsonResponse(SIGNAL_CATALOG),
      "GET /leads/l-1": () => jsonResponse(lead),
      "GET /leads/l-1/score": () =>
        jsonResponse({
          score: 72,
          explained: true,
          current: {
            score: 72,
            score_computed: 72,
            raw_sum: 71.6,
            rounded_sum: 72,
            computed_at: "2026-06-04T00:00:00Z",
            factors: [
              { factor: "decision_maker_title", points: 15 },
              {
                factor: "reply",
                points: 22.6,
                base_points: 25,
                source_activity_ids: ["a-1", "a-2", "a-3"],
                source_activities: [
                  {
                    activity_id: "a-1",
                    kind: "email",
                    subject: "Re: pricing for the pilot",
                    occurred_at: "2026-06-02T09:12:00Z",
                    occurred_precision: "instant",
                    content_state: "available",
                  },
                  {
                    activity_id: "a-2",
                    kind: "message",
                    subject: null,
                    occurred_at: "2026-05-28T00:00:00Z",
                    occurred_precision: "day",
                    content_state: "withheld",
                  },
                ],
              },
            ],
          },
        }),
      "GET /me": () =>
        jsonResponse({
          user: { id: "u-9", display_name: "Me" },
          roles: ["rep"],
          teams: [],
        }),
    });
    return (
      <StoryProviders>
        <LeadScreen id="l-1" />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByRole("button", { name: /3 / }));
  },
};

// An engaged lead as the design draws it. It shows How it stands with its
// dated ladder, the score's readings, the timeline to a booked meeting, and
// the cards under it. Dates are set from the moment the story opens, because
// the page reads "ahead" and "days since" against its own clock.
const day = 86_400_000;
const daysFromNow = (days: number) =>
  new Date(Date.now() + days * day).toISOString();

export const LeadEngagedMeetingBooked: Story = {
  render: () => {
    const engaged = {
      ...lead,
      full_name: "Jonas Weber",
      title: "Head of Operations",
      company_name: "Weber Logistik",
      email: "jonas@weber.example",
      status: "engaged" as const,
      status_set_by: "system" as const,
      source: "webform",
      created_at: daysFromNow(-11),
      first_response_at: daysFromNow(-10.9),
      last_activity_at: daysFromNow(-4),
      open_task_count: 1,
      qualification_evidence: {
        trigger: "inbound_reply" as const,
        occurred_at: daysFromNow(-4),
      },
    };
    const email = (
      id: string,
      at: string,
      direction: string,
      body: string,
    ) => ({
      id,
      kind: "email",
      direction,
      subject: "Fleet tracking",
      body,
      occurred_at: at,
      is_done: true,
      source: "capture",
      captured_by: "system",
      created_at: at,
      updated_at: at,
    });
    installFetchStub({
      "GET /lead-signal-catalog": () => jsonResponse(SIGNAL_CATALOG),
      "GET /leads/l-1": () => jsonResponse(engaged),
      "GET /activities": () =>
        jsonResponse({
          data: [
            {
              ...email("m-1", daysFromNow(2), "outbound", ""),
              kind: "meeting",
              subject: "Depot walkthrough",
              meeting_status: "booked",
              is_done: false,
            },
            email(
              "e-3",
              daysFromNow(-4),
              "inbound",
              "Thursday works. Can you show us how the depot view works?",
            ),
            email("e-2", daysFromNow(-10.9), "outbound", "Thanks for writing."),
            email(
              "e-1",
              daysFromNow(-11),
              "inbound",
              "We run 120 vans across three depots and want live tracking before winter.",
            ),
          ],
          page: { next_cursor: null, has_more: false },
        }),
      "GET /field-history": () =>
        jsonResponse({
          data: [
            {
              id: "f-2",
              entity_type: "lead",
              entity_id: "l-1",
              field: "status",
              new_value: "engaged",
              changed_at: daysFromNow(-4),
              actor_type: "system",
              actor_id: "system",
            },
            {
              id: "f-1",
              entity_type: "lead",
              entity_id: "l-1",
              field: "status",
              new_value: "contacted",
              changed_at: daysFromNow(-10.9),
              actor_type: "system",
              actor_id: "system",
            },
          ],
          page: { next_cursor: null, has_more: false },
        }),
      "GET /leads/l-1/promote-preview": () =>
        jsonResponse({ outcome: "create", contact_withheld: false }),
      "GET /leads/l-1/score": () =>
        jsonResponse({
          score: 72,
          explained: true,
          current: {
            score: 72,
            score_computed: 72,
            raw_sum: 72.4,
            rounded_sum: 72,
            computed_at: daysFromNow(-1),
            factors: [
              {
                factor: "reply",
                points: 39.4,
                base_points: 45,
                source_activity_ids: ["e-3"],
              },
              { factor: "decision_maker_title", points: 15 },
              { factor: "high_intent_source", points: 18 },
            ],
          },
        }),
      "GET /me": () =>
        jsonResponse({
          user: { id: "u-9", display_name: "Me" },
          roles: ["rep"],
          teams: [],
        }),
    });
    return (
      <StoryProviders>
        <LeadScreen id="l-1" />
      </StoryProviders>
    );
  },
};

// A disqualified lead keeps its controls, DISABLED with the reason — hiding
// them hid the fact the reader needed (STATE-4a).
export const LeadDisqualified: Story = {
  render: () => {
    installFetchStub({
      "GET /lead-signal-catalog": () => jsonResponse(SIGNAL_CATALOG),
      "GET /leads/l-1": () =>
        jsonResponse({
          ...lead,
          status: "disqualified",
          archived_at: "2026-07-13T00:00:00Z",
        }),
      "GET /leads/l-1/score": () =>
        jsonResponse({ score: 72, explained: false }),
      "GET /me": () =>
        jsonResponse({
          user: { id: "u-9", display_name: "Me" },
          roles: ["rep"],
          teams: [],
        }),
    });
    return (
      <StoryProviders>
        <LeadScreen id="l-1" />
      </StoryProviders>
    );
  },
};

// A PROMOTED lead keeps its page too (ADR-0119/A170), and leads with what the
// promotion did: which contact it became, and whether it merged into one we
// already knew or created a new one. The outcome is read from the promote
// audit row, so the story stubs that read as well as the lead itself.
export const LeadPromotedAfterMerge: Story = {
  render: () => {
    installFetchStub({
      "GET /lead-signal-catalog": () => jsonResponse(SIGNAL_CATALOG),
      "GET /leads/l-1": () =>
        jsonResponse({
          ...lead,
          status: "promoted",
          promoted_contact_id: "p-42",
          promoted_at: "2026-06-20T08:00:00Z",
          archived_at: "2026-06-20T08:00:00Z",
        }),
      "GET /records/lead/l-1/history": () =>
        jsonResponse({
          data: [
            {
              id: "a-1",
              actor_type: "human",
              actor_id: "human:u-9",
              action: "promote",
              occurred_at: "2026-06-20T08:00:00Z",
              after: {
                dedupe_outcome: "merged",
                trigger: "inbound_reply",
                evidence_note: "Replied asking for a quote.",
              },
            },
          ],
          page: { next_cursor: null, has_more: false },
        }),
      "GET /leads/l-1/score": () =>
        jsonResponse({ score: 72, explained: false }),
      "GET /me": () =>
        jsonResponse({
          user: { id: "u-9", display_name: "Me" },
          roles: ["rep"],
          teams: [],
        }),
    });
    return (
      <StoryProviders>
        <LeadScreen id="l-1" />
      </StoryProviders>
    );
  },
};

export const LeadPromotedDemoting: Story = {
  render: LeadPromotedAfterMerge.render,
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      await body.findByRole("button", { name: "More actions" }),
    );
    await userEvent.click(
      await body.findByRole("button", { name: "Reverse qualification" }),
    );
    await body.findByRole("dialog");
  },
};

// The board: the live leads in the two columns they can actually move between.
// Opened by address rather than by pressing the toggle, because the address is
// what carries the choice now — this is the surface a shared
// "#/leads?view=board" link lands on, and a reload stays on it.
// Terminal statuses get no column — a lead is promoted or disqualified through
// its own audited verb, never by dragging a card.
export const LeadsBoard: Story = {
  render: () => {
    installFetchStub({
      "GET /lead-signal-catalog": () => jsonResponse(SIGNAL_CATALOG),
      "GET /leads": () =>
        jsonResponse({
          data: [
            { ...lead, id: "l-1", status: "new", score: 82 },
            {
              ...lead,
              id: "l-2",
              full_name: "Petra Vogel",
              company_name: "Südwind AG",
              status: "contacted",
              score: 54,
            },
          ],
          page: { next_cursor: null, has_more: false },
        }),
      "GET /me": () =>
        jsonResponse({
          user: { id: "u-9", display_name: "Me" },
          roles: ["rep"],
          teams: [],
        }),
    });
    globalThis.location.hash = "#/leads?view=board";
    return (
      <StoryProviders>
        <LeadsScreen />
      </StoryProviders>
    );
  },
};

export const LeadPresentationComponents: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ display: "grid", gap: "var(--space-3)" }}>
        <div style={{ display: "flex", gap: "var(--space-2)" }}>
          <StatusBadge status="new" />
          <StatusBadge status="contacted" />
        </div>
        <LeadBoard
          rows={[
            { ...lead, status: "new", score: 82 },
            {
              ...lead,
              id: "l-2",
              full_name: "Petra Vogel",
              status: "contacted",
              score: 54,
            },
          ]}
          onMoved={() => undefined}
          hasMore={false}
          loadMore={() => undefined}
        />
      </div>
    </StoryProviders>
  ),
};

export const ManualQualificationEvidence: Story = {
  render: () => {
    installFetchStub({
      "GET /lead-signal-catalog": () => jsonResponse(SIGNAL_CATALOG),
      "GET /leads/l-1/manual-signals": () =>
        jsonResponse({
          data: [
            {
              factor: "budget_hint",
              band: "confirmed",
              points: 18,
              signal_kind: "fact",
              confidence: 0.9,
              reason: "Budget confirmed during discovery call.",
              set_by: "u-9",
              set_at: "2026-06-20T08:00:00Z",
            },
          ],
        }),
      "GET /users": () =>
        jsonResponse({
          data: [{ id: "u-9", display_name: "Mina Rep" }],
          page: { next_cursor: null, has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <LeadManualSignals id="l-1" />
      </StoryProviders>
    );
  },
};

export const SharedRecordViews: Story = {
  render: () => (
    <ul>
      {standardViews("u-9", { sort: "", mineFirst: true }).map((view) => (
        <li key={view.label}>
          {view.label}: {view.filters?.owner_id ?? "workspace"}
        </li>
      ))}
    </ul>
  ),
};

// A rep sent here to log a call attempt. The address names the verb, so the
// header's Log activity drawer opens already showing Call rather than a note
// they would have to change it to.
export const LeadArrivedToLogACall: Story = {
  render: () => {
    installFetchStub({
      "GET /lead-signal-catalog": () => jsonResponse(SIGNAL_CATALOG),
      "GET /leads/l-1": () => jsonResponse(lead),
      "GET /me": () =>
        jsonResponse({
          user: { id: "u-9", display_name: "Me" },
          roles: ["rep"],
          teams: [],
        }),
    });
    globalThis.location.hash = "#/leads/l-1?action=call";
    return (
      <StoryProviders>
        <LeadScreen id="l-1" />
      </StoryProviders>
    );
  },
};
