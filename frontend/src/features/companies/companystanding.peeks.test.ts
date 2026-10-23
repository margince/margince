import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { UTC_ZONE } from "../../format/timezone";
import { en } from "../../i18n/en";
import { contactsInTouch, openDeals } from "./companystanding.peeks";

type Company360 = components["schemas"]["Company360"];

const fill = (text: string, params?: Record<string, string>) =>
  Object.entries(params ?? {}).reduce(
    (out, [name, value]) => out.replace(`{${name}}`, value),
    text,
  );
const words = {
  t: (key: keyof typeof en, params?: Record<string, string>) =>
    fill(en[key], params),
  plural: (base: string, count: number, params?: Record<string, string>) =>
    fill(
      en[`${base}_${count === 1 ? "one" : "other"}` as keyof typeof en],
      params,
    ),
  locale: "en",
  zone: UTC_ZONE,
} as never;

const view = (over: object): Company360 =>
  ({
    sections_omitted: [],
    as_of: "2026-10-22T09:00:00Z",
    ...over,
  }) as unknown as Company360; // contract:cast only the fields this peek reads are filled

describe("contactsInTouch", () => {
  const contact = (id: string, last: string | null) => ({
    contact_id: id,
    full_name: `Name ${id}`,
    strength: { last_interaction: last },
  });

  it("lists the contacts the server counts as in touch, newest first, and not the rest", () => {
    const peek = contactsInTouch(
      view({
        contacts: {
          data: [
            contact("a", "2026-07-28T00:00:00Z"),
            contact("b", null),
            contact("c", "2026-10-12T00:00:00Z"),
          ],
        },
        health: { active_contacts: 2 },
      }),
      words,
    );

    expect(peek?.items.map((item) => item.key)).toEqual(["c", "a"]);
    expect(peek?.foot).toBeUndefined();
  });

  it("says so when the page holds fewer rows than the count", () => {
    const peek = contactsInTouch(
      view({
        contacts: { data: [contact("a", "2026-07-28T00:00:00Z")] },
        health: { active_contacts: 3 },
      }),
      words,
    );

    expect(peek?.foot).toBe("Showing 1 of 3.");
  });

  it("offers nothing for a section the reader may not see", () => {
    expect(
      contactsInTouch(view({ sections_omitted: ["contacts"] }), words),
    ).toBeUndefined();
  });
});

describe("openDeals", () => {
  it("lists only open deals, naming a deal nobody priced", () => {
    const peek = openDeals(
      view({
        deals: {
          data: [
            {
              deal_id: "1",
              name: "Expansion",
              status: "open",
              stage_name: "Proposal",
              stalled: false,
              amount: { amount_minor: 9_500_000, currency: "EUR" },
            },
            {
              deal_id: "2",
              name: "Depot",
              status: "open",
              stage_name: "Discovery",
              stalled: true,
            },
            { deal_id: "3", name: "Done", status: "won", stalled: false },
          ],
        },
        state_strip: { commercial: { open_count: 2 } },
      }),
      words,
    );

    expect(peek?.items.map((item) => item.title)).toEqual([
      "Expansion",
      "Depot",
    ]);
    expect(peek?.items[1]?.right).toBe(en["co.strip.unpriced"]);
    expect(peek?.items[1]?.sub).toContain("Stalled");
    expect(peek?.foot).toBeUndefined();
  });
});
