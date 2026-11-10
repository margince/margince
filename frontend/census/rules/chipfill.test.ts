// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { landsOn, overriddenInside } from "../../scripts/lib/rendered-inside";
import { plantedFindings, srcDir } from "../tree";
import { chainsNaming, rule } from "./chipfill.census";

describe("the chip fill's readers", () => {
  it("reaches a rule through any one of its :is() alternatives", () => {
    const remove = new Set(["token-remove"]);
    const probe = {
      file: "probe.css",
      selector: ":is(.token-remove, .pill-remove)",
      body: "",
    };
    expect(landsOn(probe, remove, [remove])).toBe(true);
    expect(chainsNaming(":is(.token, .pill) .x", new Set(["token"]))).toEqual([
      [".token", ".x"],
    ]);
  });

  it("lets a rule override an ink only where it lands on that element", () => {
    const chip = {
      file: "probe.css",
      selector: ".tok",
      body: "background: var(--bgChip);",
    };
    const drawn = {
      file: "probe.css",
      selector: ".tok-x",
      body: "color: var(--textSecondary);",
    };
    const within = new Map([["tok", [new Set(["tok-x"])]]]);
    const overrides = (selector: string) =>
      overriddenInside(
        [
          chip,
          drawn,
          { file: "probe.css", selector, body: "color: var(--textPrimary);" },
        ],
        chip,
        drawn,
        within,
      );
    expect(overrides(".tok :is(.q, .r)")).toBe(false);
    expect(overrides(".tok :is(.tok-x, .r)")).toBe(true);
    expect(overrides(".tok .q")).toBe(false);
    expect(overrides(".tok span")).toBe(false);
    expect(overrides(".tok *")).toBe(true);
  });
});

describe("the chipfill rule", () => {
  const planted = (css: string, tsx: string) =>
    plantedFindings(rule, [
      { path: join(srcDir, "features", "p.css"), area: "src", text: css },
      { path: join(srcDir, "features", "p.tsx"), area: "src", text: tsx },
    ]);

  it("reports an unmeasured ink inside a chip, by the components", () => {
    expect(
      planted(
        ".tok { background: var(--bgChip); color: var(--textPrimary); }\n.tok-x { color: var(--textTertiary); }\n",
        'export const F = () => <span className="tok"><b className="tok-x">x</b></span>;\n',
      ),
    ).toEqual([
      "src/features/p.css: .tok-x draws on .tok's --bgChip in --textTertiary, which no pair measures",
    ]);
  });

  it("reports a chip that names no ink, and a chip inside a chip", () => {
    expect(
      planted(
        ".tok { background: var(--bgChip); }\n.tok .inner { background: var(--bgChip); color: var(--textPrimary); }\n",
        'export const F = () => <span className="tok">x</span>;\n',
      ),
    ).toEqual([
      "src/features/p.css: .tok paints --bgChip under [tok], which names no ink and reads in whatever its row is drawn in",
      "src/features/p.css: .tok .inner paints --bgChip inside .tok, which already does",
    ]);
  });
});
