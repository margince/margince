// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { activityTimeline } from "./activitytimeline";

type Activity = components["schemas"]["Activity"];

const PASSPORT = "agent:0b9f3c2a-6d7e-4c1b-9a8f-1e2d3c4b5a69";

function note(createdBy: Activity["created_by"]): Activity {
  return {
    id: "a-note",
    kind: "note",
    body: "Asked for the renewal terms.",
    is_done: false,
    offers_email: false,
    occurred_at: "2026-10-21T10:00:00Z",
    occurred_precision: "instant",
    source: "manual",
    captured_by: PASSPORT,
    created_at: "2026-10-21T10:00:00Z",
    updated_at: "2026-10-21T10:00:00Z",
    created_by: createdBy,
  };
}

const provenanceOf = (row: Activity) => activityTimeline([row])[0]?.provenance;

describe("an activity an agent recorded", () => {
  it("names the colleague the agent worked for and the tool it was typed through", () => {
    expect(
      provenanceOf(
        note({
          actor_type: "agent",
          actor_id: PASSPORT,
          on_behalf_of_name: "Anna Weber",
          agent_client: "Claude",
        }),
      ),
    ).toEqual({ kind: "agent", agent: "Claude", forName: "Anna Weber" });
  });

  it("keeps the unnamed tag when no colleague stands behind the agent", () => {
    expect(
      provenanceOf(note({ actor_type: "agent", actor_id: PASSPORT })),
    ).toEqual({ kind: "agent" });
  });

  it("keeps the unnamed tag when the create row is not known", () => {
    expect(provenanceOf(note(undefined))).toEqual({ kind: "agent" });
  });
});
