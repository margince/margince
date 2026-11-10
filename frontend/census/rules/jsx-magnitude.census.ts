// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A number reaching a reader is written in some notation, and the reader's own
// is the one `format.ts` gives. This rule refuses a magnitude drawn into JSX
// without a formatter: `<span>{group.count}</span>` reads `1234` beside a
// formatted `1.234` for a German reader. `one-locale` holds the case where a
// formatter is reached with the wrong locale.
//
// The ruling is the function name. `format.ts` carries `identifierNumber` and
// `ordinalNumber` beside `formatNumber`, so outside `format.ts` a raw number in
// a rendered position is a site that skipped the ruling.
//
// The subject is derived in three parts:
// 1. A rendered position comes from the type checker: a JSX child, an
//    attribute declared as text, or a parameter typed `Record<string, string>`,
//    the shape the translator's params take. React's own `SVGAttributes` keep
//    drawing coordinates such as `viewBox` out.
// 2. A locale-blind stringifier is read from TypeScript's lib declarations: a
//    `Number` member returning `string` with no `locales` parameter.
// 3. `String(x)` and a template substitution are named, since no declaration
//    marks the language's own conversions.
//
// Not seen, each a known blind spot:
// - a number converted a statement before the JSX, as in `const label = String(n)`;
// - `n + ""`, `[n].join("")` and `JSON.stringify(n)`;
// - `{count && <Row/>}`, a stray zero whose fix is `count > 0 &&`;
// - a figure the server already rendered into a string field;
// - a prop declared `number`, which is judged where the component renders it.
//
// A position whose type the checker cannot resolve is counted, not skipped. An
// extension screen does not type against the core contract, so only those may
// stay unjudged here; the composed lane judges them.

import { readdirSync } from "node:fs";
import { join } from "node:path";
import ts from "typescript";
import { inWorker } from "../../scripts/lib/in-worker";
import type * as Sweep from "../../scripts/lib/jsx-magnitude.sweep";
import { census } from "../../scripts/lib/jsx-magnitude.sweep";
import { sourceFileAt } from "../../scripts/lib/source-tree";
import {
  type CensusFile,
  type CensusRule,
  type CensusTree,
  frontendRoot,
  srcDir,
} from "../tree";

// The one home for the rulings. The file, not format/: a second formatter
// beside it must not pass for free.
const formatModule = join(srcDir, "format", "format.ts");

/**
 * The `Number` members that turn a magnitude into text without asking whose
 * notation, selected from the lib declarations by signature rather than name.
 * `toLocaleString` carries `locales`, so it falls out.
 */
export function blindStringifiers(): Set<string> {
  const libDir = join(frontendRoot, "node_modules", "typescript", "lib");
  const found = new Set<string>();
  for (const file of readdirSync(libDir)) {
    if (!/^lib\..*\.d\.ts$/.test(file)) {
      continue;
    }
    const parsed = sourceFileAt(join(libDir, file));
    const walk = (node: ts.Node): void => {
      if (ts.isInterfaceDeclaration(node) && node.name.text === "Number") {
        for (const member of node.members) {
          if (!ts.isMethodSignature(member) || member.name === undefined) {
            continue;
          }
          const returnsText =
            member.type !== undefined &&
            member.type.kind === ts.SyntaxKind.StringKeyword;
          const takesLocale = member.parameters.some(
            (parameter) =>
              ts.isIdentifier(parameter.name) &&
              parameter.name.text === "locales",
          );
          if (returnsText && !takesLocale) {
            found.add(member.name.getText(parsed));
          }
        }
      }
      node.forEachChild(walk);
    };
    walk(parsed);
  }
  return found;
}

/**
 * The attribute names that place a mark in a drawing's own coordinate space,
 * read from React's declarations. Own members of `SVGAttributes` only: the
 * inherited ones include `aria-label`, which is text read aloud.
 */
