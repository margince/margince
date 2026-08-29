import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { byReach, countGaps, missingRolesByDeal, reachOf } from "./coverage";

type Contact = NonNullable<
  components["schemas"]["Organization360"]["people"]
>["data"][number];

const FACTORS = { recency: 0, frequency: 0, reciprocity: 0, direction: 0 };

function contact(over: Partial<Contact> = {}): Contact {
  return {
    person_id: "p1",
    full_name: "Christian Hagemeyer",
    strength: { score: 0, bucket: "none", factors: FACTORS },
    deal_roles: [],
    consent: {},
    ...over,
  };
}

const withTraffic = (out: number, back: number, over: Partial<Contact> = {}) =>
  contact({
    strength: {
      score: back > 0 ? 40 : 2,
      bucket: back > 0 ? "moderate" : "weak",
      factors: FACTORS,
      outbound_90d: out,
      inbound_90d: back,
    },
    ...over,
  });

describe("reachOf", () => {
  it("separates never approached from written to and ignored", () => {
    // The two look identical in a contact list and call for opposite moves:
    // one is a free approach, the other is a decision to follow up again.
    expect(reachOf(withTraffic(0, 0))).toBe("untried");
    expect(reachOf(withTraffic(3, 0))).toBe("silent");
  });

  it("counts one reply as answered, whatever the score", () => {
    expect(reachOf(withTraffic(9, 1))).toBe("answered");
  });
});

describe("byReach", () => {
  it("puts the way in first and the unapproached ahead of the ignored", () => {
    const people = [
      withTraffic(3, 0, { person_id: "silent" }),
      withTraffic(0, 0, { person_id: "untried" }),
      withTraffic(2, 2, { person_id: "answered" }),
    ];
    expect([...people].sort(byReach).map((p) => p.person_id)).toEqual([
      "answered",
      "untried",
      "silent",
    ]);
  });
});

describe("missingRolesByDeal", () => {
  const open = [{ id: "d-open", name: "Renewal" }];

  it("names the committee roles nobody holds on an open deal", () => {
    expect(missingRolesByDeal([contact()], open, false)).toEqual([
      {
        dealId: "d-open",
        dealName: "Renewal",
        missing: ["champion", "economic_buyer"],
      },
    ]);
  });

  it("ignores a role held on a deal that is not open", () => {
    // A champion on a deal that closed last year says nothing about the one
    // running now, and counting them hid the gap on the accounts that have it.
    const held = contact({
      deal_roles: [{ deal_id: "d-closed", role: "champion" }],
    });
    expect(missingRolesByDeal([held], open, false)[0]?.missing).toContain(
      "champion",
    );
  });

  it("says nothing when the role is held on the open deal", () => {
    const held = contact({
      deal_roles: [{ deal_id: "d-open", role: "champion" }],
    });
    expect(missingRolesByDeal([held], open, false)[0]?.missing).toEqual([
      "economic_buyer",
    ]);
  });

  // The defect this shape exists for. Deal A has a champion and no economic
  // buyer; deal B has an economic buyer and no champion. Unioning the roles
  // across the account covered both and reported NO gap — on an account with a
  // gap on every deal it has.
  it("reports each deal's own gap rather than the union across the account", () => {
    const twoDeals = [
      { id: "d-a", name: "Renewal" },
      { id: "d-b", name: "New business" },
    ];
    const contacts = [
      contact({
        person_id: "p-champ",
        deal_roles: [{ deal_id: "d-a", role: "champion" }],
      }),
      contact({
        person_id: "p-buyer",
        deal_roles: [{ deal_id: "d-b", role: "economic_buyer" }],
      }),
    ];

    expect(missingRolesByDeal(contacts, twoDeals, false)).toEqual([
      { dealId: "d-a", dealName: "Renewal", missing: ["economic_buyer"] },
      { dealId: "d-b", dealName: "New business", missing: ["champion"] },
    ]);
  });

  it("leaves out a deal whose committee is complete", () => {
    const twoDeals = [
      { id: "d-a", name: "Renewal" },
      { id: "d-b", name: "New business" },
    ];
    const covered = contact({
      deal_roles: [
        { deal_id: "d-a", role: "champion" },
        { deal_id: "d-a", role: "economic_buyer" },
      ],
    });

    expect(
      missingRolesByDeal([covered], twoDeals, false).map((gap) => gap.dealId),
    ).toEqual(["d-b"]);
  });

  it("makes no claim about a gap from a truncated contact list", () => {
    // The twenty-sixth contact is exactly where the missing champion is.
    expect(missingRolesByDeal([contact()], open, true)).toEqual([]);
  });

  it("makes no claim from an empty contact set the caller could not read", () => {
    // Withheld and truncated arrive here identically — an empty array — so the
    // caller collapses both into `incomplete`. Reporting every role missing
    // from contacts nobody read is the failure this guards.
    expect(missingRolesByDeal([], open, true)).toEqual([]);
  });

  it("reports no gap on an account with no open deal", () => {
    // A committee gap is a statement about a deal. With nothing running there
    // is no committee to be short of.
    expect(missingRolesByDeal([contact()], [], false)).toEqual([]);
  });
});

describe("countGaps", () => {
  // The summary line said "N role gaps" off the number of role TYPES missing
  // account-wide, which is at most two however many deals are short of them.
  it("counts every unfilled pair, not the role types", () => {
    expect(
      countGaps([
        { dealId: "d-a", dealName: "Renewal", missing: ["champion"] },
        {
          dealId: "d-b",
          dealName: "New business",
          missing: ["champion", "economic_buyer"],
        },
      ]),
    ).toBe(3);
  });

  it("counts nothing when no deal is short", () => {
    expect(countGaps([])).toBe(0);
  });
});
