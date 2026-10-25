// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../../../api/schema";
import { formatDayMonth, formatMoney } from "../../../format/format";
import { UTC_ZONE } from "../../../format/timezone";
import { translate } from "../../../i18n";
import { offer } from "../deals.testkit";
import { committeeRows, offerRow, otherOpenTaskRows } from "./dealoverviewrows";

type Activity = components["schemas"]["Activity"];

const t = (
  key: Parameters<typeof translate>[1],
  params?: Record<string, string>,
) => translate("en", key, params);

function task(id: string, over: Partial<Activity> = {}): Activity {
  return {
    occurred_precision: "instant",
    id,
    kind: "task",
    occurred_at: "2026-10-01T09:00:00Z",
    is_done: false,
    source: "manual",
    captured_by: "human:u1",
    created_at: "2026-10-01T09:00:00Z",
    updated_at: "2026-10-01T09:00:00Z",
    ...over,
  };
}

describe("an offer as a card row", () => {
  it("dates it by when it was written and names its status in words", () => {
    const row = offerRow(
      offer({ revision: 2, status: "superseded" }),
      t,
      "en",
      UTC_ZONE,
    );
    expect(row.title).toBe(
      t("deal.offers.rowTitle", { number: "OFF-0001", revision: "2" }),
    );
    expect(row.sub).toBe(
      t("deal.offers.rowSub", {
        date: formatDayMonth("2026-06-01T00:00:00Z", "en", UTC_ZONE),
        net: formatMoney(100_000, "EUR", "en"),
      }),
    );
    expect(row.status).toBe(t("commercial.offer.superseded"));
  });
});

describe("the other open tasks the Next bar counts", () => {
  it("counts open tasks other than the one the bar names", () => {
    const rows = [
      task("shown"),
      task("other"),
      task("done", { is_done: true }),
      task("gone", { archived_at: "2026-10-02T00:00:00Z" }),
      { ...task("call"), kind: "call" as const },
    ];
    expect(otherOpenTaskRows(rows, ["shown"])).toHaveLength(1);
  });
});

describe("the committee's rows", () => {
  it("names a gap by its rule and leaves other findings out", () => {
    const { seats, gaps } = committeeRows(
      {
        deal_id: "d1",
        stakeholders: [],
        our_side: [],
        risks: [
          { kind: "single_threaded_theirs", summary: "one thread" },
          { kind: "going_cold", summary: "quiet" },
        ],
        sections_omitted: [],
      },
      t,
      (role) => role,
    );
    expect(seats).toEqual([]);
    expect(gaps.map((gap) => gap.title)).toEqual([
      t("coverage.risk.single_threaded_theirs"),
    ]);
  });
});
