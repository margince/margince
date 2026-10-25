import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { UTC_ZONE } from "../../format/timezone";
import { type Translator, translate } from "../../i18n";
import { bucketLabel } from "../../lib/bucketlabel";
import { directionSentence } from "./contactrail";
import { awaitsOurReply, contactDims, contactVerdict } from "./contactstanding";
import {
  contactFigures,
  contactGap,
  contactTimelineEvents,
  type StandingWords,
} from "./contactstandingfigures";

type Contact360 = components["schemas"]["Contact360"];
type ContactMomentRule = components["schemas"]["ContactMomentRule"];
type Strength = components["schemas"]["RelationshipStrength"];
type Claim = components["schemas"]["ConversationClaim"];

const t: Translator = (key, params) => translate("en", key, params);
const words: StandingWords = { t, locale: "en", zone: UTC_ZONE };

const AS_OF = "2026-08-13T09:00:00Z";
const VIEW: Contact360 = {
  as_of: AS_OF,
  contact: {
    id: "p-1",
    full_name: "Dana Buyer",
    source: "manual",
    captured_by: "human:u-1",
    created_at: "2026-06-01T08:00:00Z",
    updated_at: "2026-08-01T08:00:00Z",
  },
  sections_omitted: [],
};

function strength(bucket: Strength["bucket"], score: number): Strength {
  return {
    score,
    bucket,
    factors: { recency: 0, frequency: 0, reciprocity: 0, direction: 0 },
    inbound_90d: 4,
    outbound_90d: 6,
  };
}

function moment(rule: ContactMomentRule): Contact360["moment"] {
  return {
    claim_key: `moment:${rule}`,
    evidence_fingerprint: "f",
    rule,
    headline: "Headline",
    why_now: `Why ${rule}`,
    confidence: "observed_fact",
    evidence: [],
    recommended_action: {
      kind: "log_activity",
      label: "Log",
      state: "available",
    },
  };
}

function promise(id: string, due: string | null): Claim {
  return {
    id,
    kind: "commitment_ours",
    body: `Promise ${id}`,
    source_activity_id: "a-1",
    source_quote: "We will send it",
    status: "open",
    needs_review: false,
    due_at: due,
  };
}

describe("contactVerdict", () => {
  it.each<[ContactMomentRule, string]>([
    ["overdue_promise", "danger"],
    ["gone_quiet", "danger"],
    ["open_promise", "warning"],
    ["missing_next_step", "warning"],
    ["meeting_prep", "warning"],
    ["job_change", "warning"],
    ["public_signal", "warning"],
    ["re_engaged", "success"],
    ["thin_relationship", "neutral"],
  ])(
    "reads the %s rung in %s, without repeating the Next bar's reason",
    (rule, tone) => {
      const verdict = contactVerdict(
        { ...VIEW, moment: moment(rule), strength: strength("strong", 80) },
        t,
      );
      expect(verdict).toMatchObject({
        word: t(`contact.moment.rule.${rule}`),
        tone,
      });
      expect(verdict?.sentence).not.toBe(`Why ${rule}`);
    },
  );

  it.each<[Strength["bucket"], string, string]>([
    ["strong", "Strong", "success"],
    ["moderate", "Moderate", "warning"],
    ["weak", "Weak", "danger"],
  ])("reads a %s band when nothing is asked", (bucket, word, tone) => {
    const verdict = contactVerdict(
      {
        ...VIEW,
        moment: moment("nothing_needed"),
        strength: strength(bucket, 40),
      },
      t,
    );
    expect(verdict?.word).toBe(word);
    expect(verdict?.tone).toBe(tone);
  });

  it("says no interactions yet for the none band, never a zero", () => {
    expect(
      contactVerdict({ ...VIEW, strength: strength("none", 0) }, t),
    ).toEqual({ word: "No interactions yet", tone: "neutral" });
  });

  it("falls back to the quiet rung's word when the strength is withheld", () => {
    expect(
      contactVerdict(
        {
          ...VIEW,
          moment: moment("nothing_needed"),
          sections_omitted: ["strength"],
        },
        t,
      ),
    ).toEqual({
      word: t("contact.moment.rule.nothing_needed"),
      tone: "neutral",
    });
  });

  it("has no verdict when the moment and the strength are both withheld", () => {
    expect(
      contactVerdict({ ...VIEW, sections_omitted: ["moments", "strength"] }, t),
    ).toBeNull();
  });
});

