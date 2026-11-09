// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A rendered number, date or amount is written in the reader's own locale.
// `INTL_LOCALE` maps the app's three codes onto the BCP-47 tags Intl wants, and
// unconfigured English is en-GB (A100). Everywhere, a locale reaches a
// formatter through `INTL_LOCALE` or not at all.
//
// Two shapes break it, and neither shows on the author's machine. One is no
// locale at all (the browser's guess). The other is the app's own code where
// a tag is wanted (`new Intl.NumberFormat("en")` resolves to en-US). The
// second holds no `toLocale` for a grep to find, so this reads the syntax tree.
//
// The subject is derived from the platform. `intlFormatters()` asks the runtime
// which `Intl` members take a locales argument. `localeMethods()` reads the
// TypeScript lib declarations for methods with a `locales` parameter, and
// where it sits. The planted cases assert both sets by name, so a derivation
// that stopped arriving fails.
//
// Collation and casing are in scope. `format/collate.ts` rules each site by
// function name: `forReader` for a list somebody reads, `stable` for a key,
// `foldForMatch` for a comparison. Outside it, a bare `localeCompare` or
// `toLocale*Case` skipped the ruling.

import { readdirSync } from "node:fs";
import { join } from "node:path";
import ts from "typescript";
import { parseSource, sourceFileAt } from "../../scripts/lib/source-tree";
import { stable } from "../../src/format/collate";
import { type CensusFile, type CensusRule, frontendRoot } from "../tree";

/**
 * The members of `Intl` that take a locales argument, asked of the runtime.
 * Every Intl service that accepts locales carries `supportedLocalesOf`, and
 * nothing else does: `Intl.Locale` is a locale, not a consumer of one.
 */
export function intlFormatters(): string[] {
  return Object.getOwnPropertyNames(Intl)
    .filter((name) => {
      const member = (Intl as unknown as Record<string, unknown>)[name];
      return (
        typeof member === "function" &&
        typeof (member as { supportedLocalesOf?: unknown })
          .supportedLocalesOf === "function"
      );
    })
    .sort(stable);
}

/**
 * The prototype methods that take a locales argument, and where it sits, read
 * out of TypeScript's lib declarations by signature rather than by name.
 * `*Constructor` interfaces are skipped: their `supportedLocalesOf` is a query
 * about locales, not a rendering in one.
 */
