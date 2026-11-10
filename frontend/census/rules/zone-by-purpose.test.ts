// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { FALLBACK_COMPANY_ZONE } from "../../src/format/timezone";
import { plantedFindings, srcDir } from "../tree";
import { code, rule, ZONE_LITERAL } from "./zone-by-purpose.census";

describe("a zone is named in one module", () => {
  it("sees the record zone it exists to protect", () => {
    // The pattern is derived from the IANA shape, so a fallback it cannot see
    // must fail here.
    expect(`"${FALLBACK_COMPANY_ZONE}"`.match(ZONE_LITERAL)).toEqual([
      `"${FALLBACK_COMPANY_ZONE}"`,
    ]);
    // The fallback renders when the installation's zone cannot, so Intl must
    // accept it.
    expect(
      () => new Intl.DateTimeFormat("en", { timeZone: FALLBACK_COMPANY_ZONE }),
    ).not.toThrow();
  });

  it("sees a zone that is not the record zone", () => {
    for (const zone of [
      '"UTC"',
      "'America/Los_Angeles'",
      '"Asia/Ho_Chi_Minh"',
    ]) {
      expect(zone.match(ZONE_LITERAL)).toEqual([zone]);
    }
    // A module path is the shape's near neighbour and is not a zone.
    for (const path of [
      '"screens/settings"',
      '"../format/timezone"',
      '"/v1/me"',
    ]) {
      expect(path.match(ZONE_LITERAL)).toBeNull();
    }
  });

  it("reads the code a comment sits beside, and the code beneath one", () => {
    const source = [
      'const glob = "/dist/mcp-apps/*.html";',
      "// the api client carves out /v1/public/* for this",
      'const hidden = "Europe/Berlin";',
      "/** A doc comment naming Europe/Berlin in prose. */",
      'const url = "https://example.test/x"; // trailing Europe/Berlin prose',
      'const shown = "Asia/Ho_Chi_Minh";',
    ].join("\n");
    const swept = code("sample.ts", source)
      .split("\n")
      .flatMap((line, index) =>
        [...line.matchAll(ZONE_LITERAL)].map(
          (hit) => `${index + 1}: ${hit[0]}`,
        ),
      );
    expect(swept).toEqual([`3: "Europe/Berlin"`, `6: "Asia/Ho_Chi_Minh"`]);
  });
});

describe("the zone-by-purpose rule", () => {
  it("reports a screen's zone and fallback import, and passes the zone module", () => {
    const findings = plantedFindings(rule, [
      {
        path: join(srcDir, "features", "x.tsx"),
        area: "src",
        text: [
          'import { FALLBACK_COMPANY_ZONE } from "../format/timezone";',
          'export const zone = "Europe/Berlin";',
          "export const viewer = Intl.DateTimeFormat().resolvedOptions().timeZone;",
        ].join("\n"),
      },
      {
        path: join(srcDir, "format", "timezone.ts"),
        area: "src",
        text: 'export const FALLBACK_COMPANY_ZONE = "Europe/Berlin";\n',
      },
    ]);
    const own = findings.filter((line) => line.startsWith("features/x.tsx"));
    expect(own).toEqual([
      'features/x.tsx:2: "Europe/Berlin"',
      "features/x.tsx:3: Intl.DateTimeFormat().resolvedOptions()",
      "features/x.tsx:3: resolvedOptions().timeZone",
      "features/x.tsx: names FALLBACK_COMPANY_ZONE",
    ]);
    expect(findings.some((line) => line.startsWith("format/timezone.ts"))).toBe(
      false,
    );
    expect(findings).toContain(
      "lib/worklistheld.test.ts: pinned, but names no zone",
    );
  });
});
