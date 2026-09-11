import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { viewerZone } from "../format/timezone";
import type { Translator } from "../i18n";
import { leadStanding } from "./leadstanding";

type Lead = components["schemas"]["Lead"];

// The call is a fact the server already decided, said in a word. These pin
// which fact each word is read from — and that nothing is read from a silence
// the server did not judge.

const t: Translator = ((key: string, params?: Record<string, unknown>) =>
  params ? `${key}:${JSON.stringify(params)}` : key) as Translator;

// The zone only satisfies the signature: no case here asserts a rendered date,
// so the reader's own zone is the honest one to hand over.
const zone = viewerZone();

function lead(extra: Partial<Lead>): Lead {
  return {
    id: "l1",
    status: "new",
    score: 0,
    source: "webform",
    captured_by: "human:u1",
    created_at: "2026-08-18T08:14:00Z",
    updated_at: "2026-08-18T08:14:00Z",
    ...extra,
  };
}

describe("leadStanding", () => {
  // The merge leaves the ladder alone, so the pointer is the only thing that
  // says this lead ended. Read from `status` instead, a lead merged away mid
  // conversation is open work forever — and the terminal reading it would
  // eventually get is "disqualified", which says a human judged the prospect
  // not worth pursuing rather than that two records were one prospect.
  it("says a merged-away lead was merged, whatever its ladder still says", () => {
    const merged = leadStanding(
      lead({
        status: "contacted",
        first_response_at: "2026-08-19T09:00:00Z",
        merged_into_id: "l2",
        archived_at: "2026-08-20T10:00:00Z",
      }),
      t,
      "en",
      zone,
    );
    expect(merged.label).toBe("lead.standing.merged");
    expect(merged.because).toBe("lead.standing.mergedBecause");
    expect(merged.restsOn.map((r) => r.quote)).toContain(
      "lead.standing.rests.merged",
    );
  });

  it("is our move on an unanswered lead, as loud as the first-response clock", () => {
    const breached = leadStanding(
      lead({ sla_state: "breached", sla_deadline_at: "2026-08-19T08:14:00Z" }),
      t,
      "en",
      zone,
    );
    expect(breached.label).toBe("lead.standing.yourMove");
    expect(breached.tone).toBe("danger");
    expect(breached.because).toContain("lead.standing.overdueSince");

    const soon = leadStanding(
      lead({ sla_state: "at_risk", sla_deadline_at: "2026-08-19T08:14:00Z" }),
      t,
      "en",
      zone,
    );
    expect(soon.tone).toBe("warn");
    expect(soon.because).toContain("lead.standing.dueBy");
  });

  it("carries no alarm when the installation runs no first-response clock", () => {
    const standing = leadStanding(lead({}), t, "en", zone);
    expect(standing.label).toBe("lead.standing.yourMove");
    expect(standing.tone).toBe("accent");
    expect(standing.because).toBe("lead.standing.noResponse");
  });

  it("reads a deadline the server has not judged as no clock at all", () => {
    const standing = leadStanding(
      lead({ sla_deadline_at: "2026-08-19T08:14:00Z" }),
      t,
      "en",
      zone,
    );
    expect(standing.tone).toBe("accent");
    expect(standing.because).toBe("lead.standing.noResponse");
  });

  it("is their move once we answered, and in motion once they engaged", () => {
    const answered = leadStanding(
      lead({ status: "contacted", first_response_at: "2026-08-21T09:12:00Z" }),
      t,
      "en",
      zone,
    );
    expect(answered.label).toBe("lead.standing.theirMove");
    expect(answered.tone).toBe("accent");

    const engaged = leadStanding(
      lead({
        status: "engaged",
        first_response_at: "2026-08-21T09:12:00Z",
        qualification_evidence: {
          trigger: "meeting_booked",
          occurred_at: "2026-08-23T07:40:00Z",
        },
      }),
      t,
      "en",
      zone,
    );
    expect(engaged.label).toBe("lead.standing.inMotion");
    expect(engaged.restsOn.map((row) => row.key)).toEqual([
      "response",
      "evidence",
    ]);
  });

  it("reads a closed lead off how it was closed", () => {
    const qualified = leadStanding(
      lead({ status: "promoted", promoted_at: "2026-08-27T16:40:00Z" }),
      t,
      "en",
      zone,
    );
    expect(qualified.label).toBe("lead.standing.qualified");
    expect(qualified.tone).toBe("calm");

    const closed = leadStanding(
      lead({
        status: "disqualified",
        disqualify_reason: "No budget this year",
      }),
      t,
      "en",
      zone,
    );
    expect(closed.label).toBe("lead.standing.closed");
    // Neutral, not the all-clear: a closed lead is not good news dressed green.
    expect(closed.tone).toBe("unknown");
    expect(closed.restsOn[0].quote).toBe("No budget this year");
  });
});
