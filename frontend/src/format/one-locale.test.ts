// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { INTL_LOCALE } from "./format";

// A rendered number, date or amount is written in SOME locale, and the only
// question is whose. This product's answer is the reader's own — `INTL_LOCALE`
// maps the three codes the app knows onto the BCP-47 tags Intl wants, and
// A100 pins unconfigured English to en-GB rather than en-US, so a date reads
// 21/08/2026 and not 8/21/2026. A call site that reaches a formatter any other
// way has quietly answered the question differently, and this is the gate that
// holds the negative half: outside this module, a locale reaches Intl through
// `INTL_LOCALE` or it does not reach Intl at all.
//
// Two ways it was being answered differently before this gate existed, and
// NEITHER is visible in a screen's rendered output on the machine that wrote
// it — an English developer in en-GB sees the right thing from all three:
//
//   1. No locale at all. `(1234).toLocaleString()` and
//      `new Date(x).toLocaleDateString()` render in the BROWSER's guessed
//      locale, which is the reader's operating system rather than the reader's
//      choice. Twenty-three sites across eleven screens.
//   2. The app's own code where a tag is wanted. `new Intl.NumberFormat("en")`
//      is not `en-GB`; it resolves to en-US and prints US grouping and US
//      dates. Eleven sites across four files, one of which DOCUMENTED its prop
//      as "BCP-47 tag for Intl.NumberFormat" while every caller passed "en".
//
// The second class is why this gate is an AST check rather than a grep: it
// contains no `toLocale` for a text search to find, and a hand census that
// greps for the first class reports the tree clean while half of it is live.
//
// THE SUBJECT IS DERIVED, and that is the load-bearing part. Every gate this
// codebase has grown for a rule like this was later found to have hard-coded
// some fragment of its own subject — a sampled domain list, a restated element
// list, a prefilter narrower than the census behind it — so the thing it was
// written to find kept getting through in the shape it could not see. Here the
// subject is the set of locale-sensitive APIs, and it is read from the platform
// twice over: `intlFormatters()` asks the RUNTIME which members of `Intl` take
// a locales argument, and `localeMethods()` walks TypeScript's own lib
// declarations for the methods that do. Neither is a list maintained here, and
// `DurationFormat` — which this TypeScript version does not declare at all —
// is covered because the runtime knows about it.
//
// That derivation is not free of the same trap; it was written wrong first.
// An earlier `intlFormatters()` read the lib declarations and matched only the
// inline `var X: { new (...) }` shape, so it saw five of the nine — every
// formatter declared through a named constructor interface
// (`var NumberFormat: NumberFormatConstructor`) was invisible, INCLUDING
// `NumberFormat` and `DateTimeFormat`, which are the two this whole gate is
// about. It would have passed a tree full of the defect. The arms below assert
// the derived sets by name for exactly that reason: a gate whose subject is
// derived still has to prove the derivation reached its own subject.
//
// OUT OF SCOPE, named rather than silently dropped: collation
// (`localeCompare`) and locale-sensitive casing (`toLocaleLowerCase` /
// `toLocaleUpperCase`). They are derived into `localeMethods()` below and then
// excluded, because the right answer is per site and opposite in each
// direction — a machine key must sort identically for every reader, so a
// pinned "en" is CORRECT there, while a human-readable label must not. That is
// a ruling per call site rather than a sweep. Tracked in the issue named in
// `COLLATION_ISSUE`.

const here = dirname(fileURLToPath(import.meta.url));
const srcRoot = join(here, "..");

// The one home for the rule. It is the module the mapping lives in, so it is
// the module allowed to reach a formatter without going through the mapping.
const formatModule = here;

// Named so a reader of a finding can reach the open question rather than
// guessing that collation was overlooked.
const COLLATION_ISSUE = "the collation half of this rule is filed separately";

/**
 * The members of `Intl` that take a locales argument, asked of the runtime.
 *
 * `supportedLocalesOf` is not a proxy for "takes a locale" — it is the same
 * fact. Every Intl service that accepts a locales argument is required to
 * carry it, and nothing else in the namespace does: `Intl.Locale` is a locale
 * rather than a consumer of one and correctly falls out.
 */
