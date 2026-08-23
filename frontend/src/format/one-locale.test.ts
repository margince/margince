// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  extensionFrontendFiles,
  filesUnder,
  scriptKindFor,
} from "../../scripts/lib/source-tree";
import { INTL_LOCALE } from "./format";

// A rendered number, date or amount is written in SOME locale, and the only
// question is whose. This product's answer is the reader's own — `INTL_LOCALE`
// maps the three codes the app knows onto the BCP-47 tags Intl wants, and A100
// pins unconfigured English to en-GB rather than en-US, so a date reads
// 21/08/2026 and not 8/21/2026. A call site that reaches a formatter any other
// way has quietly answered the question differently, and this is the gate that
// holds the negative half: outside `format.ts`, a locale reaches a formatter
// through `INTL_LOCALE` or it does not reach one at all.
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
// The second class is why this is an AST check rather than a grep: it contains
// no `toLocale` for a text search to find, and a hand census that greps for the
// first class reports the tree clean while half of it is live.
//
// THE SUBJECT IS DERIVED, and that is the load-bearing part. Every gate this
// codebase has grown for a rule like this was later found to have hard-coded
// some fragment of its own subject — a sampled domain list, a restated element
// list, a prefilter narrower than the census behind it — so the thing it was
// written to find kept getting through in the shape it could not see. Here the
// subject is the set of locale-sensitive APIs, read from the platform twice
// over: `intlFormatters()` asks the RUNTIME which members of `Intl` take a
// locales argument, and `localeMethods()` reads TypeScript's own lib
// declarations for the methods whose SIGNATURE has a `locales` parameter —
// which also yields the position that argument sits at, so nothing here knows
// that `toLocaleString` takes it first and `localeCompare` second. Neither set
// is a list maintained in this file, and `DurationFormat` — which this
// TypeScript version does not declare at all — is covered because the runtime
// knows it.
//
// That derivation is not immune to the same trap; it was written wrong twice.
// An earlier `intlFormatters()` read the lib declarations and matched only the
// inline `var X: { new (...) }` shape, so it saw five of the nine — every
// formatter declared through a named constructor interface
// (`var NumberFormat: NumberFormatConstructor`) was invisible, INCLUDING
// `NumberFormat` and `DateTimeFormat`, which are the two this whole gate is
// about. And an earlier `localeMethods()` selected by NAME (`/^toLocale/`),
// which is precisely the hand-maintained fragment the paragraph above warns
// against. The arms below assert the derived sets by name for that reason: a
// gate whose subject is derived still has to prove the derivation arrived.
//
// OUT OF SCOPE, named rather than silently dropped: collation
// (`localeCompare`) and locale-sensitive casing (`toLocaleLowerCase` /
// `toLocaleUpperCase`). They are derived into the set below and then excluded,
// because the right answer is per site and opposite in each direction — a
// machine key must sort identically for every reader, so a pinned "en" is
// CORRECT there, while a human-readable label must not. That is a ruling per
// call site rather than a sweep. Tracked in `COLLATION_ISSUE`; once each site
// there has one, `isRenderingMethod` stops excluding them and this gate holds
// both halves.

const here = dirname(fileURLToPath(import.meta.url));
const srcRoot = join(here, "..");
const frontendRoot = resolve(srcRoot, "..");
const extensionsDir = resolve(frontendRoot, "..", "extensions");

// The one home for the rule: the module the mapping lives in, and so the one
// module allowed to reach a formatter without going through it. The FILE, not
// the directory it sits in — see `sourceFiles` for why that distinction is the
// whole difference between a narrow exception and a blind spot.
const formatModule = join(here, "format.ts");
const thisGate = fileURLToPath(import.meta.url);

// Named so a reader of a finding can reach the open question rather than
// guessing that collation was overlooked. The issue number and nothing else:
// an earlier version carried a site COUNT, which nothing here derives and
// nothing here checks — a number in a gate that no assertion holds is the same
// stale claim this whole rule is about.
const COLLATION_ISSUE =
  "#2455 — collation and locale-sensitive casing need a ruling per call site";

