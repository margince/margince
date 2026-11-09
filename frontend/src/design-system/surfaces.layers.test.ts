import { describe, expect, it } from "vitest";
import { OVERLAY_RUNG, PAGE_RUNGS } from "../../scripts/lib/surface-ladder";
import { OPAQUE } from "../../scripts/lib/surface-sheets";
import { themes } from "./tokens-testing";

// A layer stacked under a pane shows through it unless the ground it paints is
// opaque. census/rules/surfaces.layers.census.ts holds the hosts.

describe("a layer under a pane stays under it", () => {
  it("gives every rung an opaque ground in both themes", () => {
    for (const rung of [...PAGE_RUNGS, OVERLAY_RUNG]) {
      for (const [theme, palette] of Object.entries(themes)) {
        const name = rung.ground.match(/^var\((--[\w-]+)\)$/)?.[1] ?? "";
        expect(palette[name], `${rung.ground} in ${theme}`).toMatch(OPAQUE);
      }
    }
  });
});
