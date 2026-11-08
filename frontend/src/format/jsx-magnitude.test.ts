// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// biome-ignore-all lint/suspicious/noTemplateCurlyInString: these strings are
// FIXTURES of source code, and the ${...} in them is the subject under test —
// a template literal the sweep has to recognize, not one this file meant to
// interpolate.

import { readdirSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import ts from "typescript";
import { describe, expect, it } from "vitest";

import { inWorker } from "../../scripts/lib/in-worker";
import type * as Sweep from "../../scripts/lib/jsx-magnitude.sweep";
import {
  extensionFrontendFiles,
  filesUnder,
  sourceFileAt,
} from "../../scripts/lib/source-tree";

// A number reaching a reader has been written in SOME notation, and the only
// question is whose. `format/one-locale.test.ts` holds the case where a
// formatter is reached the wrong way; this holds the case where none is reached
// at all — a magnitude interpolated straight into JSX:
//
//     <span>{group.count}</span>
//     <StatCard value={String(activity)} />
//
// A German reader sees `1234` where every formatted figure beside it on the
// same page reads `1.234`, so one screen is written in two notations and
// neither of the gates already here can say so. The `translate` narrowing to
// `Record<string, string>` only sees what is handed to `t()`, and these never
// reach `t()`. `one-locale.test.ts` gates how a locale reaches a formatter, and
// these reach no formatter to give one to. A site already wrapped in
// `String(...)` satisfies the narrowing exactly and groups nothing.
//
// THE RULING IS THE FUNCTION NAME, which is what lets this gate be absolute
// rather than a list of blessed sites. Not every number in JSX wants grouping:
// an invoice number is not one thousand two hundred, and `#1.204` is a defect
// where `1.204` was right one line above. So `format.ts` carries
// `identifierNumber` and `ordinalNumber` beside `formatNumber`, every site has
// been ruled by picking one of them, and outside `format.ts` a raw number in a
// rendered position is a site that skipped the ruling rather than a site whose
// ruling was "leave it". `format/collate.ts` did this first, for the same
// reason: `forReader` and `stable` both order, and only the name says which
// order was meant.
//
// THE SUBJECT IS DERIVED, in the three parts it has:
//
//   1. A RENDERED POSITION comes from the type checker rather than from a list
//      of props, and there are three of them. A JSX child. An attribute whose
//      declared type is text, which is how `aria-label`, `title`, `alt` and
//      `StatCard`'s `value` are all in scope without one of them being named
//      here, and how a prop renamed tomorrow stays in scope. And a catalog
//      sentence's parameter — anything whose contextual type is
//      `Record<string, string>`, which is the shape the translator declares its
//      params as, so `t()` is reached through its signature rather than through
//      its name. React's own `SVGAttributes` is what keeps a `viewBox` out of
//      the second: those are coordinates in a space the drawing invented, and
//      grouping one would break the picture.
//   2. A LOCALE-BLIND STRINGIFICATION is read out of TypeScript's own lib
//      declarations: the `Number` members returning `string` whose signature
//      has NO `locales` parameter. That yields `toFixed`, `toPrecision`,
//      `toExponential` and `toString` without this file naming one of them, and
//      it is the same two-source discipline `localeMethods()` uses next door —
//      selecting by name is a hand-maintained fragment of the subject wearing a
//      regular expression's clothes.
//   3. `String(x)` and a template literal's substitution are NAMED, because
//      there is nothing to derive them from: they are the language's own
//      conversions and no declaration marks them as locale-blind, since they
//      were never locale-aware to begin with.
//
// WHAT IT CANNOT SEE, because a census that can fail short has already failed
// and the honest thing is to say where the floor is:
//
//   - A number converted a statement away from the JSX. `const label =
//     String(n)` then `{label}` is a string by the time it reaches a rendered
//     position, and following it back is dataflow rather than a type at a node.
//   - `n + ""`, `[n].join("")`, `JSON.stringify(n)`. Concatenation has no
//     callee to anchor on and the other two are not `Number` members.
//   - `{count && <Row/>}`, whose type carries a `0` beside an element. That
//     renders a stray zero and is a real defect, but it is a different one —
//     the fix is `count > 0 &&`, not a formatter — so a type with a
//     non-numeric constituent is not a magnitude here.
//   - A figure the SERVER already rendered into a string field. Nothing on this
//     side of the wire can tell it from any other string.
//   - A rendered position whose type the checker could not resolve. That is not
//     hypothetical: this sweep reads the extension tier, and a unit's screen
//     does not type against the VANILLA contract by construction — its
//     `/ext/...` responses resolve to `never`, so a magnitude read off one is
//     invisible here and the composed lane is where that half is judged. Left
//     silent this is the whole rule-8 defect: a smaller tree read, and PASS
//     reported about it. So an unjudgeable position is COUNTED rather than
//     skipped, and the arm below holds that every one of them is in the
//     extension tier — a new one under `src/` fails.
//   - A prop declared `number`. The ruling belongs where the component renders
//     it, not where a caller passes it, and that render site is itself in
//     scope — which is how `brief.rail.tsx`'s `DigestCount` is caught once, at
//     the `<span>{value}</span>` that is actually wrong, rather than four times
//     at callers doing nothing wrong.

const here = dirname(fileURLToPath(import.meta.url));
const srcRoot = join(here, "..");
const frontendRoot = resolve(srcRoot, "..");
const extensionsDir = resolve(frontendRoot, "..", "extensions");

// The one home for the rulings, and so the one file allowed to hold a number
// crossing into a string without one. The FILE and not the directory: `format/`
// holds ten other modules, and excluding the directory would let a new
// formatter added beside this one render for nobody and pass.
const formatModule = join(here, "format.ts");
const thisGate = fileURLToPath(import.meta.url);

/**
 * The `Number` members that turn a magnitude into text WITHOUT asking whose
 * notation, read out of TypeScript's own lib declarations.
 *
 * Selected by signature — returns `string`, has no `locales` parameter — for
 * the reason its neighbour in `one-locale.test.ts` gives: a name filter is a
 * hand-maintained fragment of the subject, and it answers a different question
 * from the one asked. `toLocaleString` falls out by carrying `locales`, which
 * is exactly right: it is the locale-AWARE member, and it is that gate's
 * subject rather than this one's.
 */
function blindStringifiers(): Set<string> {
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
 * The attribute names that place a mark in a DRAWING's own coordinate space,
 * read out of React's own declarations.
 *
 * `viewBox` is `string`, so a sparkline's `viewBox={`0 0 ${W} ${H}`}` is a
 * number reaching a string-typed attribute and looks exactly like the defect.
 * It is not one: those are coordinates in a space the drawing invented, in the
 * same class as `size={14}`, and grouping them would break the picture.
 *
 * OWN members only. `SVGAttributes` inherits from `AriaAttributes` and
 * `DOMAttributes`, so taking the inherited set would swallow `aria-label` —
 * text assistive technology reads aloud, and the one attribute on an svg most
 * worth gating. React declares the geometry on the interface itself, which is
 * what makes the distinction readable off the platform rather than kept as a
 * list here.
 */
function svgGeometry(): Set<string> {
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

/**
 * Every file this gate judges: the app's own JSX and every extension unit's.
 *
 * The walk is `scripts/lib/source-tree.ts`'s, shared with the native-control,
 * extension-import and one-locale gates rather than spelled a fourth time. A
 * unit's screen is bundled into the same SPA and drawn on the same page, so a
 * sweep stopping at `src/` would hold the core to a standard the extension tier
 * escapes.
 *
 * `.tsx` only, because a rendered position is a JSX node and a `.ts` file has
 * none. Stories are IN: a story is the catalog a reader opens, and a figure
 * grouped on the screen and bare in its own story is two answers about one
 * component. Tests are out — an expectation has to be writable, and a suite
 * whose expected text moved with the notation would assert nothing.
 */
function sourceFiles(): string[] {
  return [...filesUnder(srcRoot), ...extensionFrontendFiles(extensionsDir)]
    .filter(
      (path) =>
        path.endsWith(".tsx") &&
        !path.endsWith(".test.tsx") &&
        path !== formatModule &&
        path !== thisGate,
    )
    .sort();
}

// The typed sweep runs off the coverage count, which slows the checker: see
// in-worker.ts.
const SWEEP = new URL(
  "../../scripts/lib/jsx-magnitude.sweep.ts",
  import.meta.url,
);
const blind = blindStringifiers();
const geometry = svgGeometry();
const files = sourceFiles();
const { unloaded, findings, unjudged } = await inWorker<typeof Sweep, "sweep">(
  SWEEP,
  "sweep",
  files,
  blind,
  geometry,
);

describe("a magnitude is drawn in the reader's own notation", () => {
  it("reads the tree it is meant to sweep, extension screens included", () => {
    // DERIVED from the directory listing. A floor does not hold this: the sweep
    // reads several hundred files, so any `toBeGreaterThan` still clears when
    // the walk skips `features/`. Most of the subject lives there.
    const swept = new Set(
      files
        .filter((path) => path.startsWith(srcRoot))
        .map((path) => relative(srcRoot, path).split(/[/\\]/)[0]),
    );
    const expectedDirs = readdirSync(srcRoot, { withFileTypes: true })
      .filter((entry) => entry.isDirectory() && entry.name !== "node_modules")
      .map((entry) => entry.name)
      .filter((name) =>
        filesUnder(join(srcRoot, name)).some(
          (path) => path.endsWith(".tsx") && !path.endsWith(".test.tsx"),
        ),
      );
    expect(expectedDirs.length).toBeGreaterThan(3);
    expect(expectedDirs.filter((name) => !swept.has(name))).toEqual([]);
    expect(files.some((path) => path.includes("/extensions/"))).toBe(true);
    // Stories are swept, and the sweep is TYPED — a corpus the program failed
    // to load would yield no findings and read exactly like a clean tree.
    expect(files.some((path) => path.endsWith(".stories.tsx"))).toBe(true);
    expect(unloaded).toEqual([]);
  });

  it("derives the stringifiers that answer in nobody's notation", () => {
    // Named here as proof the derivation ARRIVED, the way its neighbour proves
    // `NumberFormat` reached its own set. The SET is still derived, so a member
    // the platform grows tomorrow is covered without an edit here.
    expect([...blind].sort()).toEqual([
      "toExponential",
      "toFixed",
      "toPrecision",
      "toString",
    ]);
    // The locale-AWARE member must NOT be in it. It is `one-locale.test.ts`'s
    // subject, and swallowing it here would report every correct call as a
    // finding and get the gate switched off within a week.
    expect(blind.has("toLocaleString")).toBe(false);
  });

  it("derives the geometry it must not mistake for text", () => {
    // Named as proof the derivation ARRIVED at React's declarations; the SET is
    // still read off them, so an attribute the platform grows is covered
    // without an edit here.
    expect(geometry.has("viewBox")).toBe(true);
    expect(geometry.has("d")).toBe(true);
    expect(geometry.size).toBeGreaterThan(100);
    // Own members ONLY. Inherited would swallow the two attributes on an svg
    // most worth gating, and the sweep would go quiet about them.
    expect(geometry.has("aria-label")).toBe(false);
    expect(geometry.has("title")).toBe(false);
  });

  it("sees each shape of the defect, including the ones a grep cannot", async () => {
    // Each line says for ITSELF whether it is a finding, so the expectation is
    // read off the fixture rather than kept beside it as a list of line numbers
    // that has to agree with it — which is the very defect this gate is about.
    const planted: { code: string; finding: boolean }[] = [
      { code: "<p>{label}</p>", finding: false },
      { code: "<p>{formatNumber(count, locale)}</p>", finding: false },
      // The issue's own three shapes.
      { code: "<p>{count}</p>", finding: true },
      { code: "<Stat value={String(count)} />", finding: true },
      { code: "<p>{count} / {limit}</p>", finding: true },
      // Absent is still a magnitude when it arrives.
      { code: "<p>{maybeCount}</p>", finding: true },
      // Every locale-blind stringifier, reached by property and by element
      // access — the spelling a gate anchored on the dot cannot see.
      { code: "<p>{count.toFixed(2)}</p>", finding: true },
      { code: "<p>{count.toString()}</p>", finding: true },
      { code: '<p>{count["toFixed"](2)}</p>', finding: true },
      // The locale-aware one is the other gate's business, and correct here.
      { code: "<p>{count.toLocaleString(tag)}</p>", finding: false },
      // A template literal's substitution, which carries no call to anchor on.
      { code: "<p>{`${count} rows`}</p>", finding: true },
      // Text props, in scope through their DECLARED type and not by name.
      { code: "<p title={String(count)}>x</p>", finding: true },
      { code: "<p aria-label={`${count}`}>x</p>", finding: true },
      // A prop declared `number` is not text: the ruling belongs at the render
      // site inside the component, which this sweep reaches on its own.
      { code: "<Icon size={count} />", finding: false },
      // A drawing's own coordinate space, on an intrinsic tag React declares
      // the geometry of. Not a magnitude, and grouping one breaks the picture.
      { code: "<svg viewBox={`0 0 ${count} ${count}`} />", finding: false },
      // On the SAME tag, the text assistive technology reads aloud. It is
      // declared on `AriaAttributes` rather than on `SVGAttributes` itself,
      // which is why taking the inherited set would have lost it.
      { code: "<svg aria-label={String(count)} />", finding: true },
      // `values` IS an `SVGAttributes` member name, and this is one of our own
      // components rendering words — which is why the exclusion asks about the
      // tag as well as the name.
      { code: "<Chart values={String(count)} />", finding: true },
      // A catalog sentence's parameter. #2463 made the raw number a compile
      // error; `String(...)` satisfies that narrowing and groups nothing, which
      // is the shape it deliberately left standing.
      { code: '<p>{t("k", { n: String(count) })}</p>', finding: true },
      {
        code: '<p>{t("k", { n: formatNumber(count, loc) })}</p>',
        finding: false,
      },
      {
        code: '<p>{t("k", { n: identifierNumber(count) })}</p>',
        finding: false,
      },
      // A ReactNode prop is drawn, so the caller is the site that can rule it.
      { code: "<Slot body={count} />", finding: true },
      { code: "<Slot body={label} />", finding: false },
      // The rulings themselves. These are the whole reason the gate can be
      // absolute, so a mutation that stopped honouring them must fail here.
      { code: "<p>{identifierNumber(count)}</p>", finding: false },
      { code: "<p>{ordinalNumber(count)}</p>", finding: false },
      // A conditional draws whichever arm it picks.
      { code: "<p>{ready ? count : 0}</p>", finding: true },
      { code: "<p>{ready ? label : other}</p>", finding: false },
      // A stray zero from `&&` is a real defect and a DIFFERENT one — the fix
      // is `count > 0 &&`, not a formatter. The gate says so out loud rather
      // than reporting it as this.
      { code: "<p>{count && <b>x</b>}</p>", finding: false },
    ];
    const fixture = await inWorker<typeof Sweep, "plant">(
      SWEEP,
      "plant",
      planted.map(({ code }) => code),
      blind,
      geometry,
    );
    const reported = new Set(fixture.map(({ line }) => line));
    const verdicts = planted.map(({ code, finding }, index) => ({
      code,
      finding,
      reported: reported.has(index + 1),
    }));
    expect(verdicts.filter((row) => row.finding !== row.reported)).toEqual([]);
  });

  it("judges every position it read, or says which it could not", () => {
    // The core tree is judged completely: a rendered position here whose type
    // the checker declined to resolve is a hole in the census, and it fails
    // rather than passing as "no magnitude found".
    expect(
      unjudged.filter((site) => !site.file.startsWith("../extensions/")),
    ).toEqual([]);
  });

  it("finds nothing left in the tree", () => {
    expect(findings).toEqual([]);
  });
});
