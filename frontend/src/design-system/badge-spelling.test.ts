// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  extensionFrontendFiles,
  extensionLayers,
  filesMatching,
  filesUnder,
  parseSource,
  sourceFileAt,
} from "../../scripts/lib/source-tree";
import { type CssRule, rulesIn, withoutComments } from "../testing/css";

// A label in a pill has one spelling, and it is `Badge`.
//
// `atoms.css` draws the pill and `atoms.tsx` is the one element that carries
// its class. The tree grew the other two spellings anyway: a screen sheet that
// reached into `.badge` for a colour or an uppercase label, so the same status
// read one way on one page and another way on the next; and a pill class of a
// screen's own — `.x-tag` with a fill on a `--r-full` corner — that stops
// moving when `Badge` does. Three arms, one per shape of the offence:
//
//   restyle     — a rule outside atoms.css whose selector names `.badge` or a
//                 `.badge-*` class ANYWHERE (the subject, or an ancestor of the
//                 subject: `.badge svg` restyles what the badge draws) and that
//                 declares anything but placement. Placement is an allowlist,
//                 not a list of visual properties, so a property nobody thought
//                 of (`text-shadow`, `opacity`, a custom property the atom reads)
//                 counts as a restyle rather than slipping past.
//   hand-rolled — a class whose name ends a hyphen-bounded `badge`, `pill`,
//                 `lozenge` or `tag` segment (`x-pill`, `x-tag-warning`), whose
//                 rules together lay a fill and a full corner. Together: a pill
//                 split into `.x-pill { border-radius }` and
//                 `.x-pill-warning { background }` is still one pill. Hyphen-bounded
//                 because `filterpill` and `tagpill` are design-system controls
//                 that name themselves as one word, and a chip a reader presses
//                 is not a badge.
//   markup      — a `className` in TSX carrying `badge` or `badge-*`, a class
//                 list assembled in a shipped module that does, or a `style` /
//                 `className` handed to `<Badge>`. The atom's type refuses the
//                 last one already; the gate reads it anyway, because a spread
//                 or a cast gets past a type and not past a syntax tree.
//
// The waiver is `ds:ignore <reason>` in a comment on the declaration's line or
// directly above it, the spelling the other design-system gates taught the
// tree; a marker with no reason is itself a finding. Markup takes no waiver:
// the atom is the only element that may carry the class, and there is no case
// in which a second one draws the same thing better.
//
// Under-recognition is how this gate must not break, so the corpus is derived
// from the tree and fails closed when small, at-rules and nested rules are
// descended into, and the detector carries a planted case of every shape.

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const sourceRoot = join(frontendRoot, "src");
const extensionsRoot = join(frontendRoot, "..", "extensions");

/** The sheet that draws the pill, and the element that carries its class. */
const homeSheet = "src/design-system/atoms.css";
const homeModule = "src/design-system/atoms.tsx";
const tokensSheet = "src/design-system/tokens.css";

function fromFrontend(path: string): string {
  return relative(frontendRoot, path).replaceAll("\\", "/");
}

/** Every stylesheet but the pill's own, core and extension tier alike. */
function sheets(): string[] {
  const units = extensionLayers(extensionsRoot).flatMap((layer) =>
    filesMatching(layer, /\.css$/),
  );
  return filesMatching(sourceRoot, /\.css$/)
    .concat(units)
    .map(fromFrontend)
    .filter((where) => where !== homeSheet);
}

/** Every module but the atom's own. */
function modules(): string[] {
  return filesUnder(sourceRoot)
    .concat(extensionFrontendFiles(extensionsRoot))
    .map(fromFrontend)
    .filter((where) => where !== homeModule);
}

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
 * the words after the marker; a marker with none says a rule was skipped and
 * not why, which is the note the next reader cannot act on.
 */
function waiverIn(text: string): Waiver {
  const marker = /ds:ignore\b([^*]*)/.exec(text);
  if (!marker) {
    return "none";
  }
  return marker[1].trim().length > 0 ? "reasoned" : "bare";
}

/**
 * A rule's OWN declarations, each with its line and its waiver. A nested
 * rule's declarations and its selector are skipped, since they are that
 * rule's. The waiver is read from the comments between this declaration and
 * the line the previous one ended on, and from the rest of this declaration's
 * own line — so a trailing waiver on the declaration above is not this one's.
 */
