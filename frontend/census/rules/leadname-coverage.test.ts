// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { fallbacksIn, rule } from "./leadname-coverage.census";

describe("what a lead is called", () => {
  const planted: ReadonlyArray<readonly [string, string, number]> = [
    [
      "the plain fallback",
      'const name = lead.full_name ?? lead.email ?? "";',
      1,
    ],
    [
      "the same fallback ending at the id",
      "const name = lead.full_name ?? lead.email ?? lead.id;",
      1,
    ],
    [
      "a company-first label, which still names the lead at its second term",
      'const label = lead.company_name ?? lead.full_name ?? lead.email ?? "";',
      1,
    ],
    [
      "the rule inside JSX, where it is read rather than assigned",
      "const row = <strong>{lead.full_name ?? lead.email}</strong>;",
      1,
    ],
    [
      "the same rule spelled with ||",
      'const name = lead.full_name || lead.email || "";',
      1,
    ],
    [
      "a bracketed read, which a property-access-only rule would miss",
      'const name = lead["full_name"] ?? lead["email"];',
      1,
    ],
    [
      "a form field seeded from the stored name, where empty is the right answer",
      'const value = lead.full_name ?? "";',
      0,
    ],
    [
      "a contact, who is not a lead and has no address to fall back to",
      "const name = data.full_name ?? null;",
      0,
    ],
    [
      "an address-first label, which is a different rule and means it",
      "const name = lead.email ?? lead.full_name;",
      0,
    ],
    [
      "two different receivers, which is not one record being named",
      "const name = contact.full_name ?? lead.email;",
      0,
    ],
    [
      "a bracketed tail, where the address sits inside its own chain",
      'const name = lead.full_name ?? (lead.email || "");',
      1,
    ],
    [
      "another record's name ahead of the lead's own",
      "const name = contact.full_name ?? lead.full_name ?? lead.email;",
      1,
    ],
  ];

  for (const [what, code, count] of planted) {
    it(`sees ${what}`, () => {
      expect(fallbacksIn("planted.tsx", code)).toHaveLength(count);
    });
  }

  // Parsed as TSX, a `.ts` generic arrow opens a JSX element that never
  // closes, and the fallback after it is lost to parse recovery.
  it("reads a .ts generic arrow as TypeScript, so what follows it is still scanned", () => {
    const source =
      "const id = <T>(value: T) => value;\nconst name = lead.full_name ?? lead.email;";
    expect(fallbacksIn("planted.ts", source)).toHaveLength(1);
  });
});

describe("the leadname-coverage rule", () => {
  it("reports a screen and passes the owner", () => {
    const fallback = "export const n = lead.full_name ?? lead.email;\n";
    expect(
      plantedFindings(rule, [
        {
          path: join(srcDir, "features", "leads", "planted.tsx"),
          area: "src",
          text: fallback,
        },
        {
          path: join(srcDir, "format", "leadname.ts"),
          area: "src",
          text: fallback,
        },
      ]),
    ).toEqual(["src/features/leads/planted.tsx:1"]);
  });
});
