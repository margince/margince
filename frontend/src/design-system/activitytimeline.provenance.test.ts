// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { activityTimeline } from "./activitytimeline";

type Activity = components["schemas"]["Activity"];

const ME = "4f1c7a2e-8b3d-4e5f-9a6b-7c8d9e0f1a2b";

function note(overrides: Partial<Activity>): Activity {
  return {
    id: "n-1",
    kind: "note",
    subject: "Risk assessment",
    body: "No reply in three weeks.",
    occurred_at: "2026-10-20T10:00:00Z",
    occurred_precision: "instant",
    links: [],
    source: "manual",
    is_done: false,
    offers_email: false,
    created_at: "2026-10-20T10:00:00Z",
    updated_at: "2026-10-20T10:00:00Z",
    captured_by: `human:${ME}`,
    ...overrides,
  };
}

describe("who a timeline row says wrote a note", () => {
  it("names the assistant when the server says an assistant wrote it", () => {
    const [row] = activityTimeline([note({ recorded_by_agent: true })], ME);
    expect(row?.provenance).toEqual({ kind: "agent" });
  });

  it("keeps the reader's own note theirs", () => {
    const [row] = activityTimeline([note({ recorded_by_agent: false })], ME);
    expect(row?.provenance).toMatchObject({ kind: "human", self: true });
  });

  it("keeps the colleague an assistant worked for", () => {
    const [row] = activityTimeline(
      [
        note({
          captured_by: "agent:0b9f3c2a-6d7e-4c1b-9a8f-1e2d3c4b5a69",
          recorded_by_agent: true,
          created_by: {
            actor_type: "agent",
            actor_id: "agent:0b9f3c2a-6d7e-4c1b-9a8f-1e2d3c4b5a69",
            agent_client: "Claude",
            on_behalf_of_name: "Anna Weber",
          },
        }),
      ],
      ME,
    );
    expect(row?.provenance).toEqual({
      kind: "agent",
      agent: "Claude",
      forName: "Anna Weber",
    });
  });
});
