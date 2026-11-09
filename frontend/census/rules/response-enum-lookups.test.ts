// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir, treeOf } from "../tree";
import {
  contractPath,
  readWaivers,
  responseEnums,
  rule,
  unwaived,
} from "./response-enum-lookups.census";

// The rule, run the way the runner runs it, over modules planted beside the
// real contract types so the checker resolves them.
const planted = (text: string) =>
  plantedFindings(rule, [
    { path: join(srcDir, "features", "planted.tsx"), area: "src", text },
  ]);

const HEAD = `import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";
import { labelFor, valueFor } from "../lib/labelfor";
type Lead = components["schemas"]["Lead"];
const LABEL: Record<Lead["status"], MessageKey> = {
  new: "common.unknownValue",
  contacted: "common.unknownValue",
  engaged: "common.unknownValue",
  promoted: "common.unknownValue",
  disqualified: "common.unknownValue",
};
`;

const cases = ["new", "contacted", "engaged", "promoted", "disqualified"].map(
  (value) => `    case "${value}": return 1;`,
);

describe("the response enums read from crm.yaml", () => {
  it("reads one enum per marker in every layout the contract uses", () => {
    const { sets, markers } = responseEnums(`
a:
  properties:
    inline: { type: string, enum: [x, 'y'], x-extensible-enum: true }
    block:
      type: string
      enum: [p, q]
      x-extensible-enum: true
    listed:
      type: [string, 'null']
      x-extensible-enum: true
      enum:
        - m
        - n
        - null
    wrapped:
      type: string
      enum:
        [r, s,
         t]
      x-extensible-enum: true
    # a comment naming x-extensible-enum: true is no marker
`);
    expect(markers).toBe(4);
    expect(sets.map((set) => [...set])).toEqual([
      ["x", "y"],
      ["p", "q"],
      ["m", "n"],
      ["r", "s", "t"],
    ]);
  });

  it("reads every marker the real contract holds", () => {
    const { sets, markers } = responseEnums(readFileSync(contractPath, "utf8"));
    expect(markers).toBeGreaterThan(500);
    expect(sets).toHaveLength(markers);
    expect(sets.some((set) => set.has("disqualified"))).toBe(true);
  });

  it("refuses a marker with no enum in its mapping", () => {
    expect(() =>
      responseEnums("a:\n  type: string\n  x-extensible-enum: true\n"),
    ).toThrow(/no enum beside it/);
  });
});

