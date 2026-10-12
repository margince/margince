// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One Live List, one Shortlist and the answers their pages read, shared by the
// list stories and tests so both draw the same lists.

import { meFixture } from "../app/mefixture";
import type { FilterVocabulary } from "./filterdata";
import type {
  List,
  ListExplanation,
  ListHistoryEntry,
  ListMember,
} from "./lists.queries";
import { jsonResponse } from "./story-utils";

export const LIVE_ID = "01a0f000-0000-7000-8000-000000000001";
export const SHORTLIST_ID = "01a0f000-0000-7000-8000-000000000002";
export const MEMBER_ID = "01a0f000-0000-7000-8000-000000000003";
/** The reader `listsMe` signs in as, who owns and looks after `liveList`. */
export const READER_ID = "00000000-0000-4000-8000-000000000001";
export const TEAM_ID = "01a0f000-0000-7000-8000-000000000020";
export const OTHER_OWNER_ID = "01a0f000-0000-7000-8000-000000000021";

/** The team the reader belongs to, as `/teams` names it. */
export const teamsPage = {
  data: [{ id: TEAM_ID, name: "Team Germany" }],
  page: { has_more: false },
};

export const liveList: List = {
  id: LIVE_ID,
  name: "Quiet German manufacturers",
  purpose: "Manufacturing prospects with no activity in 45 days",
  entity_type: "company",
  list_type: "dynamic",
  definition: {
    and: [
      { field: "industry", op: "eq", value: "Manufacturing" },
      { field: "cf_last_touch", op: "lt", value: { days_ago: 45 } },
    ],
  },
  sharing: "team",
  owner_id: READER_ID,
  steward_id: READER_ID,
  steward_name: "Lena Vogt",
  version: 3,
  visible_count: 42,
  health: "ok",
  can_edit: true,
  dependencies: [],
  last_check: { checked_at: "2026-09-29T08:15:00Z", outcome: "complete" },
  since_last_visit: { since: "2026-09-28T17:00:00Z", entered: 3, left: 1 },
  joined_since_visit: [MEMBER_ID],
};

/** The company vocabulary naming both fields `liveList` filters on. */
export const liveVocabulary: FilterVocabulary = {
  resource: "company",
  fields: [
    { name: "industry", type: "text", operators: ["eq"], custom: false },
    { name: "cf_last_touch", type: "date", operators: ["lt"], custom: true },
  ],
};

export const shortlist: List = {
  id: SHORTLIST_ID,
  name: "Launch references",
  purpose: "Customers we quote at the October launch",
  entity_type: "company",
  list_type: "static",
  sharing: "workspace",
  steward_id: null,
  steward_name: null,
  version: 7,
  visible_count: 3,
  health: "ownerless",
  can_edit: true,
  dependencies: [],
};

export const members = [
  { id: MEMBER_ID, display_name: "MiTek", version: 4 },
  {
    id: "01a0f000-0000-7000-8000-000000000004",
    display_name: "Nordfracht",
    version: 2,
  },
];

export const liveWhy: ListExplanation = {
  list_id: LIVE_ID,
  entity_id: MEMBER_ID,
  list_type: "dynamic",
  member: true,
  eligible: true,
  clauses: {
    join: "and",
    result: true,
    children: [
      {
        field: "industry",
        op: "eq",
        operand: "Manufacturing",
        result: true,
        value: "Manufacturing",
      },
      {
        field: "cf_last_touch",
        op: "lt",
        operand: { days_ago: 45 },
        result: true,
        hidden: true,
      },
    ],
  },
};

/** A record the Live List does not select: its industry fails the filter. */
export const notOnLiveWhy: ListExplanation = {
  list_id: LIVE_ID,
  entity_id: MEMBER_ID,
  list_type: "dynamic",
  member: false,
  eligible: true,
  clauses: {
    join: "and",
    result: false,
    children: [
      {
        field: "industry",
        op: "eq",
        operand: "Manufacturing",
        result: false,
        value: "Logistics",
      },
      {
        field: "cf_last_touch",
        op: "lt",
        operand: { days_ago: 45 },
        result: true,
        hidden: true,
      },
    ],
  },
};

/** What the Live List's members read says of MiTek: one value shown, one hidden. */
export const liveListing: ListMember = {
  id: MEMBER_ID,
  list_id: LIVE_ID,
  entity_type: "company",
  entity_id: MEMBER_ID,
  added_by: "dynamic",
  values: {
    industry: { value: "Manufacturing", hidden: false },
    cf_last_touch: { hidden: true },
  },
};

/** What the Shortlist's members read says of MiTek: who chose it, when, why. */
export const chosenListing: ListMember = {
  id: "01a0f000-0000-7000-8000-000000000030",
  list_id: SHORTLIST_ID,
  entity_type: "company",
  entity_id: MEMBER_ID,
  added_by: "human:00000000-0000-4000-8000-000000000001",
  added_by_name: "Lena Vogt",
  created_at: "2026-09-20T09:30:00Z",
  note: "Signed the quote for the launch deck",
};

/** One members answer, as the list page reads it for the rows it shows. */
export function listingAnswer(listing: ListMember) {
  return () => jsonResponse({ data: [listing], page: { has_more: false } });
}

export const history: ListHistoryEntry[] = [
  {
    id: "01a0f000-0000-7000-8000-000000000010",
    kind: "member_added",
    occurred_at: "2026-09-20T09:30:00Z",
    actor: "human:00000000-0000-4000-8000-000000000001",
    actor_name: "Lena Vogt",
    reason: "chosen",
    note: "Signed the quote for the launch deck",
  },
];

/** What the Live List's checks saw since the reader's visit on the 28th. */
export const liveHistory: ListHistoryEntry[] = [
  {
    id: "01a0f000-0000-7000-8000-000000000011",
    kind: "member_entered",
    occurred_at: "2026-09-29T08:15:00Z",
    actor: "system:list-checker",
    entity_type: "company",
    entity_id: MEMBER_ID,
    reason: "filter_changed",
    definition_version: 3,
  },
  {
    id: "01a0f000-0000-7000-8000-000000000012",
    kind: "member_left",
    occurred_at: "2026-09-29T08:00:00Z",
    actor: "system:list-checker",
    entity_type: "company",
    entity_id: "01a0f000-0000-7000-8000-000000000005",
    reason: "evaluated",
    definition_version: 2,
  },
];

/** The visit a list page records, answering the reader's visit on the 28th. */
export function visitAnswer(listID: string) {
  return () =>
    jsonResponse({
      list_id: listID,
      visited_at: "2026-09-30T09:00:00Z",
      previous_visit_at: "2026-09-28T17:00:00Z",
    });
}

/**
 * The session probe of a reader on an installation with lists on or off, a
 * member of the given teams.
 */
export function listsMe(on: boolean, teams: readonly string[] = []) {
  return () =>
    jsonResponse({
      ...meFixture({
        allow: {
          list: ["read", "create", "update", "delete"],
          company: ["read"],
        },
        settingsAvailability: { lists: on },
      }),
      teams,
    });
}