function intlFormatters(): string[] {
  return Object.getOwnPropertyNames(Intl)
    .filter((name) => {
      const member = (Intl as unknown as Record<string, unknown>)[name];
      return (
        typeof member === "function" &&
        typeof (member as { supportedLocalesOf?: unknown })
          .supportedLocalesOf === "function"
      );
    })
    .sort();
}

/**
 * The prototype methods that take a locales argument, walked out of
 * TypeScript's own lib declarations.
 *
 * From the declarations rather than from the runtime, because the runtime can
 * only be asked about prototypes somebody names here — and naming them is the
 * hand-maintained list this is avoiding. The lib files describe the whole
 * standard library, so a method added to a type nobody thought of still lands.
 */
function localeMethods(): string[] {
  const libDir = join(srcRoot, "..", "node_modules", "typescript", "lib");
  const found = new Set<string>();
  for (const file of readdirSync(libDir)) {
    if (!/^lib\..*\.d\.ts$/.test(file)) {
      continue;
    }
    const parsed = ts.createSourceFile(
      file,
      readFileSync(join(libDir, file), "utf8"),
      ts.ScriptTarget.Latest,
      true,
      ts.ScriptKind.TS,
    );
    const walk = (node: ts.Node): void => {
      if (ts.isInterfaceDeclaration(node)) {
        for (const member of node.members) {
          const name = member.name?.getText(parsed);
          if (name && (/^toLocale/.test(name) || name === "localeCompare")) {
            found.add(name);
          }
        }
      }
      node.forEachChild(walk);
    };
    walk(parsed);
  }
  return [...found].sort();
}

// Collation and casing, excluded from the rule for the reason at the top of
// this file. Derived from the same walk so the exclusion is a decision about
// members that were FOUND, never a shorter search.
const isRenderingMethod = (name: string): boolean =>
  /^toLocale/.test(name) &&
  name !== "toLocaleLowerCase" &&
  name !== "toLocaleUpperCase";

function sourceFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      return entry.name === "node_modules" || entry.name === "dist"
        ? []
        : sourceFiles(path);
    }
    if (dirname(path) === formatModule) {
      // The module IS the permitted site, and this gate spells out the very
      // shapes it hunts for. A sweep that read either would vouch for itself.
      return [];
    }
    return /\.tsx?$/.test(entry.name) ? [path] : [];
  });
}

type Finding = Readonly<{ file: string; line: number; text: string }>;

/**
 * Whether this expression is the one permitted way to name a locale.
 *
 * `INTL_LOCALE[whatever]` and nothing else. Not a variable that happens to
 * hold a tag, and not a string literal: the point of the rule is that the
 * mapping is READ at the call site, so a reader of the line can see which
 * question was answered. A literal tag is the second locale table this module
 * exists to prevent, whatever its value.
 */
function namesTheMapping(argument: ts.Expression | undefined): boolean {
  return (
    argument !== undefined &&
    ts.isElementAccessExpression(argument) &&
    ts.isIdentifier(argument.expression) &&
    argument.expression.text === "INTL_LOCALE"
  );
}

/**
 * A zone lookup wearing a formatter's clothes.
 *
 * `Intl.DateTimeFormat().resolvedOptions()` constructs a formatter to ask it
 * which zone the runtime is in, and renders nothing. `viewerZone()` owns that
 * question and `format/zone-by-purpose.test.ts` already gates it, so reporting
 * it here would put one rule in two homes and hand this gate a copy of that
 * one's forty exemptions.
 */
function isZoneLookup(node: ts.Node): boolean {
  const parent = node.parent;
  return (
    parent !== undefined &&
    ts.isPropertyAccessExpression(parent) &&
    parent.name.text === "resolvedOptions"
  );
}