describe("directionSentence", () => {
  it("words the server's direction, not the two dates", () => {
    // Both dates set, but the server read only our side as written by a person.
    const view: Contact360 = {
      ...VIEW,
      last_inbound_at: "2026-08-10T09:00:00Z",
      last_outbound_at: "2026-08-11T09:00:00Z",
      relationship_direction: "outbound_only",
    };
    expect(directionSentence(view, t)).toBe(
      t("contact.standing.direction.outboundOnly"),
    );
  });

  it("says none for a server that sent no reading", () => {
    expect(directionSentence(VIEW, t)).toBe(
      t("contact.standing.direction.none"),
    );
  });
});

describe("awaitsOurReply", () => {
  it.each<[string | null, string | null, boolean]>([
    ["2026-08-01T09:00:00Z", null, true],
    ["2026-08-01T09:00:00Z", "2026-07-20T09:00:00Z", true],
    ["2026-07-20T09:00:00Z", "2026-08-01T09:00:00Z", false],
    [null, "2026-08-01T09:00:00Z", false],
  ])("inbound %s, outbound %s: %s", (inbound, outbound, expected) => {
    expect(
      awaitsOurReply({
        ...VIEW,
        last_inbound_at: inbound,
        last_outbound_at: outbound,
      }),
    ).toBe(expected);
  });

  it("never claims a move over withheld touch dates", () => {
    expect(
      awaitsOurReply({
        ...VIEW,
        sections_omitted: ["last_touch"],
        last_inbound_at: "2026-08-01T09:00:00Z",
      }),
    ).toBe(false);
  });
});

describe("contactDims", () => {
  it("counts our overdue promises against every open one", () => {
    const dims = contactDims(
      {
        ...VIEW,
        claims: [
          promise("c-1", "2026-06-08T09:00:00Z"),
          promise("c-2", "2026-09-01T09:00:00Z"),
          { ...promise("c-3", null), status: "done" },
        ],
        strength: strength("moderate", 41),
        network: {
          colleagues: [
            {
              user_id: "u-1",
              display_name: "Lena Fischer",
              strength_bucket: "strong",
              interactions_90d: 3,
            },
            {
              user_id: "u-2",
              display_name: "Tom Berger",
              strength_bucket: "none",
              interactions_90d: 0,
            },
          ],
        },
      },
      t,
      "en",
    );
    expect(dims.map((dim) => [dim.key, dim.value, dim.sub, dim.tone])).toEqual([
      ["promises", "1 overdue", "of 2 open · Promise c-1", "danger"],
      ["exchange", "4 in · 6 out", "Last 90 days", "success"],
      // One colleague in touch is a single thread, not coverage.
      ["reach", "1 colleague in touch", "Lena Fischer", "warning"],
    ]);
  });

  it("says none open and no colleague in touch rather than zero", () => {
    const dims = contactDims(
      { ...VIEW, claims: [], network: { colleagues: [] } },
      t,
      "en",
    );
    expect(dims.map((dim) => [dim.value, dim.tone])).toEqual([
      ["None open", "neutral"],
      ["No colleague in touch", "neutral"],
    ]);
  });

  it("draws no row for a withheld section", () => {
    expect(
      contactDims(
        { ...VIEW, sections_omitted: ["claims", "network", "strength"] },
        t,
        "en",
      ),
    ).toEqual([]);
  });
});

