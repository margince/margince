import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { UTC_ZONE } from "../../format/timezone";
import { type Translator, translate } from "../../i18n";
import { strengthSentence } from "./contactstrengthsentence";

type Strength = components["schemas"]["RelationshipStrength"];

const t: Translator = (key, params) => translate("en", key, params);
const TWO_DAYS_AGO = "2026-08-11T09:00:00Z";

// Factors as the server folds them two days after the last interaction:
// recency 2^(-2/30), frequency count/20, reciprocity 0.25 + 0.75 * balance.
function weak(
  counts: { all: number; inbound?: number; outbound?: number },
  overrides: Partial<Strength> = {},
): Strength {
  const { inbound, outbound } = counts;
  const directed = (inbound ?? 0) + (outbound ?? 0);
  const balance =
    directed > 0
      ? 1 - Math.abs((inbound ?? 0) - (outbound ?? 0)) / directed
      : 0;
  return {
    score: 10,
    bucket: "weak",
    factors: {
      recency: 2 ** (-2 / 30),
      frequency: counts.all / 20,
      reciprocity: 0.25 + 0.75 * balance,
      direction: 0,
    },
    last_interaction: TWO_DAYS_AGO,
    inbound_90d: inbound,
    outbound_90d: outbound,
    ...overrides,
  };
}

const sentence = (strength: Strength) =>
  strengthSentence(strength, t, "en", UTC_ZONE);

describe("strengthSentence", () => {
  it("never calls a weak score with recent contact rare", () => {
    const recent = [
      weak({ all: 3, inbound: 0, outbound: 3 }),
      weak({ all: 4, inbound: 2, outbound: 2 }),
      weak({ all: 2, inbound: 0, outbound: 0 }),
      weak({ all: 2, inbound: 2, outbound: 0 }),
    ];
    for (const strength of recent) {
      expect(sentence(strength)).toMatch(/^Weak: recent contact/);
      expect(sentence(strength)).not.toMatch(/rarely|a while/);
    }
  });

  it("states outbound-only evidence as no reply counted, not none sent", () => {
    expect(sentence(weak({ all: 3, inbound: 0, outbound: 3 }))).toBe(
      "Weak: recent contact, with limited evidence of a two-way exchange. Last contact was on 11 Aug. The score uses shared interactions only, and no qualifying replies from this contact were counted.",
    );
  });

  it("names how few interactions hold back a balanced two-way exchange", () => {
    expect(sentence(weak({ all: 4, inbound: 2, outbound: 2 }))).toBe(
      "Weak: recent contact, but few shared interactions in the last 90 days. Last contact was on 11 Aug.",
    );
  });

  it("calls an exchange one way only when one side far outweighs the other", () => {
    expect(sentence(weak({ all: 4, inbound: 1, outbound: 3 }))).toContain(
      "few shared interactions",
    );
    expect(sentence(weak({ all: 18, inbound: 1, outbound: 17 }))).toBe(
      "Weak: recent contact, but the shared exchange runs mostly one way. Last contact was on 11 Aug.",
    );
  });

  it("says no shared interaction counted as a message either way", () => {
    expect(sentence(weak({ all: 2, inbound: 0, outbound: 0 }))).toContain(
      "No shared interaction was counted as a message from this contact or from your team.",
    );
  });

  it("says nothing of ours was counted when only they wrote", () => {
    expect(sentence(weak({ all: 2, inbound: 2, outbound: 0 }))).toContain(
      "no qualifying messages from your team were counted",
    );
  });

  it("says it has been a while once recency has decayed", () => {
    const old = weak(
      { all: 1, inbound: 0, outbound: 1 },
      {
        last_interaction: "2026-05-01T09:00:00Z",
        factors: {
          recency: 0.1,
          frequency: 0.05,
          reciprocity: 0.25,
          direction: 0,
        },
      },
    );
    expect(sentence(old)).toBe(
      "Weak: it has been a while. You last were in touch on 1 May.",
    );
  });

  it("reads no interactions as none, not as weak", () => {
    expect(
      sentence({
        score: 0,
        bucket: "none",
        factors: { recency: 0, frequency: 0, reciprocity: 0, direction: 0 },
        last_interaction: null,
      }),
    ).toBe(t("standing.ring.noInteractions"));
  });

  it("falls back to the factors when the counts are not on the wire", () => {
    expect(sentence(weak({ all: 2 }, { inbound_90d: undefined }))).toBe(
      "Weak: recent contact, but the shared exchange runs mostly one way. Last contact was on 11 Aug.",
    );
  });

  it("keeps the moderate and strong sentences per band", () => {
    const moderate = weak(
      { all: 10, inbound: 5, outbound: 5 },
      { bucket: "moderate" },
    );
    expect(sentence(moderate)).toMatch(/^Moderate:/);
  });
});