function findingsIn(
  path: string,
  source: string,
  formatters: readonly string[],
  methods: readonly string[],
): Finding[] {
  const parsed = ts.createSourceFile(
    path,
    source,
    ts.ScriptTarget.Latest,
    true,
    path.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
  );
  const rel = relative(srcRoot, path).split("\\").join("/");
  const found: Finding[] = [];
  const at = (node: ts.Node, text: string): void => {
    found.push({
      file: rel,
      line:
        parsed.getLineAndCharacterOfPosition(node.getStart(parsed)).line + 1,
      text,
    });
  };
  const walk = (node: ts.Node): void => {
    if (ts.isNewExpression(node) || ts.isCallExpression(node)) {
      const callee = node.expression;
      // `Intl.NumberFormat(...)` — the namespace named, so a local called
      // `NumberFormat` is not mistaken for one.
      if (
        ts.isPropertyAccessExpression(callee) &&
        ts.isIdentifier(callee.expression) &&
        callee.expression.text === "Intl" &&
        formatters.includes(callee.name.text) &&
        !isZoneLookup(node) &&
        !namesTheMapping(node.arguments?.[0])
      ) {
        at(node, `Intl.${callee.name.text}`);
      }
      // `value.toLocaleDateString(...)` — any receiver. The rule is about the
      // locale the call renders in, and that does not depend on the type.
      if (
        ts.isPropertyAccessExpression(callee) &&
        methods.includes(callee.name.text) &&
        isRenderingMethod(callee.name.text) &&
        !namesTheMapping(node.arguments?.[0])
      ) {
        at(node, `.${callee.name.text}()`);
      }
    }
    node.forEachChild(walk);
  };
  walk(parsed);
  return found;
}

// Files that legitimately reach a formatter without the mapping, each with the
// reason. A test or a story PINS a locale to make an expectation writable,
// which is the one thing the mapping cannot stand in for: a suite whose
// expected output moved with the reader's browser would assert nothing.
const pinnedLocales: { file: string; why: string }[] = [
  {
    file: "mcp-apps/bridge.ts",
    why: "An MCP app document is inlined whole into a page this product does not own: there is no LocaleProvider above it and no i18n bundle inside it, and importing one to learn a single tag would ship the whole catalog to a third-party host. The host browser's own locale is the honest remaining answer, and the file states that at the subject.",
  },
  {
    file: "mcp-apps/bridge.test.ts",
    why: "Reproduces the bridge's own formatter to prove the rendering it produces, which needs the same undefined locale the bridge passes.",
  },
];

const pinnedFiles = new Set(pinnedLocales.map(({ file }) => file));