describe("the response-enum-lookups rule", () => {
  it("finds an index into a map by a response enum value", () => {
    expect(
      planted(`${HEAD}export const f = (lead: Lead) => LABEL[lead.status];\n`),
    ).toEqual([
      "src/features/planted.tsx:12 `LABEL[lead.status]` reads nothing for a value this build does not know",
    ]);
  });

  it("finds an index by a narrowed response enum value", () => {
    expect(
      planted(
        `${HEAD}export const f = (s: Exclude<Lead["status"], "new">) => LABEL[s];\n`,
      ),
    ).toHaveLength(1);
  });

  it("finds an index by an alias of the contract's field", () => {
    expect(
      planted(
        `${HEAD}type Status = Lead["status"];\nexport const f = (s: Status) => LABEL[s];\n`,
      ),
    ).toHaveLength(1);
  });

  // The app's own vocabulary, such as its locales, takes outside text only
  // through a guard. So a union it names is not the contract's.
  it("passes a union the frontend names itself", () => {
    expect(
      planted(
        `${HEAD}type Mine = "new" | "contacted";\nexport const f = (s: Mine) => LABEL[s];\n`,
      ),
    ).toEqual([]);
  });

  it("finds a switch with a case per value and no default", () => {
    expect(
      planted(
        `${HEAD}export function f(s: Lead["status"]) {\n  switch (s) {\n${cases.join("\n")}\n  }\n}\n`,
      ),
    ).toEqual([
      "src/features/planted.tsx:13 `switch (s)` covers every listed value and has no `default`",
    ]);
  });

  it("passes an exhaustive switch whose cases return and code follows", () => {
    expect(
      planted(
        `${HEAD}export function f(s: Lead["status"]) {\n  switch (s) {\n${cases.join("\n")}\n  }\n  return 0;\n}\n`,
      ),
    ).toEqual([]);
  });

  it("finds an exhaustive switch whose cases only assign", () => {
    const assigning = cases.map((line) =>
      line.replace("return 1", "n = 1; break"),
    );
    expect(
      planted(
        `${HEAD}export function f(s: Lead["status"]) {\n  let n: number;\n  switch (s) {\n${assigning.join("\n")}\n  }\n  return n;\n}\n`,
      ),
    ).toHaveLength(1);
  });

  it("passes a lookup through the helpers, a Partial map and a default", () => {
    expect(
      planted(`${HEAD}const SOME: Partial<Record<Lead["status"], number>> = { new: 1 };
export const a = (lead: Lead) => labelFor(LABEL, lead.status);
export const b = (lead: Lead) => valueFor(LABEL, lead.status, "common.unknownValue");
export const c = (lead: Lead) => SOME[lead.status];
export const c2 = (lead: Lead) => SOME[lead.status] ?? 0;
export const one = (s: "new") => LABEL[s];
export function partly(s: Lead["status"]) {
  let n = 0;
  switch (s) {
    case "new": n = 1; break;
  }
  return n;
}
export const h = (lead: Lead) => LABEL[lead.status] ?? "common.unknownValue";
export const d = LABEL["new"];
export function e(s: Lead["status"]) {
  switch (s) {
    case "new": return 1;
    default: return 2;
  }
}
export function g(s: Lead["status"]) {
  switch (s) {
    case "new": return 1;
  }
  return 2;
}
`),
    ).toEqual([]);
  });

  it("passes a union no response enum holds", () => {
    expect(
      planted(
        `const M = { zzq: 1, zzr: 2 };\nexport const f = (k: "zzq" | "zzr") => M[k];\n`,
      ),
    ).toEqual([]);
  });

  it("leaves tests, stories and the generated contract out of its corpus", () => {
    const tree = treeOf(
      [
        "features/a.tsx",
        "features/a.test.tsx",
        "features/a.stories.tsx",
        "features/a.fixtures.ts",
        "features/a.testkit.tsx",
        "api/schema.d.ts",
        "api/client.ts",
        "testing/kit.ts",
      ].map((rel) => ({
        path: join(srcDir, rel),
        area: "src" as const,
        text: "",
      })),
    );
    expect(rule.corpus(tree).map((file) => file.rel)).toEqual([
      "src/features/a.tsx",
    ]);
  });
});

describe("the waiver list", () => {
  const two = [
    "src/lib/labelfor.ts:1 `M[x]` …",
    "src/lib/labelfor.ts:9 `M[y]` …",
  ];
  const read = new Set(["src/lib/labelfor.ts", "src/b.tsx"]);
  const waivers = readWaivers("# a comment\nsrc/lib/labelfor.ts 2\n");

  it("waives a file holding exactly its count", () => {
    expect(unwaived(two, waivers, read, 2)).toEqual([]);
  });

  it("reports every finding of a file past its count", () => {
    const three = [...two, "src/lib/labelfor.ts:12 `M[z]` …"];
    expect(unwaived(three, waivers, read, 2)).toEqual(three);
  });

  it("asks a fixed file to lower its count and the pin", () => {
    expect(unwaived(two.slice(1), waivers, read, 2)).toEqual([
      "src/lib/labelfor.ts: lower its waiver count to 1",
    ]);
    expect(unwaived([], waivers, read, 2)).toEqual([
      "src/lib/labelfor.ts: delete its waiver line, it has no finding left",
    ]);
    expect(unwaived(two, waivers, read, 3)).toEqual([
      "census/response-enum-lookups-waivers.txt waives 2 lookups; lower WAIVED_LOOKUPS in response-enum-lookups.census.ts to 2",
    ]);
  });

  it("refuses a list that grew past its pin", () => {
    expect(unwaived(two, waivers, read, 1)).toEqual([
      "census/response-enum-lookups-waivers.txt waives 2 lookups and the pin is 1; the list only shrinks, so fix the lookup instead",
    ]);
  });

  it("finds a waiver line for a file that is gone", () => {
    expect(unwaived([], readWaivers("src/gone.tsx 1\n"), new Set(), 1)).toEqual(
      ["src/gone.tsx: delete its waiver line, the file is gone"],
    );
  });
});