export function localeMethods(): Map<string, number> {
  const libDir = join(frontendRoot, "node_modules", "typescript", "lib");
  const found = new Map<string, number>();
  for (const file of readdirSync(libDir)) {
    if (!/^lib\..*\.d\.ts$/.test(file)) {
      continue;
    }
    const parsed = sourceFileAt(join(libDir, file));
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

type Finding = Readonly<{ file: string; line: number; text: string }>;

/**
 * Whether this file imports the mapping from a module that declares it, and
 * declares nothing under that name itself. A file-wide check, not a type
 * check: an ambiguous file fails closed.
 */
function importsTheMapping(
  parsed: ts.SourceFile,
  mappingModules: ReadonlySet<string>,
): boolean {
  const imported = parsed.statements.some((statement) => {
    if (
      !ts.isImportDeclaration(statement) ||
      !ts.isStringLiteral(statement.moduleSpecifier)
    ) {
      return false;
    }
    // The module by its name, however the specifier reaches it.
    const target = statement.moduleSpecifier.text.split("/").pop() ?? "";
    if (!mappingModules.has(target)) {
      return false;
    }
    const bindings = statement.importClause?.namedBindings;
    return (
      bindings !== undefined &&
      ts.isNamedImports(bindings) &&
      bindings.elements.some((element) => element.name.text === "INTL_LOCALE")
    );
  });
  return imported && !bindsTheNameLocally(parsed);
}

/** Whether a local, parameter or destructured binding shadows the import. */
function bindsTheNameLocally(parsed: ts.SourceFile): boolean {
  let bound = false;
  const walk = (node: ts.Node): void => {
    if (bound) {
      return;
    }
    if (
      (ts.isVariableDeclaration(node) ||
        ts.isParameter(node) ||
        ts.isBindingElement(node)) &&
      ts.isIdentifier(node.name) &&
      node.name.text === "INTL_LOCALE"
    ) {
      bound = true;
      return;
    }
    node.forEachChild(walk);
  };
  walk(parsed);
  return bound;
}

/**
 * Whether this argument is `INTL_LOCALE[key]` with an imported mapping and a
 * key that is not a literal. `INTL_LOCALE["en"]` hands every reader English.
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
 * `Intl.DateTimeFormat().resolvedOptions()` asks which zone the runtime is in
 * and renders nothing. The zone-by-purpose rule owns that question.
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
 * The local names bound to an Intl formatter, by destructure, declaration or
 * assignment. A call through one is judged like a direct call.
 */
function intlAliases(
  parsed: ts.SourceFile,
  formatters: readonly string[],
): Set<string> {
  const aliases = new Set<string>();
  const isIntl = (node: ts.Expression): boolean =>
    ts.isIdentifier(node) && node.text === "Intl";
  const walk = (node: ts.Node): void => {
    if (
      ts.isBinaryExpression(node) &&
      node.operatorToken.kind === ts.SyntaxKind.EqualsToken &&
      ts.isIdentifier(node.left)
    ) {
      const member = memberOfIntl(node.right, isIntl);
      if (member !== undefined && formatters.includes(member)) {
        aliases.add(node.left.text);
      }
    }
    if (ts.isVariableDeclaration(node) && node.initializer !== undefined) {
      const init = node.initializer;
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

/** The `Intl` member this expression names, by either spelling. */
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

/** The Intl formatter a callee reaches: `Intl.X`, `Intl["X"]` or an alias. */
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

type Subject = Readonly<{
  formatters: readonly string[];
  methods: ReadonlyMap<string, number>;
  mappingModules: ReadonlySet<string>;
}>;

function findingsOf(
  parsed: ts.SourceFile,
  file: string,
  { formatters, methods, mappingModules }: Subject,
): Finding[] {
  const imported = importsTheMapping(parsed, mappingModules);
  const aliases = intlAliases(parsed, formatters);
  const found: Finding[] = [];
  const at = (node: ts.Node, text: string): void => {
    const { line } = parsed.getLineAndCharacterOfPosition(
      node.getStart(parsed),
    );
    found.push({ file, line: line + 1, text });
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
      // Any receiver: the locale a call renders in does not depend on its
      // type. The argument position comes from the declaration.
      const method = ts.isPropertyAccessExpression(callee)
        ? callee.name.text
        : ts.isElementAccessExpression(callee) &&
            ts.isStringLiteralLike(callee.argumentExpression)
          ? callee.argumentExpression.text
          : undefined;
      const position = method === undefined ? undefined : methods.get(method);
      if (
        position !== undefined &&
        !namesTheMapping(node.arguments?.[position], imported)
      ) {
        at(node, `.${method}()`);
      }
    }
    node.forEachChild(walk);
  };
  walk(parsed);
  return found;
}

/** The findings in planted text that imports the mapping from `format`. */
export function findingsIn(
  path: string,
  text: string,
  formatters: readonly string[],
  methods: ReadonlyMap<string, number>,
): Finding[] {
  return findingsOf(parseSource(path, text), path, {
    formatters,
    methods,
    mappingModules: new Set(["format"]),
  });
}

// Files that reach a formatter without the mapping, each with its reason. A
// formatter that computes rather than renders earns one, and so does a test
// that pins a locale to make an expectation writable.
const pinnedLocales: { file: string; why: string }[] = [
  {
    file: "format/address.ts",
    why: "countryBeyondCity names a country code in EVERY locale this build speaks to compare it with a city, whichever language the city was typed in. The names are matched, never shown.",
  },
  {
    file: "format/format.dates.ts",
    why: "hourInZone reads an hour back as a number. The output is never shown, so the pinned tag fixes the parsing and not the wording.",
  },
  {
    file: "format/calendarday.ts",
    why: "Both sites format in en-CA to COMPUTE, never to render: en-CA yields ISO-8601 YYYY-MM-DD, which is what makes the string comparable and parseable back into parts. A reader's locale here would be a defect — the bucketing would read 'yesterday' because an ICU format changed under it.",
  },
  {
    file: "format/timezone.ts",
    why: "zoneOffsetMs formats in en-US with hourCycle h23 to READ an instant's wall clock back as numbers and derive a zone offset from it. The output is parsed, never shown, so a locale that varied would change the arithmetic rather than the wording. assertIanaZone builds a formatter only to make Intl reject an unknown zone, and shows nothing.",
  },
  {
    file: "features/sign-in/fiscalyear.test.ts",
    why: "Constructs a formatter on a named western clock to show that a UTC-minted 1st-of-the-month instant reads as the PREVIOUS month there — the trap monthName fell into. The claim is about a zone rather than a locale, and it is unwritable without naming both.",
  },
  {
    file: "format/format.test.ts",
    why: "Asserts that Intl ACCEPTS every fixed-offset zone this module refuses, which is the whole reason isRenderableZone exists. That claim is only writable by constructing a formatter with a named locale the assertion chose.",
  },
  {
    file: "mcp-apps/values.ts",
    why: "An MCP app document is inlined whole into a page this product does not own. Importing INTL_LOCALE would be cheap; what is missing is the locale to look it up WITH — no LocaleProvider stands above the document and no signed-in reader's choice reaches inside it, so there is nothing to index the mapping by. The host browser's own locale is the honest remaining answer, and the file says so at the subject.",
  },
  {
    file: "mcp-apps/values.test.ts",
    why: "Reproduces the views' own money formatter to prove the rendering it produces, which needs the same undefined locale that formatter passes.",
  },
];

// Every file under src/ and every extension frontend, except the module that
// owns the two orderings.
const COLLATE_MODULE = "src/format/collate.ts";
const judged = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  file.source !== undefined &&
  file.rel !== COLLATE_MODULE;

// The key a pinned entry names: the path from src/, or from frontend/ for an
// extension.
const keyOf = (file: CensusFile) => file.rel.replace(/^src\//, "");

// The modules that declare the mapping, found by their declaration.
// format.ts re-exports it, so an import may name either.
function mappingModulesIn(corpus: readonly CensusFile[]): Set<string> {
  return new Set([
    "format",
    ...corpus
      .filter(
        (file) =>
          /^src\/format\/[a-z]+\.ts$/.test(file.rel) &&
          file.text.includes("export const INTL_LOCALE"),
      )
      .map((file) => file.rel.replace(/^src\/format\/|\.ts$/g, "")),
  ]);
}

// Each subdirectory of src/ holding a module, derived from the tree.
function unreadDirs(corpus: readonly CensusFile[], all: readonly CensusFile[]) {
  const top = (file: CensusFile) => file.rel.split("/")[1] ?? "";
  const nested = (file: CensusFile) =>
    file.area === "src" &&
    file.source !== undefined &&
    file.rel.split("/").length > 2;
  const read = new Set(corpus.filter(nested).map(top));
  return [...new Set(all.filter(nested).map(top))].filter(
    (dir) => !read.has(dir),
  );
}

export const rule: CensusRule = {
  name: "one-locale",
  corpus: (tree) => tree.files.filter(judged),
  floors: (corpus, tree) => {
    const formatters = intlFormatters();
    const methods = localeMethods();
    const missed = unreadDirs(corpus, tree.files);
    return [
      ...(missed.length === 0
        ? []
        : [`src/ folders not read: ${missed.join(", ")}`]),
      ...(corpus.some((file) => file.area === "extensions")
        ? []
        : ["no extension frontend was read"]),
      ...["NumberFormat", "DateTimeFormat", "RelativeTimeFormat", "PluralRules"]
        .filter((name) => !formatters.includes(name))
        .map((name) => `the Intl formatters derived lack ${name}`),
      ...(methods.get("localeCompare") === 1
        ? []
        : ["localeCompare was not derived with its locale second"]),
    ];
  },
  across: (corpus) => {
    const subject: Subject = {
      formatters: intlFormatters(),
      methods: localeMethods(),
      mappingModules: mappingModulesIn(corpus),
    };
    const all = corpus.flatMap((file) =>
      file.source ? findingsOf(file.source, keyOf(file), subject) : [],
    );
    const pinned = new Set(pinnedLocales.map(({ file }) => file));
    const covered = new Set(all.map(({ file }) => file));
    return [
      ...all
        .filter(({ file }) => !pinned.has(file))
        .map(({ file, line, text }) => `${file}:${line}: ${text}`)
        .sort(stable),
      // An entry whose last pinned site is gone is a standing permission.
      ...pinnedLocales
        .filter(({ file }) => !covered.has(file))
        .map(({ file }) => `${file}: pinned, but reaches no formatter`),
      ...pinnedLocales
        .filter(({ why }) => why.trim().length < 40)
        .map(({ file }) => `${file}: pinned without a reason`),
    ];
  },
  advice:
    "A count goes through formatNumber; a date through formatDate, " +
    "formatDateAbbrev, formatDayMonth or formatDateTime; money through " +
    "formatMoney. A rendering none of those covers builds its formatter with " +
    "INTL_LOCALE[locale]. A file that must pin a locale earns an entry in " +
    "pinnedLocales with its reason.",
};
