// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { checkedProgram } from "../../scripts/lib/source-tree";
import { frontendRoot, plantedFindings, srcDir } from "../tree";
import {
  browserZoneSites,
  companyZoneSites,
  rule,
  zoneSourceRefs,
} from "./zone-by-purpose.readers.census";

// The planted files exist only in memory, so a relay one imports resolves.
const refsIn = (roots: string[], overlay: Map<string, string>) =>
  zoneSourceRefs(
    checkedProgram(join(frontendRoot, "tsconfig.app.json"), roots, overlay)
      .program,
    roots,
  );

describe("a signed-in screen reads the reader's clock unless it says why", () => {
  it("catches a renamed, a namespaced and an uncalled zone read in an unlisted file", () => {
    const planted = join(srcDir, "features", "zzplanted.tsx");
    const refs = refsIn(
      [planted],
      new Map([
        [
          planted,
          [
            'import { useState } from "react";',
            'import { browserZone, browserZone as bz } from "../format/timezone";',
            'import * as zones from "../format/timezone";',
            'import { useCompanyZone as companyClock } from "../lib/companyzone";',
            "export function Planted() {",
            "  const [a] = useState(browserZone);",
            "  const b = bz();",
            "  const c = zones.browserZone();",
            "  const d = companyClock();",
            "  const e = { browserZone };",
            '  const f = zones["browserZone"]();',
            '  const key = "browserZone";',
            "  const g = zones[key]();",
            '  const either = Math.random() > 0.5 ? "browserZone" : "nothing";',
            "  const h = zones[either]();",
            "  return [a, b, c, d, e, f, g, h].join();",
            "}",
            "export type Zone = ReturnType<typeof browserZone>;",
          ].join("\n"),
        ],
      ]),
    );
    expect(refs).toEqual([
      { source: "browserZone", file: "features/zzplanted.tsx", line: 6 },
      { source: "browserZone", file: "features/zzplanted.tsx", line: 7 },
      { source: "browserZone", file: "features/zzplanted.tsx", line: 8 },
      { source: "useCompanyZone", file: "features/zzplanted.tsx", line: 9 },
      { source: "browserZone", file: "features/zzplanted.tsx", line: 10 },
      { source: "browserZone", file: "features/zzplanted.tsx", line: 11 },
      { source: "browserZone", file: "features/zzplanted.tsx", line: 13 },
      { source: "browserZone", file: "features/zzplanted.tsx", line: 15 },
    ]);
    const listed = [...companyZoneSites, ...browserZoneSites].map(
      ({ file }) => file,
    );
    expect(listed).not.toContain("features/zzplanted.tsx");
  });

  it("catches a zone read through a name another module re-exports it under", () => {
    const relay = join(srcDir, "features", "zzrelay.ts");
    const planted = join(srcDir, "features", "zzrelayed.tsx");
    const refs = refsIn(
      [planted],
      new Map([
        [relay, 'export { browserZone as clock } from "../format/timezone";'],
        [
          planted,
          [
            'import * as relay from "./zzrelay";',
            'import { clock as tick } from "./zzrelay";',
            "export const zones = [relay.clock(), tick()];",
            "export const { clock: held } = relay;",
          ].join("\n"),
        ],
      ]),
    );
    expect(refs).toEqual([
      { source: "browserZone", file: "features/zzrelayed.tsx", line: 3 },
      { source: "browserZone", file: "features/zzrelayed.tsx", line: 3 },
      { source: "browserZone", file: "features/zzrelayed.tsx", line: 4 },
    ]);
  });
});

describe("the zone-by-purpose.readers rule", () => {
  it("reports an unlisted read and a listed file that reads nothing", () => {
    const findings = plantedFindings(rule, [
      {
        path: join(srcDir, "features", "zzplanted.tsx"),
        area: "src",
        text: [
          'import { browserZone } from "../format/timezone";',
          "export const zone = browserZone();",
        ].join("\n"),
      },
    ]);
    expect(findings).toContain("features/zzplanted.tsx:2: browserZone");
    expect(findings).toContain(
      "lib/viewerzone.tsx: listed, but reads no browserZone",
    );
  });
});