export function svgGeometry(): Set<string> {
  const declarations = join(
    frontendRoot,
    "node_modules",
    "@types",
    "react",
    "index.d.ts",
  );
  const parsed = sourceFileAt(declarations);
  const found = new Set<string>();
  const walk = (node: ts.Node): void => {
    if (ts.isInterfaceDeclaration(node) && node.name.text === "SVGAttributes") {
      for (const member of node.members) {
        if (member.name !== undefined) {
          found.add(
            ts.isStringLiteralLike(member.name)
              ? member.name.text
              : member.name.getText(parsed),
          );
        }
      }
    }
    node.forEachChild(walk);
  };
  walk(parsed);
  return found;
}

const blind = blindStringifiers();
const geometry = svgGeometry();

// Stories are in: a story is a catalog a reader opens. Tests are out, since
// an expectation has to be writable in one notation.
const isScreen = (file: CensusFile) =>
  file.path.endsWith(".tsx") && !file.path.endsWith(".test.tsx");

const inScope = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  isScreen(file) &&
  file.path !== formatModule;

type Verdict = ReturnType<typeof census>;

const SWEEP = new URL(
  "../../scripts/lib/jsx-magnitude.sweep.ts",
  import.meta.url,
);

/** What the verdict says: files the program missed, then the findings. */
function verdictFindings({ unloaded, findings, unjudged }: Verdict): string[] {
  return [
    ...unloaded.map((path) => `${path}: the typed program did not load it`),
    ...findings.map(
      (site) => `${site.file}:${site.line}: a raw magnitude: ${site.text}`,
    ),
    ...unjudged
      .filter((site) => !site.file.startsWith("../extensions/"))
      .map(
        (site) =>
          `${site.file}:${site.line}: the checker could not type: ${site.text}`,
      ),
  ];
}

/**
 * The same verdict in this thread, over the tree's shared program, for a
 * planted tree whose files exist only in memory.
 */
export const judge = (corpus: readonly CensusFile[], tree: CensusTree) =>
  verdictFindings(
    census(
      tree.checked(),
      corpus.map((file) => file.path),
      blind,
      geometry,
    ),
  );

const srcFolder = (file: CensusFile) =>
  file.area === "src" ? file.rel.split("/")[1] : undefined;

function derivationFloors(): string[] {
  const stringifiers = [...blind].sort().join(", ");
  return [
    ...(stringifiers === "toExponential, toFixed, toPrecision, toString"
      ? []
      : [`the stringifiers derived are ${stringifiers}`]),
    ...(geometry.has("viewBox") && geometry.has("d") && geometry.size > 100
      ? []
      : ["the svg geometry was not read from React's declarations"]),
    ...(geometry.has("aria-label") || geometry.has("title")
      ? ["the svg geometry took inherited text attributes"]
      : []),
  ];
}

export const rule: CensusRule = {
  name: "jsx-magnitude",
  corpus: (tree) => tree.files.filter(inScope),
  floors: (corpus, tree) => {
    const swept = new Set(corpus.map(srcFolder));
    const unswept = [
      ...new Set(
        tree.files
          .filter((file) => file.area === "src" && isScreen(file))
          .map(srcFolder),
      ),
    ].filter((name) => !swept.has(name));
    return [
      ...(unswept.length === 0
        ? []
        : [
            `src/ folders with screens the census missed: ${unswept.join(", ")}`,
          ]),
      ...(corpus.some((file) => file.area === "extensions")
        ? []
        : ["the census reached no extension frontend layer"]),
      ...(corpus.some((file) => file.path.endsWith(".stories.tsx"))
        ? []
        : ["the census read no story"]),
      ...derivationFloors(),
    ];
  },
  settle: async (corpus) =>
    verdictFindings(
      await inWorker<typeof Sweep, "sweepFiles">(
        SWEEP,
        "sweepFiles",
        corpus.map((file) => file.path),
        blind,
        geometry,
      ),
    ),
  advice:
    "A number drawn for a reader goes through formatNumber, or through " +
    "identifierNumber or ordinalNumber when it is a code or a rank. Pick the " +
    "one that rules the site; String(n) and toFixed answer in nobody's notation.",
};
