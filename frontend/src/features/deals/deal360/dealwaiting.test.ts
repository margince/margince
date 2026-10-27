// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../../../api/schema";
import { currentReplyTarget, daysWaiting, waitingSince } from "./dealwaiting";

type Card = components["schemas"]["DealStatusCard"];
type Basis = components["schemas"]["WorkEvidenceBasis"];

const asOf = "2026-08-24T00:30:00Z";
const basis: Basis = {
  kind: "current_request",
  scope: "deal",
  source_activity_id: "request-1",
  source_at: "2026-08-23T23:30:00Z",
  source_precision: "instant",
  source_version: 1,
  evaluated_at: asOf,
  policy_version: "request-test",
};

function card(over: Partial<Card> = {}): Card {
  return {
    deal_id: "deal-1",
    story: { sentences: [] },
    generated_at: asOf,
    generated_by: "deterministic",
    reply_to: "request-1",
    request_basis: basis,
    ...over,
  };
}

describe("current request evidence", () => {
  it("uses the classified source clock even when no timeline page is loaded", () => {
    expect(waitingSince(card())).toEqual({
      at: basis.source_at,
      precision: "instant",
    });
    expect(currentReplyTarget(card())).toBe("request-1");
  });

  it.each([
    card({ request_basis: undefined }),
    card({ request_basis: { ...basis, kind: "request_review" } }),
    card({ request_basis: { ...basis, kind: "future_kind" } }),
    card({ request_basis: { ...basis, scope: "contact" } }),
    card({ request_basis: { ...basis, source_activity_id: "other-request" } }),
    card({ request_basis: { ...basis, source_at: "unknown" } }),
    card({ reply_to: null }),
  ])("keeps absent, unknown and mismatched proof neutral", (candidate) => {
    expect(waitingSince(candidate)).toBeNull();
    expect(currentReplyTarget(candidate)).toBeNull();
  });

  it("counts instant days in the record zone", () => {
    const when = waitingSince(card());
    expect(when).not.toBeNull();
    if (!when) throw new Error("current request missing");
    expect(daysWaiting(when, asOf, "Europe/Berlin")).toBe(0);
    expect(daysWaiting(when, asOf, "UTC")).toBe(1);
  });

  it("keeps a date-only request on its recorded date", () => {
    const when = waitingSince(
      card({
        request_basis: {
          ...basis,
          source_at: "2026-08-23T12:00:00Z",
          source_precision: "day",
        },
      }),
    );
    expect(when).not.toBeNull();
    if (!when) throw new Error("current request missing");
    expect(daysWaiting(when, asOf, "Europe/Berlin")).toBe(1);
  });
});