function ownDeclarations(rule: CssRule): Declaration[] {
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

/** Split at top-level separators, leaving `(…)` and `[…]` whole. */
function splitTopLevel(text: string, isSeparator: RegExp): string[] {
  const parts: string[] = [];
  let depth = 0;
  let from = 0;
  for (let i = 0; i < text.length; i++) {
    if ("([".includes(text[i])) {
      depth++;
    } else if (")]".includes(text[i])) {
      depth--;
    } else if (depth === 0 && isSeparator.test(text[i])) {
      parts.push(text.slice(from, i));
      from = i + 1;
    }
  }
  parts.push(text.slice(from));
  return parts.map((part) => part.trim()).filter(Boolean);
}

/**
 * Every complex selector a rule applies to, its nesting resolved: `&` stands
 * for each parent selector, and a nested selector without one is a
 * descendant of it.
 */
function selectorsOf(rule: CssRule): string[] {
  return [...rule.parents, rule.selector].reduce<string[]>(
    (outer, selector) =>
      splitTopLevel(selector, /,/).flatMap((inner) => {
        if (outer.length === 0) {
          return [inner];
        }
        return outer.map((parent) =>
          inner.includes("&")
            ? inner.replaceAll("&", parent)
            : `${parent} ${inner}`,
        );
      }),
    [],
  );
}

/**
 * The selector with what it does NOT select removed: `:not(.badge)` is every
 * other element and `:has(.badge)` is the element around one, so neither
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

function namesBadge(selector: string): boolean {
  const attributes = withoutConditions(selector).matchAll(
    /\[\s*class\s*[~|^$*]?=\s*["']?([^"'\]]*)/g,
  );
  return (
    classesIn(selector).some((name) => badgeClass.test(name)) ||
    [...attributes].some((match) => /\bbadge/.test(match[1]))
  );
}

/** The rightmost compound: the element the rule actually draws. */
function subjectOf(selector: string): string {
  return splitTopLevel(selector, /[\s>+~]/).at(-1) ?? "";
}

// ─── Arm 1: restyling .badge ───────────────────────────────────────────────

/**
 * What a rule may say about a badge it did not draw: where the pill sits, and
 * how its row gives it room. Everything else is the pill's own look.
 */
const placement =
  /^(margin(-[a-z-]+)?|(align|justify|place)-self|order|flex(-grow|-shrink|-basis)?|grid-(area|column|row)(-start|-end)?|position|inset(-[a-z-]+)?|top|right|bottom|left|z-index|(max|min)-(inline-size|width)|overflow(-[a-z-]+)?|text-overflow|white-space|vertical-align)$/;

type Restyle = { selector: string; declaration: Declaration };

function restyles(rule: CssRule): Restyle[] {
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

type Located = {
  where: string;
  selector: string;
  declaration: Declaration;
};

type Pill = { family: string; fills: Located[]; corners: Located[] };

/** Every pill-named family's fills and full corners, across the corpus. */
function handRolled(
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
 * for `<Badge>` only: a test names the class to assert the contract the atom
 * emits, and an expected value in a table is not an element anybody sees.
 */
function markupIn(source: ts.SourceFile, shipped: boolean): MarkupFinding[] {
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

function fullRungOf(tokens: string): string {
  const rung = /--r-full\s*:\s*([^;]+);/.exec(withoutComments(tokens));
  if (!rung) {
    throw new Error("--r-full is gone from tokens.css");
  }
  return rung[1].trim();
}

function census() {
  const fullRung = fullRungOf(
    readFileSync(join(frontendRoot, tokensSheet), "utf8"),
  );
  const rules = sheets().flatMap((where) =>
    rulesIn(readFileSync(join(frontendRoot, where), "utf8")).map((rule) => ({
      where,
      rule,
    })),
  );
  const restyled = rules.flatMap(({ where, rule }) =>
    restyles(rule)
      .filter(({ declaration }) => declaration.waiver === "none")
      .map((restyle) => ({ where, ...restyle })),
  );
  const bare = rules.flatMap(({ where, rule }) =>
    ownDeclarations(rule)
      .filter((declaration) => declaration.waiver === "bare")
      .map((declaration) => ({ where, selector: rule.selector, declaration })),
  );
  const markup = modules().flatMap((where) =>
    markupIn(
      sourceFileAt(join(frontendRoot, where)),
      !/\.(test|spec)\.[jt]sx?$/.test(where),
    ).map((finding) => `${where}:${finding.line}  ${finding.says}`),
  );
  return { rules, restyled, bare, pills: handRolled(rules, fullRung), markup };
}

function shown({ where, selector, declaration }: Located): string {
  return `${where}:${declaration.line}  ${selector}  { ${declaration.property}: ${declaration.value} }`;
}

describe("a label in a pill has one spelling", () => {
  const found = census();

  it("reads a corpus, an owner and a rung that are not empty", () => {
    expect(sheets().length).toBeGreaterThan(100);
    expect(modules().length).toBeGreaterThan(500);
    expect(found.rules.length).toBeGreaterThan(2000);
    const home = rulesIn(readFileSync(join(frontendRoot, homeSheet), "utf8"));
    expect(
      home.filter((rule) => selectorsOf(rule).some(namesBadge)).length,
    ).toBeGreaterThan(5);
    const atom = markupIn(
      parseSource(
        homeModule,
        readFileSync(join(frontendRoot, homeModule), "utf8"),
      ),
      true,
    );
    expect(
      atom.length,
      "atoms.tsx no longer mints the badge class",
    ).toBeGreaterThan(0);
  });

  it("finds no stylesheet restyling .badge outside atoms.css", () => {
    expect(
      found.restyled.map(shown),
      "each of these rules restyles a badge from outside its home. Use " +
        "<Badge variant tone icon>; a look Badge does not have is a variant " +
        "proposed in atoms.css. A placement-only rule (margin, align-self, " +
        "order, flex, grid, position, max-width, overflow) may stay\n",
    ).toEqual([]);
  });

  it("finds no hand-rolled pill class", () => {
    expect(
      found.pills.map(
        (pill) =>
          `.${pill.family}: fill ${pill.fills.map(shown).join(" | ")} ; corner ${pill.corners.map(shown).join(" | ")}`,
      ),
      "each of these classes draws a filled pill of its own. Use <Badge " +
        "variant tone icon> and delete the fill and the corner; what the rule " +
        "says about placement may stay on a wrapper\n",
    ).toEqual([]);
  });

  it("finds no markup carrying the badge class or restyling Badge", () => {
    expect(
      found.markup,
      "only the Badge atom carries the badge class. Render <Badge variant tone " +
        "icon> instead, and place it from the parent's layout rather than a " +
        "style or a class on the pill\n",
    ).toEqual([]);
  });

  it("finds no waiver without a reason", () => {
    expect(
      found.bare.map(shown),
      "ds:ignore says a declaration was skipped and not why; write the reason after it",
    ).toEqual([]);
  });

  // The detector must be able to SEE each shape, or a clean tree and a broken
  // reader look identical. Planted here rather than trusted.
  describe("the detector", () => {
    const rung = fullRungOf(
      readFileSync(join(frontendRoot, tokensSheet), "utf8"),
    );
    const restyledIn = (css: string) =>
      rulesIn(css)
        .flatMap(restyles)
        .map(
          ({ selector, declaration }) => `${selector} ${declaration.property}`,
        );
    const pillsIn = (css: string) =>
      handRolled(
        rulesIn(css).map((rule) => ({ where: "x.css", rule })),
        rung,
      ).map((pill) => pill.family);
    const markup = (text: string, shipped = true) =>
      markupIn(parseSource("x.tsx", text), shipped).map((f) => f.says);

    it("reads a colour on a descendant badge, and not its margin", () => {
      expect(restyledIn(".x .badge { color: var(--danger); }")).toEqual([
        ".x .badge color",
      ]);
      expect(
        restyledIn(
          ".x .badge { margin-inline-start: var(--space-2); align-self: center; }",
        ),
      ).toEqual([]);
    });

    it("reads a child-combinator tone class and a compound", () => {
      expect(
        restyledIn(".x > .badge-warning { text-transform: uppercase; }"),
      ).toEqual([".x > .badge-warning text-transform"]);
      expect(restyledIn(".badge.x { letter-spacing: 0.04em; }")).toEqual([
        ".badge.x letter-spacing",
      ]);
    });

    it("reads a badge as an ancestor, and a property nobody listed", () => {
      expect(restyledIn(".badge svg { opacity: 0.6; }")).toEqual([
        ".badge svg opacity",
      ]);
      expect(restyledIn(".x .badge { --badge-ink: red; }")).toEqual([
        ".x .badge --badge-ink",
      ]);
    });

    it("reads a badge named through :where() or a class attribute", () => {
      expect(
        restyledIn(
          ':where(.x .badge) { color: red; } .y [class*="badge-"] { color: red; }',
        ),
      ).toEqual([":where(.x .badge) color", '.y [class*="badge-"] color']);
    });

    it("does not read .badges, a :not(.badge), or a :has(.badge)", () => {
      expect(
        restyledIn(
          ".x-badges { color: red; } .x > :not(.badge) { color: red; } .row:has(.badge) { background: red; }",
        ),
      ).toEqual([]);
    });

    it("reads a restyle inside @media, @supports and a nested block", () => {
      expect(
        restyledIn(
          "@media (max-width: 600px) { .x .badge { padding: 0; } }\n" +
            "@supports (display: grid) { .y .badge-ai { font-size: 1em; } }\n" +
            ".z { display: grid; .badge { border: 1px solid; } &.w > .badge-danger { background: red; } }",
        ),
      ).toEqual([
        ".x .badge padding",
        ".y .badge-ai font-size",
        ".z .badge border",
        ".z.w > .badge-danger background",
      ]);
    });

    it("reads one member of a selector list", () => {
      expect(restyledIn(".a, .b .badge, .c { color: red; }")).toEqual([
        ".b .badge color",
      ]);
    });

    it("reads a filled full-corner pill class, and not an unfilled one", () => {
      expect(
        pillsIn(
          ".foo-pill { background: var(--bgChip); border-radius: var(--r-full); }",
        ),
      ).toEqual(["foo-pill"]);
      expect(pillsIn(".foo-pill { border-radius: var(--r-full); }")).toEqual(
        [],
      );
      expect(
        pillsIn(
          ".foo-pill { background: transparent; border-radius: var(--r-full); }",
        ),
      ).toEqual([]);
      expect(
        pillsIn(
          ".foo-tag { background: var(--bgChip); border-radius: var(--r-sm); }",
        ),
      ).toEqual([]);
    });

    it("reads a pill split across its base and a modifier, in a breakpoint", () => {
      expect(
        pillsIn(
          ".lead-tag { border-radius: 50%; }\n@media (min-width: 1px) { .lead-tag-warning { background-color: var(--warningBg); } }",
        ),
      ).toEqual(["lead-tag"]);
      expect(
        pillsIn(
          `.x-lozenge { border-radius: ${rung} ${rung}; background: var(--bgChip); }`,
        ),
      ).toEqual(["x-lozenge"]);
    });

    it("does not read a one-word control name, or a pill that is only an ancestor", () => {
      expect(
        pillsIn(
          ".filterpill { background: var(--bgElevated); border-radius: var(--r-full); }\n" +
            ".x-pill .dot { background: red; border-radius: 50%; }",
        ),
      ).toEqual([]);
    });

    it("reads a waiver on the declaration's line or above it, with a reason", () => {
      const [rule] = rulesIn(
        ".x .badge {\n  /* ds:ignore the host page draws this */\n  color: red;\n" +
          "  margin: 0; /* ds:ignore */\n  padding: 0;\n}",
      );
      expect(
        ownDeclarations(rule).map((d) => [d.property, d.line, d.waiver]),
      ).toEqual([
        ["color", 3, "reasoned"],
        ["margin", 4, "bare"],
        ["padding", 5, "none"],
      ]);
    });

    it("reads the badge class in className, however it is assembled", () => {
      expect(markup('<span className="badge badge-warning">x</span>')).toEqual([
        "<span> carries the badge class",
      ]);
      expect(
        markup('<span className={cx("badge", on && "x")}>x</span>'),
      ).toEqual(["<span> carries the badge class"]);
      // biome-ignore lint/suspicious/noTemplateCurlyInString: a fixture of source code
      expect(markup("<b className={`x badge-${tone}`}>x</b>")).toEqual([
        "<b> carries the badge class",
      ]);
      expect(markup('<span className="x-badges tagpill">x</span>')).toEqual([]);
    });

    it("reads a class list assembled away from the element, in a shipped module only", () => {
      expect(markup('const classes = ["badge", "badge-ai"];')).toHaveLength(2);
      expect(markup('expect(el).toHaveClass("badge-ai");', false)).toEqual([]);
      expect(markup('const words = "A badge is a label.";')).toEqual([]);
    });

    it("reads a style or a class handed to Badge, under an alias too", () => {
      expect(markup("<Badge style={{ color: 'red' }}>x</Badge>")).toEqual([
        "<Badge> is handed style",
      ]);
      expect(
        markup(
          'import { Badge as Pill } from "../design-system/atoms";\n<Pill {...{ className: "x" }}>x</Pill>',
        ),
      ).toEqual(["<Badge> is handed {...{ className }}"]);
      expect(markup('<Badge tone="warning">x</Badge>')).toEqual([]);
    });
  });
});