describe("one locale for every rendered value", () => {
  const formatters = intlFormatters();
  const methods = localeMethods();
  const files = sourceFiles(srcRoot);
  const all = files.flatMap((path) =>
    findingsIn(path, readFileSync(path, "utf8"), formatters, methods),
  );

  it("reads the tree it is meant to sweep", () => {
    // A miswired walk passes every assertion below by inspecting nothing.
    expect(files.length).toBeGreaterThan(200);
    expect(COLLATION_ISSUE.length).toBeGreaterThan(0);
  });

  it("derives the Intl formatters it gates, reaching the two it is about", () => {
    // The first version of this derivation returned five of nine and left out
    // NumberFormat and DateTimeFormat — the entire point of the gate — because
    // it understood one of the two declaration shapes. Naming them here is the
    // proof that the derivation reached its own subject; the SET is still
    // derived, so a formatter added to the platform tomorrow is gated without
    // an edit here.
    expect(formatters).toContain("NumberFormat");
    expect(formatters).toContain("DateTimeFormat");
    expect(formatters).toContain("RelativeTimeFormat");
    expect(formatters).toContain("PluralRules");
    expect(formatters.length).toBeGreaterThanOrEqual(8);
    // Intl.Locale IS a locale rather than a consumer of one, so it must not be
    // gated — a derivation that swept it up would report every construction of
    // one as a finding and get switched off within a week.
    expect(formatters).not.toContain("Locale");
  });

  it("derives the locale-sensitive methods, and holds the line it draws", () => {
    expect(methods).toEqual([
      "localeCompare",
      "toLocaleDateString",
      "toLocaleLowerCase",
      "toLocaleString",
      "toLocaleTimeString",
      "toLocaleUpperCase",
    ]);
    // The rendering half is gated; collation and casing are a different rule
    // with the opposite right answer at some sites, and are excluded here
    // rather than left out of the search. If this ever narrows silently, the
    // set above fails first.
    expect(methods.filter(isRenderingMethod)).toEqual([
      "toLocaleDateString",
      "toLocaleString",
      "toLocaleTimeString",
    ]);
  });

  it("sees each shape of the defect, including the ones a grep cannot", () => {
    const planted = [
      // No locale at all — the browser's guess.
      "const a = (1234).toLocaleString();",
      "const b = new Date(x).toLocaleDateString();",
      "const c = new Date(x).toLocaleTimeString(undefined, {});",
      // A locale, but not through the mapping. `"en"` is not `en-GB`, and this
      // whole class contains no `toLocale` for a text search to find.
      "const d = new Intl.NumberFormat(locale).format(1);",
      'const e = new Intl.NumberFormat("en-US").format(1);',
      "const f = Intl.DateTimeFormat(locale).format(now);",
      // A formatter the TypeScript lib in this tree does not declare, reached
      // because the runtime does know it.
      "const g = new Intl.DurationFormat(locale);",
      // A tag held in a variable is still a second answer: the point is that
      // the mapping is read where the reader of the line can see it.
      "const h = new Intl.NumberFormat(tag).format(1);",
    ].join("\n");
    const lines = findingsIn("planted.ts", planted, formatters, methods).map(
      ({ line }) => line,
    );
    expect(lines).toEqual([1, 2, 3, 4, 5, 6, 7, 8]);
  });

  it("passes the one permitted spelling, and the zone lookup next door", () => {
    const clean = [
      "const a = new Intl.NumberFormat(INTL_LOCALE[locale]).format(1);",
      "const b = new Intl.DateTimeFormat(INTL_LOCALE[locale], {}).format(d);",
      // The zone lookup constructs a formatter to read a zone and renders
      // nothing; zone-by-purpose.test.ts owns it.
      "const z = Intl.DateTimeFormat().resolvedOptions().timeZone;",
      // Collation and casing are the excluded half.
      'const s = a.localeCompare(b, "en");',
      "const t = name.toLocaleLowerCase();",
      // A local named like a formatter is not the Intl one.
      "const u = new NumberFormat(whatever);",
    ].join("\n");
    expect(findingsIn("clean.ts", clean, formatters, methods)).toEqual([]);
  });

  it("leaves no surface rendering in a locale of its own", () => {
    const loose = all
      .filter(({ file }) => !pinnedFiles.has(file))
      .map(({ file, line, text }) => `${file}:${line}: ${text}`);
    // Zero, not a ratchet. A count goes through `formatNumber`; a date through
    // `formatDate`, `formatDateAbbrev`, `formatDayMonth` or `formatDateTime`;
    // money through `formatMoney`. A rendering none of those covers builds its
    // formatter with `INTL_LOCALE[locale]` so the reader of the line can see
    // whose locale was chosen. A file that must pin one earns an entry in
    // `pinnedLocales` with the reason it needs to.
    expect(loose.sort()).toEqual([]);
  });

  it("keeps no exemption that has stopped covering a site", () => {
    // An allowlist is only as honest as its narrowest entry. Once the last
    // pinned site in a file is gone the entry is a standing permission nobody
    // needs, and the next screen to render in the browser's locale inherits
    // the excuse.
    const covered = new Set(all.map(({ file }) => file));
    const idle = pinnedLocales
      .filter(({ file }) => !covered.has(file))
      .map(({ file }) => file);
    expect(idle.sort()).toEqual([]);
  });

  it("states a reason for every exemption", () => {
    const unexplained = pinnedLocales
      .filter(({ why }) => why.trim().length < 40)
      .map(({ file }) => file);
    // A reasonless entry records that somebody wanted the gate quiet, not
    // that the file has a case.
    expect(unexplained).toEqual([]);
  });

  it("pins the mapping the whole rule is written against", () => {
    // The gate is worth nothing if the table it points every call site at has
    // itself drifted: A100 is that unconfigured English is en-GB, and en-US
    // formats both dates and numbers differently.
    expect(INTL_LOCALE).toEqual({ de: "de-DE", en: "en-GB", vi: "vi-VN" });
  });
});