/**
 * The members of `Intl` that take a locales argument, asked of the runtime.
 *
 * `supportedLocalesOf` is not a proxy for "takes a locale" — it is the same
 * fact. Every Intl service that accepts a locales argument is required to carry
 * it, and nothing else in the namespace does: `Intl.Locale` is a locale rather
 * than a consumer of one and correctly falls out.
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
 * The prototype methods that take a locales argument, and WHERE it sits, read
 * out of TypeScript's own lib declarations.
 *
 * Selected by SIGNATURE — a parameter named `locales` — not by name. Selecting
 * by name is a hand-maintained fragment of the subject wearing a regular
 * expression's clothes: it would miss anything a future standard called
 * something else, and it answers a different question from the one this gate
 * asks.
 *
 * `*Constructor` interfaces are excluded because their `supportedLocalesOf` is
 * a QUERY about locales rather than a rendering in one, and that exclusion is
 * derived from the declaring interface's own name rather than from the
 * method's.
 *
 * From the declarations and not the runtime, because the runtime can only be
 * asked about prototypes somebody names here — and naming them is the
 * hand-maintained list this is avoiding.
 */
function localeMethods(): Map<string, number> {
  const libDir = join(frontendRoot, "node_modules", "typescript", "lib");
  const found = new Map<string, number>();
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
      if (
        ts.isInterfaceDeclaration(node) &&
        !/Constructor$/.test(node.name.text)
      ) {
        for (const member of node.members) {
          if (!ts.isMethodSignature(member) || member.name === undefined) {
            continue;
          }
          const at = member.parameters.findIndex(
            (parameter) =>
              ts.isIdentifier(parameter.name) &&
              parameter.name.text === "locales",
          );
          if (at >= 0) {
            found.set(member.name.getText(parsed), at);
          }
        }
      }
      node.forEachChild(walk);
    };
    walk(parsed);
  }
  return found;
}

// Collation and casing, excluded for the reason at the top of this file. A
// decision about members that were FOUND, never a shorter search.
const isRenderingMethod = (name: string): boolean =>
  name !== "localeCompare" &&
  name !== "toLocaleLowerCase" &&
  name !== "toLocaleUpperCase";

/**
 * Every file this gate judges: the app's own sources and every extension unit's
 * frontend.
 *
 * The walk is `scripts/lib/source-tree.ts`'s, shared with the native-control
 * and extension-import gates rather than spelled a third time. A unit's screen
 * is bundled into the same SPA and drawn on the same page, so a gate stopping
 * at `src/` would hold the core to a standard the extension tier escapes — and
 * a walk is the worst place for a second answer, because the narrower one reads
 * a smaller tree and reports the same word for it: PASS.
 *
 * TWO files are excluded, named rather than the directory they sit in.
 * `format.ts` owns the mapping; this gate spells out the very shapes it hunts
 * for, and a sweep that read it would vouch for itself. The DIRECTORY is not
 * the unit, and excluding it was this gate's own first hole: `format/` holds
 * seven other modules, so a new formatter added beside `format.ts` could render
 * in the browser's locale and pass. A gate's exception is where it goes blind,
 * and the exception has to be as narrow as the reason for it.
 */
function sourceFiles(): string[] {
  return [...filesUnder(srcRoot), ...extensionFrontendFiles(extensionsDir)]
    .filter((path) => path !== formatModule && path !== thisGate)
    .sort();
}

type Finding = Readonly<{ file: string; line: number; text: string }>;

/**
 * Whether this file imports the mapping from the module that owns it.
 *
 * Without this, `namesTheMapping` is satisfied by ANY identifier spelled
 * `INTL_LOCALE` — a local, a parameter, a second locale table the file made up.
 * Requiring the import is not a type check and does not pretend to be one; it
 * is the cheap half that closes the shadowing case without standing up a whole
 * TypeScript program, and it is asked once per file rather than per call.
 */
function importsTheMapping(parsed: ts.SourceFile): boolean {
  return parsed.statements.some((statement) => {
    if (
      !ts.isImportDeclaration(statement) ||
      !ts.isStringLiteral(statement.moduleSpecifier) ||
      !/\/format\/format$/.test(statement.moduleSpecifier.text)
    ) {
      return false;
    }
    const bindings = statement.importClause?.namedBindings;
    return (
      bindings !== undefined &&
      ts.isNamedImports(bindings) &&
      bindings.elements.some((element) => element.name.text === "INTL_LOCALE")
    );
  });
}

/**
 * Whether this expression is the one permitted way to name a locale.
 *
 * `INTL_LOCALE[key]`, where the file imports `INTL_LOCALE` from the module that
 * declares it and `key` is not a literal. Not a variable that happens to hold a
 * tag, and not a string literal: the point of the rule is that the mapping is
 * READ for the reader's locale at the call site.
 *
 * The literal-key half matters on its own. `INTL_LOCALE["en"]` reads the shared
 * table and still hands every reader English — it is a pinned locale wearing
 * the mapping's clothes, and it passed an earlier version of this predicate,
 * which looked only at the identifier.
 */
