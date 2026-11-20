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

function call(
  id: string,
  call_outcome?: Activity["call_outcome"],
  subject?: string,
): Activity {
  return {
    id,
    subject,
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
      [
        call("a", "reached", "Renewal"),
        call("b", "voicemail", "Renewal"),
        call("c", undefined, "Renewal"),
      ],
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
      [call("a", "not_reached", "Renewal")],
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

  it("titles a call with no subject or details by its outcome, said once", () => {
    const [row] = activityTimeline(
      [call("a", "reached")],
      undefined,
      undefined,
      {
        t,
        locale: "en",
      },
    );
    expect(row?.title).toBe("Reached");
    expect(row?.qualifier).toBeUndefined();
  });

  it("titles an empty call without an outcome by its kind, in words", () => {
    const [row] = activityTimeline([call("a")], undefined, undefined, {
      t,
      locale: "en",
    });
    expect(row?.title).toBe("Call");
  });

  it("keeps a call's subject as its title and the outcome beside it", () => {
    const [row] = activityTimeline(
      [call("a", "voicemail", "Renewal")],
      undefined,
      undefined,
      { t, locale: "en" },
    );
    expect(row?.title).toBe("Renewal");
    expect(row?.qualifier).toBe("Left a voicemail");
  });
});
