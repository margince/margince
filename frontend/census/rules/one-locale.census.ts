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
export function isZoneLookup(node: ts.Node): boolean {
  const parent = node.parent;
  return (
    parent !== undefined &&
    ts.isPropertyAccessExpression(parent) &&
    parent.name.text === "resolvedOptions"
  );
}

/** The local names bound to the `Intl` namespace and to its formatters. */
export type IntlBindings = Readonly<{
  namespaces: ReadonlySet<string>;
  formatters: ReadonlySet<string>;
}>;

// `Intl` itself, a name bound to it, or the global's own property.
function namesIntl(node: ts.Expression, namespaces: ReadonlySet<string>) {
  return (
    (ts.isIdentifier(node) && namespaces.has(node.text)) ||
    (ts.isPropertyAccessExpression(node) && node.name.text === "Intl")
  );
}

/** Each local name a declaration or assignment binds to `value`. */
function bindingsOf(parsed: ts.SourceFile) {
  const found: { name: string; value: ts.Expression }[] = [];
  const walk = (node: ts.Node): void => {
    if (
      ts.isBinaryExpression(node) &&
      node.operatorToken.kind === ts.SyntaxKind.EqualsToken &&
      ts.isIdentifier(node.left)
    ) {
      found.push({ name: node.left.text, value: node.right });
    }
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      node.initializer !== undefined
    ) {
      found.push({ name: node.name.text, value: node.initializer });
    }
    node.forEachChild(walk);
  };
  walk(parsed);
  return found;
}

/**
 * The local names bound to `Intl` or to one of its formatters, by destructure,
 * declaration or assignment. A call through one is judged like a direct call.
 */
export function intlAliases(
  parsed: ts.SourceFile,
  formatters: readonly string[],
): IntlBindings {
  const bound = bindingsOf(parsed);
  // A name bound to a name bound to Intl is Intl too, so repeat to a fixpoint.
  const namespaces = new Set(["Intl"]);
  for (let size = 0; size !== namespaces.size; ) {
    size = namespaces.size;
    for (const { name, value } of bound) {
      if (namesIntl(value, namespaces)) namespaces.add(name);
    }
  }
  const isIntl = (node: ts.Expression) => namesIntl(node, namespaces);
  const aliases = new Set<string>();
  for (const { name, value } of bound) {
    const member = memberOfIntl(value, isIntl);
    if (member !== undefined && formatters.includes(member)) aliases.add(name);
  }
  const walk = (node: ts.Node): void => {
    if (
      ts.isVariableDeclaration(node) &&
      ts.isObjectBindingPattern(node.name) &&
      node.initializer !== undefined &&
      isIntl(node.initializer)
    ) {
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
    node.forEachChild(walk);
  };
  walk(parsed);
  return { namespaces, formatters: aliases };
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
export function formatterReached(
  callee: ts.Expression,
  formatters: readonly string[],
  bindings: IntlBindings,
): string | undefined {
  if (ts.isIdentifier(callee) && bindings.formatters.has(callee.text)) {
    return callee.text;
  }
  const member = memberOfIntl(callee, (candidate) =>
    namesIntl(candidate, bindings.namespaces),
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
    file: "format/formatters.ts",
    why: "pinnedDateTimeFormat builds the en-CA and en-US formatters whose output calendarday.ts and timezone.ts read back as numbers and never show. graphemeSegmenter passes no locale, because Unicode grapheme clusters are the same in every locale.",
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
    "formatMoney. A rendering none of those covers asks src/format/formatters.ts " +
    "for its formatter. A file that must pin a locale earns an entry in " +
    "pinnedLocales with its reason.",
};
