// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { PersonNetworkTab } from "./personnetwork";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The contact's network tab: who reaches them, how warmly, and what moved.
//
// The states worth seeing are the ones that render identically if their
// distinction is dropped: a WITHHELD arm against an empty one, and a graph
// that dropped nodes against one that showed everybody. Both are the "never
// imply completeness" rule, and both are invisible on a happy path.

type Person360 = components["schemas"]["Person360"];
type PersonGraph = components["schemas"]["PersonGraph"];

const PERSON_ID = "01a03000-0000-7000-8000-000000000001";
const ANCHOR = `person:${PERSON_ID}`;
const COLLEAGUE = "user:01a03000-0000-7000-8000-0000000000c1";
const COWORKER = "person:01a03000-0000-7000-8000-0000000000b2";

const graph = (over: Partial<PersonGraph> = {}): PersonGraph => ({
  person_id: PERSON_ID,
  nodes: [
    { id: ANCHOR, type: "contact", group: "anchor", label: "Dana Weiss" },
    {
      id: COLLEAGUE,
      type: "colleague",
      group: "direct",
      label: "Lena Fischer",
      sublabel: "Sales",
    },
    {
      id: COWORKER,
      type: "contact",
      group: "account",
      label: "Tomas Berg",
      sublabel: "Head of Ops",
    },
  ],
  edges: [
    {
      from: COLLEAGUE,
      to: ANCHOR,
      strength_bucket: "strong",
      interactions_90d: 24,
      inbound_90d: 11,
      outbound_90d: 13,
      last_at: "2026-08-20T09:00:00Z",
      receipts: [
        {
          activity_id: "01a03000-0000-7000-8000-0000000000e1",
          subject: "Re: rollout plan",
          occurred_at: "2026-08-20T09:00:00Z",
        },
      ],
    },
    {
      from: COLLEAGUE,
      to: COWORKER,
      strength_bucket: "weak",
      interactions_90d: 2,
      inbound_90d: 1,
      outbound_90d: 1,
      last_at: "2026-07-02T09:00:00Z",
    },
  ],
  route: {
    via_user_id: "01a03000-0000-7000-8000-0000000000c1",
    via_display_name: "Lena Fischer",
    why: "24 exchanges in the last 90 days, most recently on 20 August.",
  },
  groups_omitted: [],
  dropped_count: { direct: 0, account: 0 },
  ...over,
});

const view = (over: Partial<Person360> = {}): Person360 =>
  ({
    person: { id: PERSON_ID, display_name: "Dana Weiss" },
    relationship_changes: [
      {
        kind: "replied_after_gap",
        at: "2026-08-20T09:00:00Z",
        days: 41,
      },
      { kind: "warmed", at: "2026-08-25T09:00:00Z" },
    ],
    sections_omitted: [],
    ...over,
  }) as Person360;

const meta: Meta<typeof PersonNetworkTab> = {
  title: "Records/Person network",
  component: PersonNetworkTab,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof PersonNetworkTab>;

/** A contact one colleague knows well, with a coworker beside them. */
export const Warm: Story = {
  render: () => {
    installFetchStub({
      [`GET /api/people/${PERSON_ID}/graph`]: () => jsonResponse(graph()),
    });
    return (
      <StoryProviders>
        <PersonNetworkTab personId={PERSON_ID} view={view()} />
      </StoryProviders>
    );
  },
};

/**
 * An arm the reader may not see. Withheld is not empty: the account group says
 * so rather than rendering the sentence an absence would have produced.
 */
export const AccountWithheld: Story = {
  render: () => {
    installFetchStub({
      [`GET /api/people/${PERSON_ID}/graph`]: () =>
        jsonResponse(
          graph({
            nodes: graph().nodes?.filter((n) => n.group !== "account"),
            edges: graph().edges?.filter((e) => e.to !== COWORKER),
            groups_omitted: ["account"],
          }),
        ),
    });
    return (
      <StoryProviders>
        <PersonNetworkTab personId={PERSON_ID} view={view()} />
      </StoryProviders>
    );
  },
};

/**
 * Nodes were dropped. Silent truncation reads as "this is everyone", which is
 * the one thing a graph must never imply falsely.
 */
export const Truncated: Story = {
  render: () => {
    installFetchStub({
      [`GET /api/people/${PERSON_ID}/graph`]: () =>
        jsonResponse(graph({ dropped_count: { direct: 3, account: 12 } })),
    });
    return (
      <StoryProviders>
        <PersonNetworkTab personId={PERSON_ID} view={view()} />
      </StoryProviders>
    );
  },
};

/** Nobody here has spoken to this contact, and nothing has moved. */
export const NoRoute: Story = {
  render: () => {
    installFetchStub({
      [`GET /api/people/${PERSON_ID}/graph`]: () =>
        jsonResponse(
          graph({
            nodes: [
              {
                id: ANCHOR,
                type: "contact",
                group: "anchor",
                label: "Dana Weiss",
              },
            ],
            edges: [],
            route: undefined,
          }),
        ),
    });
    return (
      <StoryProviders>
        <PersonNetworkTab
          personId={PERSON_ID}
          view={view({ relationship_changes: [] })}
        />
      </StoryProviders>
    );
  },
};
