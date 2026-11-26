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

type Reading = NonNullable<NonNullable<Company360["state_strip"]>["dormancy"]>;

function reading(over: Partial<Reading>): Company360 {
  return view({
    state_strip: {
      account: { status: "customer", relationship_types: [] },
      dormancy: {
        rung: "active",
        silent_days: 0,
        no_activity: false,
        ladder_days: [30, 60, 90],
        ...over,
      },
    },
  });
}

describe("an unrated account's verdict", () => {
  it("is active and names the last activity", () => {
    expect(
      companyActivityVerdict(
        reading({ last_activity_at: "2026-10-20T10:00:00Z", silent_days: 8 }),
        words,
      ),
    ).toEqual({
      word: "co.verdict.active",
      sentence: t("co.verdict.lastExchange", { date: "20 Oct" }),
      tone: "success",
    });
  });

  it("names each colder rung with the silent days and the ladder behind it", () => {
    for (const [rung, word, tone] of [
      ["cooling", "co.verdict.cooling", "warning"],
      ["dormant", "co.verdict.dormant", "warning"],
      ["stale", "co.verdict.stale", "neutral"],
    ] as const) {
      expect(
        companyActivityVerdict(
          reading({
            rung,
            last_activity_at: "2026-08-20T10:00:00Z",
            silent_days: 69,
            ladder_days: [14, 45, 120],
          }),
          words,
        ),
      ).toEqual({
        word,
        sentence: t("co.verdict.silentFor", {
          days: "69",
          date: "20 Aug",
          cooling: "14",
          dormant: "45",
          stale: "120",
        }),
        tone,
      });
    }
  });

  it("says no activity is recorded when there is none", () => {
    expect(
      companyActivityVerdict(
        reading({ rung: "stale", no_activity: true }),
        words,
      ),
    ).toEqual({
      word: "co.verdict.stale",
      sentence: t("co.verdict.noActivity", { date: "5 Jan" }),
      tone: "neutral",
    });
  });

  it("makes no call when the page carries no reading", () => {
    expect(companyActivityVerdict(view(), words)).toBeUndefined();
  });
});
