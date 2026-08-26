import { describe, expect, it } from "vitest";
import { entryFieldChanges } from "./history.logic";

describe("entryFieldChanges", () => {
  it("reads what moved out of the images the entry already carries", () => {
    expect(
      entryFieldChanges({
        before: { name: "Globex", amount_minor: 2500000 },
        after: { name: "Globex Renewal", amount_minor: 2500000 },
      }),
    ).toEqual([
      { field: "name", oldValue: "Globex", newValue: "Globex Renewal" },
    ]);
  });

  // A field that did not exist before, and one that does not now, are the two
  // states the diff has its own wording for — so they must not arrive as empty
  // text, which is a value somebody stored.
  it("keeps an absent value absent in both directions", () => {
    expect(
      entryFieldChanges({ before: null, after: { lost_reason: "budget" } }),
    ).toEqual([{ field: "lost_reason", oldValue: null, newValue: "budget" }]);
    expect(
      entryFieldChanges({ before: { lost_reason: "budget" }, after: {} }),
    ).toEqual([{ field: "lost_reason", oldValue: "budget", newValue: null }]);
  });

  it("spells a stored document rather than printing an object", () => {
    const [change] = entryFieldChanges({
      before: {},
      after: { address: { city: "Berlin" } },
    });
    expect(change.newValue).toBe('{"city":"Berlin"}');
  });

  it("has nothing to show for an entry that carries no images", () => {
    expect(entryFieldChanges({ before: null, after: null })).toEqual([]);
  });
});
