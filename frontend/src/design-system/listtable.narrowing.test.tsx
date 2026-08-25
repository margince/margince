import { describe, expect, it } from "vitest";
import { narrowingSignature } from "./listtable";

/**
 * Narrowing a list changes what page one means, so the table sends the reader
 * back to it. What counts as narrowing is this signature, and it is compared
 * rather than counted: an effect runs on arrival, twice on arrival under
 * StrictMode, and again for any dial that settles a tick after mount — three
 * arrivals a run-counter reads as a reader turning something, each of which
 * threw a reader off the page their own address had asked for.
 */
const DIALS = {
  search: "acme",
  chosen: { owner_id: "u-1", stalled: "true" },
  perPage: 25,
  sort: "-created_at",
  archived: false,
  scopeKey: "",
} as const;

describe("what the table calls a narrowing", () => {
  it("is the same narrowing when nothing has moved", () => {
    expect(narrowingSignature(DIALS)).toBe(narrowingSignature({ ...DIALS }));
  });

  it("is the same narrowing when the filters arrive in another order", () => {
    // Two objects holding the same filters differ only in insertion order, and
    // a signature that called that a change would send the reader to page one
    // for nothing — every render that rebuilds the object being one.
    expect(
      narrowingSignature({
        ...DIALS,
        chosen: { stalled: "true", owner_id: "u-1" },
      }),
    ).toBe(narrowingSignature(DIALS));
  });

  it("hears a filter the reader has changed", () => {
    expect(
      narrowingSignature({ ...DIALS, chosen: { owner_id: "u-2" } }),
    ).not.toBe(narrowingSignature(DIALS));
  });

  it("hears a re-ordering, because page two of one order is other rows", () => {
    expect(narrowingSignature({ ...DIALS, sort: "full_name" })).not.toBe(
      narrowingSignature(DIALS),
    );
  });

  it("hears the search box, the page size, the scope and the archived toggle", () => {
    for (const moved of [
      { search: "" },
      { perPage: 50 },
      { scopeKey: "mine" },
      { archived: true },
    ]) {
      expect(narrowingSignature({ ...DIALS, ...moved })).not.toBe(
        narrowingSignature(DIALS),
      );
    }
  });

  it("takes a caller's own key over the dials when it has one", () => {
    // A screen whose filters are not all declared up front says what its
    // narrowing is; `chosen` is then what the CHIPS show and not the answer.
    expect(narrowingSignature({ ...DIALS, narrowKey: "k1", chosen: {} })).toBe(
      narrowingSignature({ ...DIALS, narrowKey: "k1", chosen: { a: "b" } }),
    );
  });

  it("distinguishes two of those keys", () => {
    expect(narrowingSignature({ ...DIALS, narrowKey: "k1" })).not.toBe(
      narrowingSignature({ ...DIALS, narrowKey: "k2" }),
    );
  });
});
