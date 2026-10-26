import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { UTC_ZONE, viewerZone } from "../../format/timezone";
import { type Translator, translate } from "../../i18n";
import { withShippedLabels } from "../../lib/shippedlabels";
import type { LeadDisqualifyReason } from "./leadhandling";
import { leadStanding } from "./leadstanding";

type Lead = components["schemas"]["Lead"];

// The call is a fact the server already decided, said in a word. These pin
// which fact each word is read from — and that nothing is read from a silence
// the server did not judge.

const t: Translator = (key: string, params?: Record<string, string>) =>
  params ? `${key}:${JSON.stringify(params)}` : key;

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
  // says this lead ended. Read from `status`, it would be open work forever.
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
      zone,
      [],
    );
    expect(merged).toEqual({
      word: "lead.standing.merged",
      sentence: "lead.standing.mergedBecause",
      tone: "neutral",
    });
  });

  it.each(["manual", "import", "webform"])(
    "reports the recorded status of an unanswered %s lead without claiming a reply is owed",
    (source) => {
      const standing = leadStanding(
        lead({
          source,
          sla_state: "breached",
          sla_deadline_at: "2026-08-19T08:14:00Z",
        }),
        t,
        "en",
        zone,
        zone,
        [],
      );
      expect(standing).toEqual({
        word: "lead.status.new",
        sentence: "lead.standing.noResponse",
        tone: "neutral",
      });
    },
  );

  it("is their move once we answered, and in motion once they engaged", () => {
    const answered = leadStanding(
      lead({ status: "contacted", first_response_at: "2026-08-21T09:12:00Z" }),
      t,
      "en",
      zone,
      zone,
      [],
    );
    expect(answered.word).toBe("lead.standing.theirMove");
    expect(answered.tone).toBe("success");

    const engaged = leadStanding(
      lead({
        status: "engaged",
        full_name: "Jonas Weber",
        first_response_at: "2026-08-21T09:12:00Z",
        qualification_evidence: {
          trigger: "meeting_booked",
          occurred_at: "2026-08-23T07:40:00Z",
        },
      }),
      t,
      "en",
      UTC_ZONE,
      UTC_ZONE,
      [],
    );
    expect(engaged.word).toBe("lead.standing.inMotion");
    expect(engaged.sentence).toBe(
      'lead.standing.meetingBookedFor:{"name":"Jonas Weber","at":"23 Aug 2026"}',
    );
  });

  // A `day` meeting is stored at noon on the record's clock. West of that
  // zone the same instant falls on the day before.
  it("dates the engaging signal on the record's clock", () => {
    const engaged = leadStanding(
      lead({
        status: "engaged",
        full_name: "Jonas Weber",
        qualification_evidence: {
          trigger: "meeting_held",
          occurred_at: "2026-08-23T05:00:00Z",
        },
      }),
      t,
      "en",
      "America/Los_Angeles",
      "Asia/Ho_Chi_Minh",
      [],
    );
    expect(engaged.sentence).toBe(
      'lead.standing.meetingHeldOn:{"name":"Jonas Weber","at":"23 Aug 2026"}',
    );
  });

  // A lead moved to Engaged by hand has no captured signal to date, so it
  // keeps the general sentence rather than a date nothing recorded.
  it("keeps the general sentence for an engaged lead with no evidence", () => {
    const engaged = leadStanding(
      lead({ status: "engaged" }),
      t,
      "en",
      zone,
      zone,
      [],
    );
    expect(engaged).toEqual({
      word: "lead.status.engaged",
      sentence: "lead.standing.engagedBecause",
      tone: "success",
    });
  });

  it("reads a closed lead off how it was closed", () => {
    const qualified = leadStanding(
      lead({ status: "promoted", promoted_at: "2026-08-27T16:40:00Z" }),
      t,
      "en",
      zone,
      zone,
      [],
    );
    expect(qualified.word).toBe("lead.standing.qualified");
    expect(qualified.tone).toBe("success");

    const closed = leadStanding(
      lead({
        status: "disqualified",
        disqualify_reason: "No budget this year",
      }),
      t,
      "en",
      zone,
      zone,
      [],
    );
    expect(closed.word).toBe("lead.standing.closed");
    // Neutral, not the all-clear: a closed lead is not good news dressed green.
    expect(closed.tone).toBe("neutral");
    expect(closed.sentence).toBe(
      'lead.standing.closedFor:{"reason":"No budget this year"}',
    );
  });

  // The server's `disqualify_reason` is the stored English label, so the
  // sentence names the reason the list carries in the reader's language.
  it("names a built-in reason in the reader's language and an edited one as typed", () => {
    const de: Translator = (key, params) => translate("de", key, params);
    const reasons = withShippedLabels(
      [
        reason("r-1", "No budget", "leadReasons.builtin.no_budget"),
        reason("r-2", "Budget frozen until Q3", null),
      ],
      de,
    );
    const closedFor = (id: string, stored: string) =>
      leadStanding(
        lead({
          status: "disqualified",
          disqualify_reason_id: id,
          disqualify_reason: stored,
        }),
        de,
        "de",
        zone,
        zone,
        reasons,
      ).sentence;

    expect(closedFor("r-1", "No budget")).toBe(
      "Geschlossen: Kein Budget. Der Datensatz bleibt als Verlauf erhalten.",
    );
    expect(closedFor("r-2", "Budget frozen until Q3")).toBe(
      "Geschlossen: Budget frozen until Q3. Der Datensatz bleibt als Verlauf erhalten.",
    );
    // A reason the list does not hold keeps the server's label.
    expect(closedFor("r-9", "Removed reason")).toBe(
      "Geschlossen: Removed reason. Der Datensatz bleibt als Verlauf erhalten.",
    );
  });
});

function reason(
  id: string,
  label: string,
  labelKey: string | null,
): LeadDisqualifyReason {
  return {
    id,
    label,
    label_key: labelKey,
    sort_order: 10,
    active: true,
    system: labelKey !== null,
    lead_count: 1,
    version: 1,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}
