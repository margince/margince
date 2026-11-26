// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A label in a pill has one spelling, and it is `Badge`.
//
// badge.css draws the pill. atoms.tsx is the one element that carries its
// class, and mcp-apps/badge.ts is the views' one badge builder, since those
// documents carry no React. Three arms, one per shape of the offence:
//
//   restyle: a rule outside badge.css whose selector names `.badge` or a
//     `.badge-*` class anywhere, and declares anything but placement.
//     Placement is an allowlist, so a property nobody listed counts.
//   hand-rolled: a class ending a hyphen-bounded `badge`, `pill`, `lozenge`
//     or `tag` segment whose rules together lay a fill and a full corner.
//     `filterpill` and `tagpill` are one-word controls a reader presses.
//   markup: a `className` carrying `badge` or `badge-*`, a class list
//     assembled in a shipped module, or a `style` or `className` handed to
//     `<Badge>`. A spread or a cast gets past a type, not past a syntax tree.
//
// The waiver is `ds:ignore <reason>` on the declaration's line or above it; a
// marker with no reason is a finding. Markup takes no waiver.

import { join } from "node:path";
import ts from "typescript";
import { resolveNesting, subjectOf } from "../../scripts/lib/css-rules";
import { parseSource } from "../../scripts/lib/source-tree";
import { type CssRule, rulesIn, withoutComments } from "../../src/testing/css";
import { type CensusFile, type CensusRule, frontendRoot } from "../tree";

/** The sheet that draws the pill, and the elements that carry its class. */
const homeSheet = "src/design-system/badge.css";
const homeModule = "src/design-system/atoms.tsx";
const viewBuilder = "src/mcp-apps/badge.ts";
const tokensSheet = "src/design-system/tokens.css";

// ─── Declarations ──────────────────────────────────────────────────────────

type Waiver = "none" | "reasoned" | "bare";

type Declaration = {
  property: string;
  value: string;
  line: number;
  waiver: Waiver;
};

/**
 * The marker in the comment text that sits with one declaration. A reason is
 * the words after the marker. A marker with none says a rule is skipped and
 * not why, and the next reader cannot act on that note.
 */
function waiverIn(text: string): Waiver {
  const marker = /ds:ignore\b([^*]*)/.exec(text);
  if (!marker) {
    return "none";
  }
  return marker[1].trim().length > 0 ? "reasoned" : "bare";
}

/**
 * A rule's own declarations, each with its line and its waiver. A nested
 * rule's declarations are that rule's. The waiver is read from the comments
 * after the line the previous declaration ended on, and from the rest of this
 * declaration's line. A trailing waiver on the declaration above is not this one's.
 */
export function ownDeclarations(rule: CssRule): Declaration[] {
  const scan = withoutComments(rule.raw);
  const out: Declaration[] = [];
  let depth = 0;
  let start: number | null = 0;
  const close = (end: number) => {
    if (start !== null) {
      const declaration = declarationAt(rule, scan, start, end);
      if (declaration) {
        out.push(declaration);
      }
    }
    start = end + 1;
  };
  for (let i = 0; i < scan.length; i++) {
    if (scan[i] === "{") {
      start = depth === 0 ? null : start;
      depth++;
    } else if (scan[i] === "}") {
      depth--;
      start = depth === 0 ? i + 1 : start;
    } else if (scan[i] === ";" && depth === 0) {
      close(i);
    }
  }
  close(scan.length);
  return out;
}

function declarationAt(
  rule: CssRule,
  scan: string,
  start: number,
  end: number,
): Declaration | undefined {
  const text = scan.slice(start, end);
  const colon = text.indexOf(":");
  const property = text.slice(0, colon).trim().toLowerCase();
  if (colon < 0 || !/^-{0,2}[a-z][\w-]*$/.test(property)) {
    return undefined;
  }
  const at = start + text.search(/\S/);
  const before = rule.raw.slice(start, at);
  const newline = before.indexOf("\n");
  const lineEnd = rule.raw.indexOf("\n", end);
  return {
    property,
    value: text
      .slice(colon + 1)
      .trim()
      .replaceAll(/\s+/g, " "),
    line: rule.line + (rule.raw.slice(0, at).match(/\n/g)?.length ?? 0),
    waiver: waiverIn(
      (newline < 0 ? before : before.slice(newline + 1)) +
        rule.raw.slice(end, lineEnd < 0 ? undefined : lineEnd),
    ),
  };
}

// ─── Selectors ─────────────────────────────────────────────────────────────

