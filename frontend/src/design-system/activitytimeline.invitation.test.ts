// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { translate } from "../i18n";
import { activityTimeline } from "./activitytimeline";

type Activity = components["schemas"]["Activity"];

const t = ((
  key: Parameters<typeof translate>[1],
  values?: Record<string, string>,
) => translate("en", key, values)) as never;

function meeting(id: string, invitation?: Activity["invitation"]): Activity {
  return {
    id,
    kind: "meeting",
    subject: "Kickoff",
    occurred_at: "2026-10-20T10:00:00Z",
    occurred_precision: "instant",
    links: [],
    source: "calendar",
    is_done: false,
    offers_email: false,
    created_at: "2026-10-20T10:00:00Z",
    updated_at: "2026-10-20T10:00:00Z",
    captured_by: "system",
    invitation,
  };
}

// The invitation mail History folds onto a meeting reads as one line on it.
describe("a meeting's folded invitation on the timeline", () => {
  it("says who was invited and names only the answers given", () => {
    const [answered, unanswered, plain] = activityTimeline(
      [
        meeting("a", { invited: 3, accepted: 1, declined: 1, tentative: 0 }),
        meeting("b", { invited: 2, accepted: 0, declined: 0, tentative: 0 }),
        meeting("c"),
      ],
      undefined,
      undefined,
      { nameOf: () => undefined, t, locale: "en" },
    );
    expect(answered?.qualifier).toBe("3 invited · 1 accepted · 1 declined");
    expect(unanswered?.qualifier).toBe("2 invited");
    expect(plain?.qualifier).toBeUndefined();
  });
});
