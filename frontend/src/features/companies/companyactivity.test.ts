// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { UTC_ZONE } from "../../format/timezone";
import { translate } from "../../i18n";
import { companyActivityVerdict } from "./companyactivity";

type Company360 = components["schemas"]["Company360"];

const t = (
  key: Parameters<typeof translate>[1],
  values?: Record<string, string>,
) => translate("en", key, values);
const words = { t, locale: "en", zone: UTC_ZONE } as const;

function view(over: Partial<Company360> = {}): Company360 {
  return {
    as_of: "2026-10-28T09:00:00Z",
    company: {
      id: "o-1",
      display_name: "Brandt Automotive GmbH",
      source: "manual",
      captured_by: "human:u1",
      created_at: "2026-01-05T08:00:00Z",
      updated_at: "2026-01-05T08:00:00Z",
    },
    sections_omitted: [],
    deals: {
      data: [],
      page: { has_more: false },
      won_lifetime: { amount_minor: 0, currency: "EUR" },
      lost_count: 0,
    },
    ...over,
  };
}

const lastContact = (at: string): Company360["last_contact"] => ({
  at,
  occurred_precision: "instant",
  kind: "email",
  activity_id: "a-1",
});

describe("an unrated account's verdict", () => {
  it("is active after an exchange this month", () => {
    expect(
      companyActivityVerdict(
        view({ last_contact: lastContact("2026-10-20T10:00:00Z") }),
        words,
      ),
    ).toEqual({
      word: "co.verdict.active",
      sentence: t("co.verdict.lastExchange", { date: "20 Oct" }),
      tone: "success",
    });
  });

  it("is going quiet after two silent months", () => {
    expect(
      companyActivityVerdict(
        view({ last_contact: lastContact("2026-08-20T10:00:00Z") }),
        words,
      ),
    ).toMatchObject({ word: "co.verdict.goingQuiet", tone: "warning" });
  });

  it("is dormant after half a year with nothing open", () => {
    expect(
      companyActivityVerdict(
        view({ last_contact: lastContact("2026-03-02T10:00:00Z") }),
        words,
      ),
    ).toMatchObject({ word: "co.verdict.dormant", tone: "neutral" });
  });

  it("is never dormant when the deal list was cut", () => {
    const cut = view({ last_contact: lastContact("2026-03-02T10:00:00Z") });
    const deals = cut.deals && { ...cut.deals, page: { has_more: true } };
    expect(companyActivityVerdict({ ...cut, deals }, words)).toMatchObject({
      word: "co.verdict.goingQuiet",
    });
  });

  it("says nobody was contacted when no exchange is recorded", () => {
    expect(companyActivityVerdict(view(), words)).toMatchObject({
      word: "co.verdict.noContact",
      tone: "neutral",
    });
  });

  it("makes no call when the last exchange is withheld", () => {
    expect(
      companyActivityVerdict(view({ sections_omitted: ["last_touch"] }), words),
    ).toBeUndefined();
  });
});
