// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../../../api/schema";
import type { StandingVerdict } from "../../../design-system/howitstands";
import { UTC_ZONE } from "../../../format/timezone";
import { translate } from "../../../i18n";
import { deal, stages } from "../deals.testkit";
import type { DealStandingInputs } from "./dealstanding";
import { openDealVerdict } from "./dealverdict";

type Deal = components["schemas"]["Deal"];
type DealStatusCard = components["schemas"]["DealStatusCard"];

const AS_OF = "2026-05-30T09:00:00Z";
const t = (
  key: Parameters<typeof translate>[1],
  values?: Record<string, string>,
) => translate("en", key, values);
const words = { t, locale: "en", zone: UTC_ZONE } as const;
const written: StandingVerdict = {
  word: "On track",
  sentence: "Last activity 2 days ago.",
  tone: "success",
};

function verdictFor(
  over: Partial<Deal>,
  card?: DealStatusCard,
  standing = "live",
): StandingVerdict {
  const inputs: DealStandingInputs = {
    deal: deal({ last_activity_at: "2026-05-28T10:00:00Z", ...over }),
    stages,
    asOf: AS_OF,
    activities: [],
    offers: [],
    coverage: { withheld: false, pending: false },
    card,
    ladder: { advancing: false, advanceRefused: false, onAdvance: () => {} },
  };
  return openDealVerdict(written, standing, inputs, words);
}

describe("an open deal's verdict", () => {
  it("keeps the written call when no fact on the record overrides it", () => {
    expect(verdictFor({})).toBe(written);
  });

  it("says the reader owes the buyer a reply, and for how long", () => {
    const card: DealStatusCard = {
      deal_id: "d1",
      story: { sentences: [] },
      generated_at: AS_OF,
      generated_by: "deterministic",
      reply_to: "a1",
      request_basis: {
        kind: "current_request",
        scope: "deal",
        source_activity_id: "a1",
        source_at: "2026-05-27T08:00:00Z",
        source_precision: "instant",
        source_version: 1,
        evaluated_at: AS_OF,
        policy_version: "test",
      },
    };
    expect(verdictFor({}, card)).toEqual({
      word: t("deal.verdict.yourMove"),
      sentence: "The buyer wrote 3 days ago and is waiting for a reply.",
      tone: "warning",
    });
  });

  it("calls a stalled deal gone quiet since its last activity", () => {
    expect(verdictFor({ stalled: true })).toEqual({
      word: t("deal.verdict.goneQuiet"),
      sentence: t("deal.verdict.goneQuietSince", { date: "28 May" }),
      tone: "danger",
    });
  });

  it("calls a paused deal on hold until the agreed day", () => {
    expect(verdictFor({ wait_until: "2026-06-12T00:00:00Z" })).toMatchObject({
      word: t("deal.verdict.onHold"),
      tone: "neutral",
    });
  });

  it("flags a close date gone by", () => {
    expect(verdictFor({ expected_close_date: "2020-01-15" })).toMatchObject({
      word: t("deal.verdict.overdueToClose"),
      tone: "warning",
    });
  });

  it("calls a deal with nothing filed not started", () => {
    expect(verdictFor({ last_activity_at: null })).toEqual({
      word: t("deal.verdict.notStarted"),
      sentence: t("deal.verdict.nothingFiled"),
      tone: "neutral",
    });
  });

  it("keeps a named blocker over every recorded fact", () => {
    expect(verdictFor({ stalled: true }, undefined, "blocked")).toBe(written);
  });
});