function namesTheMapping(
  argument: ts.Expression | undefined,
  imported: boolean,
): boolean {
  return (
    imported &&
    argument !== undefined &&
    ts.isElementAccessExpression(argument) &&
    ts.isIdentifier(argument.expression) &&
    argument.expression.text === "INTL_LOCALE" &&
    !ts.isStringLiteralLike(argument.argumentExpression) &&
    !ts.isNumericLiteral(argument.argumentExpression)
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

/**
 * The local names this file has bound to an Intl formatter.
 *
 * `const { NumberFormat } = Intl` and `const NF = Intl.NumberFormat` both leave
 * a formatter reachable under a name that is not `Intl.something`, and a gate
 * anchored on the namespace sees neither. Collected so a construction through
 * one of them is judged exactly like a direct call: an alias is not a second
 * formatter, it is the same one under another name, so the same rule applies.
 */
function intlAliases(
  parsed: ts.SourceFile,
  formatters: readonly string[],
): Set<string> {
  const aliases = new Set<string>();
  const isIntl = (node: ts.Expression): boolean =>
    ts.isIdentifier(node) && node.text === "Intl";
  const walk = (node: ts.Node): void => {
    if (ts.isVariableDeclaration(node) && node.initializer !== undefined) {
      const init = node.initializer;
      // const { NumberFormat, DateTimeFormat } = Intl
      if (ts.isObjectBindingPattern(node.name) && isIntl(init)) {
        for (const element of node.name.elements) {
          const source = element.propertyName ?? element.name;
          if (
            ts.isIdentifier(source) &&
            formatters.includes(source.text) &&
            ts.isIdentifier(element.name)
          ) {
            aliases.add(element.name.text);
          }
        }
      }
      // const NF = Intl.NumberFormat  /  const NF = Intl["NumberFormat"]
      if (ts.isIdentifier(node.name)) {
        const member = memberOfIntl(init, isIntl);
        if (member !== undefined && formatters.includes(member)) {
          aliases.add(node.name.text);
        }
      }
    }
    node.forEachChild(walk);
  };
  walk(parsed);
  return aliases;
}

/** The `Intl` member this expression names, by either spelling, or undefined. */
function memberOfIntl(
  node: ts.Expression,
  isIntl: (candidate: ts.Expression) => boolean,
): string | undefined {
  if (ts.isPropertyAccessExpression(node) && isIntl(node.expression)) {
    return node.name.text;
  }
  if (
    ts.isElementAccessExpression(node) &&
    isIntl(node.expression) &&
    ts.isStringLiteralLike(node.argumentExpression)
  ) {
    return node.argumentExpression.text;
  }
  return undefined;
}

/**
 * The Intl formatter this call reaches, whatever spelling it used.
 *
 * Three spellings, because a namespace member can be reached three ways and a
 * gate that knew one of them would wave the other two through:
 * `Intl.NumberFormat`, `Intl["NumberFormat"]`, and a local bound to either.
 */
function formatterReached(
  callee: ts.Expression,
  formatters: readonly string[],
  aliases: ReadonlySet<string>,
): string | undefined {
  if (ts.isIdentifier(callee) && aliases.has(callee.text)) {
    return callee.text;
  }
  const member = memberOfIntl(
    callee,
    (candidate) => ts.isIdentifier(candidate) && candidate.text === "Intl",
  );
  return member !== undefined && formatters.includes(member)
    ? member
    : undefined;
}

function findingsIn(
  path: string,
  source: string,
  formatters: readonly string[],
  methods: ReadonlyMap<string, number>,
): Finding[] {
  const parsed = ts.createSourceFile(
    path,
    source,
    ts.ScriptTarget.Latest,
    true,
    scriptKindFor(path),
  );
  const rel = relative(srcRoot, path).split("\\").join("/");
  const imported = importsTheMapping(parsed);
  const aliases = intlAliases(parsed, formatters);
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
      const formatter = formatterReached(callee, formatters, aliases);
      if (
        formatter !== undefined &&
        !isZoneLookup(node) &&
        !namesTheMapping(node.arguments?.[0], imported)
      ) {
        at(node, `Intl.${formatter}`);
      }
      // `value.toLocaleDateString(...)` — any receiver. The rule is about the
      // locale the call renders in, and that does not depend on the type. The
      // ARGUMENT POSITION comes from the declaration rather than from this
      // file's knowledge of which method puts it where.
      if (ts.isPropertyAccessExpression(callee)) {
        const name = callee.name.text;
        const position = methods.get(name);
        if (
          position !== undefined &&
          isRenderingMethod(name) &&
          !namesTheMapping(node.arguments?.[position], imported)
        ) {
          at(node, `.${name}()`);
        }
      }
    }
    node.forEachChild(walk);
  };
  walk(parsed);
  return found;
}

