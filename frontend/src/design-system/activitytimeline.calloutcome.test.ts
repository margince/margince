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

function call(id: string, call_outcome?: Activity["call_outcome"]): Activity {
  return {
    id,
    kind: "call",
    occurred_at: "2026-10-20T10:00:00Z",
    occurred_precision: "instant",
    links: [],
    source: "manual",
    is_done: false,
    offers_email: false,
    created_at: "2026-10-20T10:00:00Z",
    updated_at: "2026-10-20T10:00:00Z",
    captured_by: "human:u-1",
    call_outcome,
  };
}

// How a call ended sits where a direction sits, in the reader's words.
describe("a call's outcome on the timeline", () => {
  it("reads the outcome's label on a call that has one, and nothing otherwise", () => {
    const [reached, voicemail, none] = activityTimeline(
      [call("a", "reached"), call("b", "voicemail"), call("c")],
      undefined,
      undefined,
      { nameOf: () => undefined, t, locale: "en" },
    );
    expect(reached?.qualifier).toBe("Reached");
    expect(voicemail?.qualifier).toBe("Left a voicemail");
    expect(none?.qualifier).toBeUndefined();
  });

  it("labels the same call in German", () => {
    const de = ((key: Parameters<typeof translate>[1]) =>
      translate("de", key)) as never;
    const [row] = activityTimeline(
      [call("a", "not_reached")],
      undefined,
      undefined,
      {
        nameOf: () => undefined,
        t: de,
        locale: "de",
      },
    );
    expect(row?.qualifier).toBe("Nicht erreicht");
  });
});
