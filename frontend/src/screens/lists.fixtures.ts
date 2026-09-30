// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One Live List, one Shortlist and the answers their pages read, shared by the
// list stories and tests so both draw the same lists.

import { meFixture } from "../app/mefixture";
import type { List, ListExplanation, ListHistoryEntry } from "./lists.queries";
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
  { id: MEMBER_ID, display_name: "MiTek" },
  { id: "01a0f000-0000-7000-8000-000000000004", display_name: "Nordfracht" },
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

export const chosenWhy: ListExplanation = {
  list_id: SHORTLIST_ID,
  entity_id: MEMBER_ID,
  list_type: "static",
  member: true,
  added_by: "human:00000000-0000-4000-8000-000000000001",
  added_by_name: "Lena Vogt",
  added_at: "2026-09-20T09:30:00Z",
  note: "Signed the quote for the launch deck",
};

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