// Files that legitimately reach a formatter without the mapping, each with the
// reason. Two kinds earn one: a formatter used to COMPUTE rather than to render
// (its output is parsed back by code, so a reader's locale would change the
// arithmetic), and a test or story that PINS a locale to make an expectation
// writable — the one thing the mapping cannot stand in for, since a suite whose
// expected output moved with the reader's browser would assert nothing.
const pinnedLocales: { file: string; why: string }[] = [
  {
    file: "format/calendarday.ts",
    why: "Both sites format in en-CA to COMPUTE, never to render: en-CA yields ISO-8601 YYYY-MM-DD, which is what makes the string comparable and parseable back into parts. A reader's locale here would be a defect — the bucketing would read 'yesterday' because an ICU format changed under it.",
  },
  {
    file: "format/timezone.ts",
    why: "zoneOffsetMs formats in en-US with hourCycle h23 to READ an instant's wall clock back as numbers and derive a zone offset from it. The output is parsed, never shown, so a locale that varied would change the arithmetic rather than the wording.",
  },
  {
    file: "format/format.test.ts",
    why: "Asserts that Intl ACCEPTS every fixed-offset zone this module refuses, which is the whole reason isRenderableZone exists. That claim is only writable by constructing a formatter with a named locale the assertion chose.",
  },
  {
    file: "format/zone-by-purpose.test.ts",
    why: "Proves the record zone is one Intl will accept, which needs a formatter built with a locale that suite pinned rather than one that moved with the machine it ran on.",
  },
  {
    file: "mcp-apps/bridge.ts",
    why: "An MCP app document is inlined whole into a page this product does not own. Importing INTL_LOCALE would be cheap; what is missing is the locale to look it up WITH — no LocaleProvider stands above the document and no signed-in reader's choice reaches inside it, so there is nothing to index the mapping by. The host browser's own locale is the honest remaining answer, and the file says so at the subject.",
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
  const files = sourceFiles();
  const all = files.flatMap((path) =>
    findingsIn(path, readFileSync(path, "utf8"), formatters, methods),
  );

  it("reads the tree it is meant to sweep, extension screens included", () => {
    // A miswired walk passes every assertion below by inspecting nothing.
    expect(files.length).toBeGreaterThan(200);
    // And the extension tier is genuinely in it: a unit's screen ships in the
    // same bundle, so a sweep that quietly stopped at src/ would hold the core
    // to a standard the extension tier escapes.
    expect(files.some((path) => path.includes("/extensions/"))).toBe(true);
    // The excluded half names where it went. A gate that narrows its own
    // subject and says nothing about it reads exactly like one that found the
    // tree clean.
    expect(COLLATION_ISSUE).toMatch(/#\d+/);
  });

  it("derives the Intl formatters it gates, reaching the two it is about", () => {
    // The first version of this derivation returned five of nine and left out
    // NumberFormat and DateTimeFormat — the entire point of the gate — because
    // it understood one of two declaration shapes. Naming them here is the
    // proof that the derivation arrived; the SET is still derived, so a
    // formatter added to the platform tomorrow is gated without an edit here.
    expect(formatters).toContain("NumberFormat");
    expect(formatters).toContain("DateTimeFormat");
    expect(formatters).toContain("RelativeTimeFormat");
    expect(formatters).toContain("PluralRules");
    expect(formatters.length).toBeGreaterThanOrEqual(8);
    // Intl.Locale IS a locale rather than a consumer of one, so it must not be
    // gated — a derivation that swept it up would report every construction of
    // one as a finding and be switched off within a week.
    expect(formatters).not.toContain("Locale");
  });

  it("derives the locale-sensitive methods and where each takes its locale", () => {
    expect([...methods.entries()].sort()).toEqual([
      ["localeCompare", 1],
      ["toLocaleDateString", 0],
      ["toLocaleLowerCase", 0],
      ["toLocaleString", 0],
      ["toLocaleTimeString", 0],
      ["toLocaleUpperCase", 0],
    ]);
    // localeCompare taking its locale SECOND is why the position is derived
    // rather than assumed: a gate that read argument 0 for every method would
    // judge the string being compared and report every call.
    expect(methods.get("localeCompare")).toBe(1);
    // supportedLocalesOf has a `locales` parameter too and renders nothing. It
    // is excluded by its declaring interface's name, not by its own.
    expect(methods.has("supportedLocalesOf")).toBe(false);
    // The rendering half is gated; collation and casing are the excluded half.
    // If that line ever moves silently, the map above fails first.
    expect([...methods.keys()].filter(isRenderingMethod).sort()).toEqual([
      "toLocaleDateString",
      "toLocaleString",
      "toLocaleTimeString",
    ]);
  });

  it("sees each shape of the defect, including the ones a grep cannot", () => {
    const planted = [
      // The fixture IMPORTS the mapping, which is the realistic shape: a file
      // that uses it correctly somewhere and then builds a second formatter
      // beside it. It also keeps this arm honest — without the import every
      // line below is refused by `importsTheMapping` before the argument is
      // ever looked at, so the argument check would be untested and a mutation
      // removing it passed this suite. It did, once.
      'import { INTL_LOCALE } from "../format/format";',
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
      // The mapping read with a PINNED key: the shared table, and still English
      // for every reader. This passed an earlier predicate that looked only at
      // the identifier.
      'const i = new Intl.NumberFormat(INTL_LOCALE["en"]).format(1);',
      // The namespace reached by element access rather than by property.
      'const j = new Intl["NumberFormat"](locale).format(1);',
      // And reached through a local bound to it — a destructure and an alias.
      "const { DateTimeFormat } = Intl;",
      "const k = new DateTimeFormat(locale).format(now);",
      "const NF = Intl.NumberFormat;",
      "const l = new NF(locale).format(1);",
    ].join("\n");
    const lines = findingsIn("planted.ts", planted, formatters, methods).map(
      ({ line }) => line,
    );
    expect(lines).toEqual([2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 13, 15]);
  });

  it("passes the one permitted spelling, and the zone lookup next door", () => {
    const clean = [
      'import { INTL_LOCALE } from "../format/format";',
      "const a = new Intl.NumberFormat(INTL_LOCALE[locale]).format(1);",
      "const b = new Intl.DateTimeFormat(INTL_LOCALE[locale], {}).format(d);",
      // The zone lookup constructs a formatter to read a zone and renders
      // nothing; zone-by-purpose.test.ts owns it.
      "const z = Intl.DateTimeFormat().resolvedOptions().timeZone;",
      // Collation and casing are the excluded half.
      'const s = a.localeCompare(b, "en");',
      "const t = name.toLocaleLowerCase();",
      // A local named like a formatter, bound to something that is not Intl's.
      "const NumberFormat = ourOwnThing;",
      "const u = new NumberFormat(whatever);",
    ].join("\n");
    expect(findingsIn("clean.ts", clean, formatters, methods)).toEqual([]);
  });

  it("refuses a mapping the file never imported", () => {
    // Without the import requirement the permitted spelling is satisfied by ANY
    // identifier spelled INTL_LOCALE — a local, a parameter, a second table the
    // file declared — so one could be read through a name that looks like the
    // shared one. Cheap to close, and it costs one check per file rather than a
    // whole TypeScript program.
    const shadowed = [
      'const INTL_LOCALE = { en: "en-US" };',
      "const a = new Intl.NumberFormat(INTL_LOCALE[locale]).format(1);",
    ].join("\n");
    expect(
      findingsIn("shadowed.ts", shadowed, formatters, methods).map(
        ({ line }) => line,
      ),
    ).toEqual([2]);
    // A default import of the module does not carry the binding either.
    const wrongImport = [
      'import format from "../format/format";',
      "const a = new Intl.NumberFormat(INTL_LOCALE[locale]).format(1);",
    ].join("\n");
    expect(
      findingsIn("wrong.ts", wrongImport, formatters, methods).map(
        ({ line }) => line,
      ),
    ).toEqual([2]);
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
    // needs, and the next screen to render in the browser's locale inherits the
    // excuse.
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
    // A reasonless entry records that somebody wanted the gate quiet, not that
    // the file has a case.
    expect(unexplained).toEqual([]);
  });

  it("pins the mapping the whole rule is written against", () => {
    // The gate is worth nothing if the table it points every call site at has
    // itself drifted: A100 is that unconfigured English is en-GB, and en-US
    // formats both dates and numbers differently.
    expect(INTL_LOCALE).toEqual({ de: "de-DE", en: "en-GB", vi: "vi-VN" });
  });
});
