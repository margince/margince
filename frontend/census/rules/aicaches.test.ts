// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource, resolveRelative } from "../../scripts/lib/source-tree";
import { LANGUAGE_DEPENDENT_QUERY_PREFIXES } from "../../src/features/sign-in/aicaches";
import { plantedFindings, srcDir } from "../tree";
import { queryPrefixes, rule } from "./aicaches.census";

describe("query-prefix census syntax", () => {
  it("resolves literal, imported alias, local alias and spread tuple keys from their owner", () => {
    const files = new Map([
      [
        "/census/keys.ts",
        parseSource(
          "/census/keys.ts",
          `export const briefKey = ["brief"] as const; export const unused = ["unusedBrief"] as const;`,
        ),
      ],
    ]);
    const source = parseSource(
      "/census/screen.ts",
      `
      import { briefKey as imported } from "./keys";
      const local = imported;
      useQuery({ queryKey: ["literalBrief", id] });
      useQuery({ queryKey: imported });
      useQuery({ queryKey: local });
      useQuery({ queryKey: [...local, id] });
      const unused = ["inventedBrief"] as const;
      function shadow(imported) { useQuery({ queryKey: imported }); }
      useQuery({ queryKey: missing });
    `,
    );
    const read = (path: string) => files.get(path);
    expect(
      queryPrefixes(source, read, (from, specifier) =>
        resolveRelative(from, specifier, (path) => files.has(path)),
      ),
    ).toEqual(["literalBrief", "brief", "brief", "brief"]);
  });
});

it("does not borrow an outer tuple through destructuring, loop or catch shadows", () => {
  const source = parseSource(
    "/census/shadows.ts",
    `
    const key = ["outerBrief"] as const;
    function parameter({ key }) { useQuery({ queryKey: key }); }
    function nested({ value: { key } }) { useQuery({ queryKey: key }); }
    function array([key]) { useQuery({ queryKey: key }); }
    { const { key } = options; useQuery({ queryKey: key }); }
    { const [key] = options; useQuery({ queryKey: key }); }
    for (const key of keys) { useQuery({ queryKey: key }); }
    for (const key in keys) { useQuery({ queryKey: key }); }
    for (let key = start; key; key = next) { useQuery({ queryKey: key }); }
    try {} catch (key) { useQuery({ queryKey: key }); }
    try {} catch ({ key }) { useQuery({ queryKey: key }); }
    useQuery({ queryKey: key });
  `,
  );
  expect(
    queryPrefixes(
      source,
      () => undefined,
      () => null,
    ),
  ).toEqual(["outerBrief"]);
});

it("follows the actual export alias rather than a same-named private constant", () => {
  const owner = parseSource(
    "/census/exports.ts",
    `
    const briefKey = ["privateBrief"] as const;
    const privateOnly = ["hiddenBrief"] as const;
    const actual = ["publicBrief"] as const;
    export { actual as briefKey };
  `,
  );
  const source = parseSource(
    "/census/consumer.ts",
    `
    import { briefKey as key, privateOnly } from "./exports";
    useQuery({ queryKey: key });
    useQuery({ queryKey: privateOnly });
  `,
  );
  expect(
    queryPrefixes(
      source,
      () => owner,
      (from, specifier) =>
        resolveRelative(from, specifier, (path) => path === owner.fileName),
    ),
  ).toEqual(["publicBrief"]);
});

describe("the aicaches rule", () => {
  it("fails a planted model-written key, read through an imported constant", () => {
    const named = LANGUAGE_DEPENDENT_QUERY_PREFIXES.map((prefix) => ({
      path: join(srcDir, "lib", `named${prefix[0]}.ts`),
      area: "src" as const,
      text: `useQuery({ queryKey: ["${prefix[0]}"] });`,
    }));
    const findings = plantedFindings(rule, [
      ...named,
      {
        path: join(srcDir, "lib", "plantedkeys.ts"),
        area: "src",
        text: 'export const plantedKey = ["plantedBrief"] as const;',
      },
      {
        path: join(srcDir, "features", "planted", "screen.tsx"),
        area: "src",
        text: 'import { plantedKey } from "../../lib/plantedkeys";\nuseQuery({ queryKey: plantedKey });',
      },
      {
        path: join(srcDir, "features", "planted", "screen.test.tsx"),
        area: "src",
        text: 'useQuery({ queryKey: ["testBrief"] });',
      },
    ]);
    expect(findings).toEqual([
      "src/features/planted/screen.tsx: plantedBrief looks model-written and is not in LANGUAGE_DEPENDENT_QUERY_PREFIXES",
    ]);
  });

  it("fails a listed prefix no screen queries", () => {
    const findings = plantedFindings(rule, []);
    expect(findings).toHaveLength(LANGUAGE_DEPENDENT_QUERY_PREFIXES.length);
  });
});
