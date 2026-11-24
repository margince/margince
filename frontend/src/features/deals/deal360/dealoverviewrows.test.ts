// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../../../api/schema";
import { formatDayMonth, formatMoney } from "../../../format/format";
import { UTC_ZONE } from "../../../format/timezone";
import { translate } from "../../../i18n";
import { dealRoleLabel } from "../../../lib/recordlabels";
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
    offers_email: false,
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
    expect(row.statusLabel).toBe(t("commercial.offer.superseded"));
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
  it("names the seat a gap misses and leaves other findings out", () => {
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
      t("deal.committee.missing.firstContact"),
    ]);
  });

  // An older server sends no threshold and judged at a floor of two.
  it("asks for a second contact when the server names no threshold", () => {
    const { gaps } = committeeRows(
      {
        deal_id: "d1",
        stakeholders: [{ contact_id: "c1", role: "user", engaged: true }],
        our_side: [],
        risks: [{ kind: "single_threaded_theirs", summary: "one thread" }],
        sections_omitted: [],
      },
      t,
      (role) => role,
    );
    expect(gaps.map((gap) => gap.title)).toEqual([
      t("deal.committee.missing.secondContact"),
    ]);
  });

  it("draws one row per contact, naming every role they hold", () => {
    const { seats } = committeeRows(
      {
        deal_id: "d1",
        stakeholders: [
          {
            contact_id: "c1",
            contact_name: "Zoe",
            role: "economic_buyer",
            engaged: false,
          },
          {
            contact_id: "c1",
            contact_name: "Zoe",
            role: "champion",
            engaged: false,
          },
          {
            contact_id: "c2",
            contact_name: "Bob",
            role: "user",
            engaged: false,
          },
        ],
        our_side: [],
        risks: [],
        sections_omitted: [],
      },
      t,
      (role) => role,
    );
    expect(seats.map((seat) => seat.key)).toEqual(["c1", "c2"]);
    expect(seats[0]?.sub).toContain("economic_buyer, champion");
  });

  // A typed role has no label, so the seat reads as one with no role.
  it("leaves a typed role out of a seat's words", () => {
    const seat = (role: string) => ({
      contact_id: "c1",
      contact_name: "Zoe",
      role,
      engaged: false,
    });
    const { seats } = committeeRows(
      {
        deal_id: "d1",
        stakeholders: [seat("contact"), seat(" Champion ")],
        our_side: [],
        risks: [],
        sections_omitted: [],
      },
      t,
      (role) => dealRoleLabel(role, t),
    );
    expect(seats[0]?.sub).toContain(t("co.roleLabel.champion"));
    expect(seats[0]?.sub).not.toMatch(/contact|, $|^, /);
    expect(seats[0]?.sub).not.toContain(`, ${t("co.roleLabel.champion")}`);
  });
});
