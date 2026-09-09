import { describe, expect, it } from "vitest";
import { RECORDS } from "../../e2e/records";
import { GRIDDED_RECORD_SCREENS } from "./nav";

/**
 * The record sweep measures every record page, and this is what says so.
 *
 * `e2e/records.ts` is the corpus the browser sweeps — the record chrome and the
 * tab strip are shell invariants, so both are measured on every record page
 * rather than on the one a defect was reported from. A hand-kept corpus can
 * only fail SHORT: it sweeps five pages of six, every test passes, and nothing
 * in the run is shaped like a failure.
 *
 * `GRIDDED_RECORD_SCREENS` is the set the shell reads on every navigation to
 * decide whether a route gets the record column, so a new record page cannot
 * ship without joining it. That makes it the one list the corpus can be checked
 * AGAINST rather than merely kept beside.
 *
 * Here rather than in the specs because `nav.ts` reaches `custom.ts` and its
 * `import.meta.glob`, which Playwright's transform cannot compile; vitest runs
 * through Vite and can.
 */
describe("the record sweep's census", () => {
  const swept = new Set(RECORDS.map((record) => record.screen));

  it("gives every screen the shell caps as a record a route to sweep", () => {
    expect(
      [...GRIDDED_RECORD_SCREENS].filter((screen) => !swept.has(screen)),
      "a screen whose record page takes the record column has no route in e2e/records.ts, so its record shell is measured by nothing",
    ).toEqual([]);
  });

  // The other direction cannot fail quietly — a route for a screen with no
  // record page fails in the browser, loudly — but it is what keeps the corpus
  // readable as the same list the shell works from.
  it("sweeps no page the shell does not treat as a record", () => {
    expect(
      [...swept].filter((screen) => !GRIDDED_RECORD_SCREENS.has(screen)),
      "e2e/records.ts sweeps a screen the shell gives no record column",
    ).toEqual([]);
  });

  it("names each record page once", () => {
    expect(swept.size, "two routes claim the same screen").toBe(RECORDS.length);
  });
});
