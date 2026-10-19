// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { populated } from "./contactpage.fixtures";
import { contactRings } from "./contactrings";

type Contact360 = components["schemas"]["Contact360"];

const sam = populated.network?.colleagues[0];
const ours = populated.claims?.find(
  (claim) => claim.kind === "commitment_ours",
);
if (!sam || !ours) {
  throw new Error("the populated contact fixture lost its colleague or promise");
}

const strength: NonNullable<Contact360["strength"]> = {
  score: 70,
  bucket: "strong",
  factors: { recency: 1, frequency: 1, reciprocity: 1, direction: 1 },
  inbound_90d: 9,
  outbound_90d: 11,
};

// Nine of twenty messages hers, one of two colleagues in touch this quarter,
// one of two promises kept and one dismissed.
const view: Contact360 = {
  ...populated,
  strength,
  network: {
    colleagues: [sam, { ...sam, user_id: "u-3", interactions_90d: 0 }],
  },
  claims: [
    ...(populated.claims ?? []),
    { ...ours, id: "c-kept", status: "done" },
    { ...ours, id: "c-dropped", status: "dismissed" },
  ],
};

describe("a contact's rings", () => {
  it("count her part of the exchange, the colleagues in touch and the promises kept", () => {
    expect(contactRings(view)).toEqual([
      { key: "conversation", value: 9, max: 20, tone: "calm" },
      { key: "reach", value: 1, max: 2, tone: "warning" },
      { key: "promises", value: 1, max: 2, tone: "warning" },
    ]);
  });

  it("call a conversation one way when under a third of it is hers", () => {
    const quiet: Contact360 = {
      ...view,
      strength: { ...strength, inbound_90d: 2, outbound_90d: 10 },
    };
    expect(contactRings(quiet)[0]).toEqual({
      key: "conversation",
      value: 2,
      max: 12,
      tone: "warning",
    });
  });

  it("leave out what was withheld or has nothing to count, rather than draw a zero", () => {
    const withheld: Contact360 = {
      ...view,
      sections_omitted: ["strength", "claims"],
      network: { colleagues: [] },
    };
    expect(contactRings(withheld)).toEqual([]);
  });
});