/**
 * Every complex selector a rule applies to, its nesting resolved. `&` stands
 * for each parent selector, and a nested selector without one is a
 * descendant of it.
 */
export function selectorsOf(rule: CssRule): string[] {
  return [...rule.parents, rule.selector].reduce<string[]>(
    (parents, selector) => resolveNesting(parents, selector),
    [],
  );
}

/**
 * The selector with what it does not select removed. `:not(.badge)` is every
 * other element and `:has(.badge)` is the element around one. So neither
 * names the badge as what the rule draws.
 */
function withoutConditions(selector: string): string {
  let out = selector;
  for (let at = out.search(/:(not|has)\(/); at >= 0; ) {
    let depth = 0;
    let end = out.indexOf("(", at);
    for (; end < out.length; end++) {
      depth += out[end] === "(" ? 1 : out[end] === ")" ? -1 : 0;
      if (depth === 0) {
        break;
      }
    }
    out = out.slice(0, at) + out.slice(end + 1);
    at = out.search(/:(not|has)\(/);
  }
  return out;
}

/** The class names a selector (or a compound of one) is keyed on. */
function classesIn(selector: string): string[] {
  const bare = withoutConditions(selector)
    .replaceAll(/"[^"]*"|'[^']*'/g, '""')
    .replaceAll(/\[[^\]]*\]/g, "");
  return [...bare.matchAll(/\.(-?[_a-zA-Z][\w-]*)/g)].map((match) => match[1]);
}

/** The badge's own class, and every class in its namespace. */
const badgeClass = /^badge(?:-|$)/;

export function namesBadge(selector: string): boolean {
  const attributes = withoutConditions(selector).matchAll(
    /\[\s*class\s*[~|^$*]?=\s*["']?([^"'\]]*)/g,
  );
  return (
    classesIn(selector).some((name) => badgeClass.test(name)) ||
    [...attributes].some((match) => /\bbadge/.test(match[1]))
  );
}

// ─── Arm 1: restyling .badge ───────────────────────────────────────────────

/**
 * What a rule may say about a badge it did not draw: where the pill sits, and
 * how its row gives it room. Everything else is the pill's own look.
 */
const placement =
  /^(margin(-[a-z-]+)?|(align|justify|place)-self|order|flex(-grow|-shrink|-basis)?|grid-(area|column|row)(-start|-end)?|position|inset(-[a-z-]+)?|top|right|bottom|left|z-index|(max|min)-(inline-size|width)|overflow(-[a-z-]+)?|text-overflow|white-space|vertical-align)$/;

type Restyle = { selector: string; declaration: Declaration };

export function restyles(rule: CssRule): Restyle[] {
  const selector = selectorsOf(rule).find(namesBadge);
  if (selector === undefined) {
    return [];
  }
  return ownDeclarations(rule)
    .filter((declaration) => !placement.test(declaration.property))
    .map((declaration) => ({ selector, declaration }));
}

// ─── Arm 2: a hand-rolled pill ─────────────────────────────────────────────

/** A class named for a pill: a hyphen-bounded noun, and the family it heads. */
const pillNoun = /(?:^|-)(?:badge|pill|lozenge|tag)(?=-|$)/i;

function pillFamilies(compound: string): string[] {
  return classesIn(compound).flatMap((name) => {
    const noun = pillNoun.exec(name);
    return noun ? [name.slice(0, noun.index + noun[0].length)] : [];
  });
}

const noFill = /^(none|transparent|inherit|initial|unset|revert(-layer)?)$/;

function fills(declaration: Declaration): boolean {
  return (
    /^background(-color|-image)?$/.test(declaration.property) &&
    !noFill.test(declaration.value)
  );
}

/**
 * A corner a pill has: the ladder's full rung however it is spelled, or half
 * the box. Every corner of the shorthand, because a pill rounds all four.
 * The rung's literal value is read from tokens.css, not restated here.
 */
function roundsFully(declaration: Declaration, fullRung: string): boolean {
  if (declaration.property !== "border-radius") {
    return false;
  }
  return declaration.value
    .split(/[\s/]+/)
    .every(
      (corner) =>
        corner.includes("--r-full") || corner === fullRung || corner === "50%",
    );
}

export type Located = {
  where: string;
  selector: string;
  declaration: Declaration;
};

type Pill = { family: string; fills: Located[]; corners: Located[] };

/** Every pill-named family's fills and full corners, across the corpus. */
export function handRolled(
  rules: { where: string; rule: CssRule }[],
  fullRung: string,
): Pill[] {
  const families = new Map<string, Pill>();
  for (const { where, rule } of rules) {
    const own = ownDeclarations(rule).filter((d) => d.waiver === "none");
    for (const selector of selectorsOf(rule)) {
      for (const family of pillFamilies(subjectOf(selector))) {
        const pill = families.get(family) ?? { family, fills: [], corners: [] };
        families.set(family, pill);
        for (const declaration of own) {
          const located = { where, selector, declaration };
          if (fills(declaration)) {
            pill.fills.push(located);
          } else if (roundsFully(declaration, fullRung)) {
            pill.corners.push(located);
          }
        }
      }
    }
  }
  return [...families.values()].filter(
    (pill) => pill.fills.length > 0 && pill.corners.length > 0,
  );
}

// ─── Arm 3: markup ─────────────────────────────────────────────────────────

/** The literal text a node can evaluate to: strings and template pieces. */
function fragmentsOf(node: ts.Node): string[] {
  const out: string[] = [];
  const collect = (child: ts.Node) => {
    if (ts.isStringLiteral(child) || ts.isTemplateLiteralToken(child)) {
      out.push(child.text);
    }
    ts.forEachChild(child, collect);
  };
  collect(node);
  return out;
}

function carriesBadgeClass(fragment: string): boolean {
  return fragment.split(/\s+/).some((token) => badgeClass.test(token));
}

/** A literal that reads as a class list rather than as words for a reader. */
function isClassList(fragment: string): boolean {
  return /^\s*[a-z][\w-]*(\s+[a-z][\w-]*)*\s*$/.test(fragment);
}

/** The local names `Badge` goes by in a module: its own, and any alias. */
function badgeNames(source: ts.SourceFile): Set<string> {
  const names = new Set(["Badge"]);
  for (const statement of source.statements) {
    const bindings =
      ts.isImportDeclaration(statement) &&
      statement.importClause?.namedBindings;
    if (bindings && ts.isNamedImports(bindings)) {
      for (const element of bindings.elements) {
        if ((element.propertyName ?? element.name).text === "Badge") {
          names.add(element.name.text);
        }
      }
    }
  }
  return names;
}

function isBadgeTag(tag: ts.JsxTagNameExpression, names: Set<string>): boolean {
  if (ts.isIdentifier(tag)) {
    return names.has(tag.text);
  }
  return ts.isPropertyAccessExpression(tag) && tag.name.text === "Badge";
}

/** `style` or `className` handed to Badge, as an attribute or a literal spread. */
function styledBadge(attributes: ts.JsxAttributes): string | undefined {
  for (const property of attributes.properties) {
    if (ts.isJsxAttribute(property)) {
      const name = property.name.getText();
      if (name === "style" || name === "className") {
        return name;
      }
    } else if (ts.isObjectLiteralExpression(property.expression)) {
      const named = property.expression.properties.find((member) =>
        /^(style|className)$/.test(member.name?.getText() ?? ""),
      );
      if (named?.name) {
        return `{...{ ${named.name.getText()} }}`;
      }
    }
  }
  return undefined;
}

type MarkupFinding = { line: number; says: string };

/**
 * The markup arm over one module. A test module is read for `className` and
 * for `<Badge>` only. A test names the class to assert the contract the atom
 * emits. An expected value in a table is not an element anybody sees.
 */
export function markupIn(
  source: ts.SourceFile,
  shipped: boolean,
): MarkupFinding[] {
  const names = badgeNames(source);
  const out: MarkupFinding[] = [];
  const at = (node: ts.Node) =>
    source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1;
  const visit = (node: ts.Node): void => {
    if (ts.isJsxAttribute(node) && node.name.getText() === "className") {
      if (
        node.initializer &&
        fragmentsOf(node.initializer).some(carriesBadgeClass)
      ) {
        out.push({
          line: at(node),
          says: `<${node.parent.parent.tagName.getText()}> carries the badge class`,
        });
      }
      return;
    }
    if (
      (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) &&
      isBadgeTag(node.tagName, names)
    ) {
      const handed = styledBadge(node.attributes);
      if (handed) {
        out.push({ line: at(node), says: `<Badge> is handed ${handed}` });
      }
    }
    if (
      shipped &&
      (ts.isStringLiteral(node) || ts.isTemplateLiteralToken(node)) &&
      isClassList(node.text) &&
      carriesBadgeClass(node.text)
    ) {
      out.push({
        line: at(node),
        says: `"${node.text}" assembles the badge class`,
      });
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return out;
}

// ─── The census ────────────────────────────────────────────────────────────

export function fullRungOf(tokens: string): string {
  const rung = /--r-full\s*:\s*([^;]+);/.exec(withoutComments(tokens));
  if (!rung) {
    throw new Error("--r-full is gone from tokens.css");
  }
  return rung[1].trim();
}

const inLayers = (file: CensusFile) =>
  file.area === "src" || file.area === "extensions";

/** Every stylesheet but the pill's own, core and extension tier alike. */
const sheetsOf = (files: readonly CensusFile[]) =>
  files.filter(
    (file) =>
      inLayers(file) && file.rel.endsWith(".css") && file.rel !== homeSheet,
  );

const rulesOf = (sheets: readonly CensusFile[]) =>
  sheets.flatMap((file) =>
    rulesIn(file.text).map((rule) => ({ where: file.rel, rule })),
  );

/** The markup arm over one module, as finding lines. */
const markupOf = (file: CensusFile) =>
  file.source
    ? markupIn(file.source, !/\.(test|spec)\.[jt]sx?$/.test(file.rel)).map(
        (finding) => `${file.rel}:${finding.line}  ${finding.says}`,
      )
    : [];

function shown({ where, selector, declaration }: Located): string {
  return `${where}:${declaration.line}  ${selector}  { ${declaration.property}: ${declaration.value} }`;
}

const textAt = (
  tree: { at(path: string): CensusFile | undefined },
  rel: string,
) => tree.at(join(frontendRoot, rel))?.text ?? "";

const mints = (rel: string, text: string) =>
  markupIn(parseSource(rel, text), true).length > 0;

export const rule: CensusRule = {
  name: "badge-spelling",
  // Every module but the atom's own and the views' badge builder.
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        inLayers(file) &&
        file.source !== undefined &&
        file.rel !== homeModule &&
        file.rel !== viewBuilder,
    ),
  floors: (corpus, tree) => {
    const sheets = sheetsOf(tree.files);
    const home = rulesIn(textAt(tree, homeSheet));
    return [
      ...(sheets.length > 100 ? [] : ["fewer than 100 stylesheets were read"]),
      ...(corpus.length > 500 ? [] : ["fewer than 500 modules were read"]),
      ...(rulesOf(sheets).length > 2000
        ? []
        : ["fewer than 2000 CSS rules were read"]),
      ...(home.filter((one) => selectorsOf(one).some(namesBadge)).length > 5
        ? []
        : ["badge.css names the badge in five rules or fewer"]),
      ...(mints(homeModule, textAt(tree, homeModule))
        ? []
        : ["atoms.tsx no longer mints the badge class"]),
      ...(mints(viewBuilder, textAt(tree, viewBuilder))
        ? []
        : [
            "mcp-apps/badge.ts no longer mints the badge class, so its exemption is stale",
          ]),
    ];
  },
  across: (corpus, tree) => {
    const rules = rulesOf(sheetsOf(tree.files));
    const fullRung = fullRungOf(textAt(tree, tokensSheet));
    const restyled = rules.flatMap(({ where, rule: one }) =>
      restyles(one)
        .filter(({ declaration }) => declaration.waiver === "none")
        .map((restyle) => `restyle: ${shown({ where, ...restyle })}`),
    );
    const pills = handRolled(rules, fullRung).map(
      (pill) =>
        `hand-rolled: .${pill.family}: fill ${pill.fills.map(shown).join(" | ")} ; corner ${pill.corners.map(shown).join(" | ")}`,
    );
    const bare = rules.flatMap(({ where, rule: one }) =>
      ownDeclarations(one)
        .filter((declaration) => declaration.waiver === "bare")
        .map(
          (declaration) =>
            `bare waiver: ${shown({ where, selector: one.selector, declaration })}`,
        ),
    );
    const markup = corpus.flatMap(markupOf).map((line) => `markup: ${line}`);
    return [...restyled, ...pills, ...markup, ...bare];
  },
  advice:
    "restyle: use <Badge variant tone icon>; a look Badge does not have is a " +
    "variant proposed in badge.css. A placement-only rule (margin, align-self, " +
    "order, flex, grid, position, max-width, overflow) may stay. " +
    "hand-rolled: use <Badge variant tone icon> and delete the fill and the " +
    "corner; placement may stay on a wrapper. " +
    "markup: only the Badge atom carries the badge class; place it from the " +
    "parent's layout rather than a style or a class on the pill. " +
    "bare waiver: ds:ignore says a declaration is skipped and not why; write the reason after it.",
};