describe("contactFigures", () => {
  it("reads the four cells from the 360, never an empty value", () => {
    const figures = contactFigures(
      {
        ...VIEW,
        last_inbound_at: "2026-08-01T09:00:00Z",
        last_outbound_at: "2026-07-20T09:00:00Z",
        activities: {
          data: [
            {
              offers_email: false,
              id: "a-1",
              kind: "email",
              subject: "Re: retrofit timeline",
              occurred_at: "2026-08-01T09:00:00Z",
              is_done: true,
              source: "gmail",
              captured_by: "connector:gmail",
              created_at: "2026-08-01T09:00:00Z",
              updated_at: "2026-08-01T09:00:00Z",
              email_summary: {
                activity_id: "a-1",
                occurred_at: "2026-08-01T09:00:00Z",
                version: 1,
                direction: "inbound",
                display_status: "team",
                move: "needs_reply",
                attachment_count: 0,
              },
            },
          ],
          page: { has_more: false },
        },
        commercial: { committee: [] },
      },
      words,
    );
    expect(figures.map((cell) => [cell.key, cell.value, cell.tone])).toEqual([
      ["lastReply", "1 Aug", "warning"],
      ["unanswered", "1 email", "warning"],
      ["openDeal", "None", "neutral"],
      ["nextMeeting", "None scheduled", "neutral"],
    ]);
    expect(figures[0]?.detail).toBe("12 days ago");
  });

  it("says a withheld section is not shown rather than none", () => {
    const figures = contactFigures(
      {
        ...VIEW,
        sections_omitted: [
          "last_touch",
          "activities",
          "commercial",
          "next_meeting",
        ],
      },
      words,
    );
    expect(new Set(figures.map((cell) => cell.value))).toEqual(
      new Set([t("record.notShown")]),
    );
  });
});

describe("the timeline", () => {
  it("dates a slipped promise as late and the newest message as last", () => {
    const events = contactTimelineEvents(
      {
        ...VIEW,
        last_inbound_at: "2026-08-01T09:00:00Z",
        claims: [promise("c-1", "2026-06-08T09:00:00Z")],
        next_meeting: {
          activity_id: "m-2",
          starts_at: "2026-08-20T09:00:00Z",
          subject: "Walkthrough",
        },
      },
      words,
    );
    expect(events.map((event) => [event.title, event.kind])).toEqual([
      ["Commitment due", "late"],
      ["Dana Buyer wrote", "last"],
      ["Walkthrough", "ahead"],
    ]);
  });

  it("names the silence after their message as no reply", () => {
    expect(
      contactGap(
        {
          ...VIEW,
          last_inbound_at: "2026-08-01T09:00:00Z",
          last_outbound_at: "2026-07-20T09:00:00Z",
          last_contact_at: "2026-08-01T09:00:00Z",
        },
        words,
      ),
    ).toEqual({
      from: "2026-08-01T09:00:00Z",
      title: "12 days",
      sub: "no reply",
    });
  });

  it("counts the silence from a meeting, not only from the last message", () => {
    expect(
      contactGap(
        {
          ...VIEW,
          last_inbound_at: "2026-07-20T09:00:00Z",
          last_outbound_at: "2026-07-25T09:00:00Z",
          last_contact_at: "2026-08-05T09:00:00Z",
        },
        words,
      )?.title,
    ).toBe("8 days");
  });
});

describe("bucketLabel", () => {
  const buckets: Strength["bucket"][] = ["none", "weak", "moderate", "strong"];
  it.each(["en", "de", "vi"] as const)(
    "names no signal yet for none and the band word otherwise (%s)",
    (locale) => {
      const say: Translator = (key, params) => translate(locale, key, params);
      for (const bucket of buckets) {
        const want =
          bucket === "none"
            ? translate(locale, "standing.ring.noInteractions")
            : translate(locale, `contact.bandBadge.${bucket}`);
        expect(bucketLabel(bucket, say)).toBe(want);
      }
    },
  );

  it("is the word the contact page's verdict prints", () => {
    for (const bucket of buckets) {
      const verdict = contactVerdict(
        { ...VIEW, strength: strength(bucket, 40) },
        t,
      );
      expect(verdict?.word).toBe(bucketLabel(bucket, t));
    }
  });
});
