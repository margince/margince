// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The nesting ladder has one painter: surfaces.css. A rung is named and a
// member is painted nowhere else, apart from the lines surfaces.ledger.txt
// keeps. Every class the ladder lists is one some other sheet styles.

import { readFileSync } from "node:fs";
import { join, relative } from "node:path";
import ts from "typescript";
import {
  alternativesOf,
  classesOf,
  compounds,
  type Rule,
  readsEveryCombination,
  rulesOf,
  specificity,
  splitTopLevel,
} from "../../scripts/lib/css-rules";
import { mayHoldLiteral } from "../../scripts/lib/literal-text";
import { parseSource } from "../../scripts/lib/source-tree";
import {
  OVERLAY_RUNG,
  PAGE_RUNGS,
  PINNED_RUNGS,
} from "../../scripts/lib/surface-ladder";
import {
  ALIAS,
  type Declaration,
  declarations,
  flatFill,
  frontendRoot,
  here,
  IMAGE,
  ladderPath,
  lists,
  MEMBERS,
  OPAQUE,
  PSEUDO_ELEMENT,
  VAR,
  type Written,
  within,
  written,
} from "../../scripts/lib/surface-sheets";
import { blocks, resolve } from "../../src/design-system/tokens-testing";
import { byCodeUnit } from "../../src/lib/codeunitorder";
import type { CensusFile, CensusRule } from "../tree";
import { sheetRules } from "../tree";

// A rule outside surfaces.css that names a rung or paints a member overrules
// the ladder. The ledger lists the ones still standing; the ceiling only falls.
const LEDGER_CEILING = { spellings: 76, states: 6 };

const ledgerPath = join(here, "surfaces.ledger.txt");
export const tokensPath = join(here, "tokens.css");
const TOKEN_SHEETS = new Set([tokensPath, join(here, "brand.css")]);

const oneLine = (selector: string) => selector.replace(/\s+/g, " ");

// The selectors tokens.css states its palette under: rules that name no
// element and hold tokens only.
export const THEME_ROOTS = new Set(
  rulesOf([tokensPath])
    .filter(
      (rule) =>
        !rule.selector.includes("*") &&
        compounds(rule.selector).every((part) => classesOf(part).size === 0) &&
        declarations(rule.body).every(({ property }) =>
          property.startsWith("--"),
        ),
    )
    .map((rule) => oneLine(rule.selector)),
);

function definesToken(rule: Rule, property: string): boolean {
  return (
    property.startsWith("--") &&
    TOKEN_SHEETS.has(rule.file) &&
    THEME_ROOTS.has(oneLine(rule.selector))
  );
}

export const tokenDefinitions = rulesOf([...TOKEN_SHEETS]).flatMap((rule) =>
  declarations(rule.body).filter(({ property }) =>
    definesToken(rule, property),
  ),
);

// The rungs and every alias of one, either way and in either theme: --paneEdge
// picks level 1 as surely as --ladder1Edge does. A mix derives a new colour.
export function rungTokens(definitions: readonly Declaration[]): Set<string> {
  const rungs = new Set(
    [...PAGE_RUNGS, OVERLAY_RUNG].flatMap((rung) =>
      [rung.ground, rung.paint, rung.edge].flatMap((value) =>
        [...value.matchAll(VAR)].map((match) => match[1]),
      ),
    ),
  );
  for (let size = 0; size !== rungs.size; ) {
    size = rungs.size;
    for (const { property, value } of definitions) {
      const alias = ALIAS.exec(value)?.[1];
      if (alias === undefined) continue;
      if (rungs.has(alias)) rungs.add(property);
      if (rungs.has(property)) rungs.add(alias);
    }
  }
  return rungs;
}

export const RUNGS = rungTokens(tokenDefinitions);
const NAMES_A_RUNG = new RegExp(
  `--(?:${[...RUNGS].map((name) => name.slice(2)).join("|")})(?![\\w-])`,
);

const spelling = (rule: Rule, { property, value }: Declaration) =>
  `${relative(frontendRoot, rule.file)}: ${oneLine(rule.selector)} { ${property}: ${value} }`;

export function cssSpellings(rules: readonly Rule[]): string[] {
  return rules.flatMap((rule) => {
    if (rule.file === ladderPath) return [];
    return declarations(rule.body)
      .filter(({ value }) => NAMES_A_RUNG.test(value))
      .filter(({ property }) => !definesToken(rule, property))
      .map((declaration) => spelling(rule, declaration));
  });
}

const SPELLS_A_RUNG = new RegExp(`${NAMES_A_RUNG.source}|var\\(`);

export function scriptSpellings(path: string, text: string): string[] {
  if (!mayHoldLiteral(text, SPELLS_A_RUNG)) return [];
  const found: string[] = [];
  const walk = (node: ts.Node) => {
    const part =
      ts.isStringLiteralLike(node) || ts.isTemplateLiteralToken(node)
        ? node.text
        : undefined;
    if (
      part !== undefined &&
      (NAMES_A_RUNG.test(part) ||
        (!ts.isTemplateTail(node) && /var\((?:--)?$/.test(part)))
    ) {
      found.push(`${relative(frontendRoot, path)}: "${part}"`);
    }
    ts.forEachChild(node, walk);
  };
  walk(parseSource(path, text));
  return found;
}

const STATE_LINE = "state: ";

function ledger(): { spellings: string[]; states: string[] } {
  const lines = readFileSync(ledgerPath, "utf8")
    .split("\n")
    .filter((line) => line.trim() !== "" && !line.startsWith("#"));
  return {
    spellings: lines.filter((line) => !line.startsWith(STATE_LINE)),
    states: lines
      .filter((line) => line.startsWith(STATE_LINE))
      .map((line) => line.slice(STATE_LINE.length).split(" | ")[0] ?? ""),
  };
}

// A rule touches a member when its subject may be that element: it names the
// member's class, or a classless member's element in its context.
function touches(rule: Written, member: Written): boolean {
  if (member.own.size === 0) {
    return rule.type === member.type && within(member.context, rule.context);
  }
  return (
    [...rule.own].some((name) => member.own.has(name)) &&
    (within(rule.own, member.own) || within(member.own, rule.own))
  );
}

const GROUND_FILLS = new Set([
  "var(--ground)",
  "var(--groundHover)",
  "var(--groundStrip)",
  "transparent",
  "none",
]);

function seeThrough(colour: string): boolean {
  return Object.values(blocks).every((palette) => {
    try {
      return !OPAQUE.test(resolve(colour, palette, new Set()));
    } catch {
      return false;
    }
  });
}

// An image layer over a ground is a gradient, or a flat tint the ground shows
// through.
function keepsTheGround(layer: string): boolean {
  if (!IMAGE.test(layer)) return true;
  const fill = flatFill(layer);
  return fill === undefined || GROUND_FILLS.has(fill) || seeThrough(fill);
}

type Paint = { allowed: boolean; resetsColour: boolean };

function paintOf({ property, value }: Declaration): Paint | undefined {
  if (!/^background(?:-color|-image)?$/.test(property)) return undefined;
  const layers = splitTopLevel(value, ",");
  if (property === "background-color") {
    return { allowed: GROUND_FILLS.has(value), resetsColour: true };
  }
  const images = layers.every(keepsTheGround);
  if (property === "background-image") {
    return { allowed: images, resetsColour: false };
  }
  const colour =
    splitTopLevel(layers.at(-1) ?? "", " ").find(
      (word) =>
        !IMAGE.test(word) &&
        /^(?:var|color-mix|rgba?|hsla?)\(|^#|^transparent$/.test(word),
    ) ?? "transparent";
  return { allowed: images && GROUND_FILLS.has(colour), resetsColour: true };
}

type MemberPaint = Paint & {
  line: string;
  file: string;
  order: number;
  weight: number;
  written: Written;
};

function paintsIn(rules: readonly Rule[]): MemberPaint[] {
  return rules.flatMap((rule, order) => {
    if (rule.file === ladderPath) return [];
    return declarations(rule.body).flatMap((declaration) => {
      const paint = paintOf(declaration);
      if (paint === undefined) return [];
      return alternativesOf(rule.selector)
        .filter(
          (alternative) =>
            !PSEUDO_ELEMENT.test(compounds(alternative).at(-1) ?? ""),
        )
        .map((alternative) => ({
          ...paint,
          line: spelling(rule, declaration),
          file: rule.file,
          order,
          weight: specificity(alternative),
          written: written(alternative),
        }));
    });
  });
}

function covers(
  other: MemberPaint,
  paint: MemberPaint,
  member: Written,
): boolean {
  const outranks =
    other.weight > paint.weight ||
    (other.weight === paint.weight &&
      other.file === paint.file &&
      other.order > paint.order);
  return (
    other.allowed &&
    other.resetsColour &&
    outranks &&
    within(
      other.written.ownSimples,
      paint.written.ownSimples,
      member.ownSimples,
    ) &&
    within(
      other.written.contextSimples,
      paint.written.contextSimples,
      member.contextSimples,
    )
  );
}

export function memberPaints(
  rules: readonly Rule[],
  members: readonly Written[],
): string[] {
  const paints = paintsIn(rules);
  const found = members.flatMap((member) => {
    const on = paints.filter((paint) => touches(paint.written, member));
    return on
      .filter(
        (paint) =>
          !paint.allowed && !on.some((other) => covers(other, paint, member)),
      )
      .map((paint) => paint.line);
  });
  return [...new Set(found)].sort(byCodeUnit);
}

/** Every ladder class no sheet but surfaces.css styles. */
export function unstyledLadderClasses(rules: readonly Rule[]): string[] {
  const styled = new Set(
    rules
      .filter((one) => one.file !== ladderPath)
      .flatMap((one) =>
        compounds(one.selector).flatMap((part) => [...classesOf(part)]),
      ),
  );
  return [
    ...lists.surfaces,
    ...lists.overlays,
    ...PINNED_RUNGS.map((pin) => pin.selector),
  ]
    .flatMap((selector) =>
      compounds(selector).flatMap((part) => [...classesOf(part)]),
    )
    .filter((name) => !styled.has(name));
}

/** A ledger held against what the tree finds: new lines, and stale ones. */
function againstLedger(
  found: readonly string[],
  kept: readonly string[],
  ceiling: number,
  what: string,
): string[] {
  return [
    ...(kept.length <= ceiling
      ? []
      : [
          `the ledger keeps ${kept.length} ${what}, past its ceiling of ${ceiling}`,
        ]),
    ...found
      .filter((line) => !kept.includes(line))
      .map((line) => `${line}: not in the ledger`),
    ...kept
      .filter((line) => !found.includes(line))
      .map((line) => `${line}: in the ledger, no longer found`),
  ];
}

const isSheet = (file: CensusFile) => file.rel.endsWith(".css");
// Tests plant spellings to probe gates; stories draw tokens as specimens.
const isScript = (file: CensusFile) =>
  /\.tsx?$/.test(file.rel) && !/\.(test|stories)\.tsx?$/.test(file.rel);

export const rule: CensusRule = {
  name: "surfaces",
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        (file.area === "src" || file.area === "extensions") &&
        (isSheet(file) || isScript(file)),
    ),
  floors: (corpus) => {
    const sheets = corpus.filter(isSheet);
    return [
      ...(sheets.length > 250 ? [] : ["fewer than 250 stylesheets were read"]),
      ...["recordview.css", "conversation.css", "view.css"]
        .filter((name) => !sheets.some((file) => file.rel.endsWith(name)))
        .map((name) => `${name} was not read`),
      ...(corpus.filter(isScript).length > 500
        ? []
        : ["fewer than 500 scripts were read"]),
      ...(sheetRules(sheets).some((one) => !readsEveryCombination(one.selector))
        ? []
        : ["no selector past the cap was read"]),
    ];
  },
  across: (corpus) => {
    const all = sheetRules(corpus.filter(isSheet));
    const kept = ledger();
    const spellings = [
      ...cssSpellings(all),
      ...corpus
        .filter(isScript)
        .flatMap((file) => scriptSpellings(file.path, file.text)),
    ].sort(byCodeUnit);
    return [
      ...all
        .filter(
          (one) =>
            one.file !== ladderPath && !readsEveryCombination(one.selector),
        )
        .map(
          (one) =>
            `${relative(frontendRoot, one.file)}: ${oneLine(one.selector)} is past the cap and read one alternative at a time`,
        ),
      ...againstLedger(
        spellings,
        kept.spellings,
        LEDGER_CEILING.spellings,
        "spellings",
      ),
      ...againstLedger(
        memberPaints(all, MEMBERS),
        kept.states,
        LEDGER_CEILING.states,
        "states",
      ),
      ...unstyledLadderClasses(all).map(
        (name) => `.${name} is on the ladder and no other sheet styles it`,
      ),
    ];
  },
  advice:
    "Paint a pane by joining the ladder (node scripts/surface-ladder.ts " +
    "<selector>); a band or mark that must match its host reads var(--ground) " +
    "or var(--groundEdge), and a tint goes on as background-image over it. " +
    "Remove the ledger lines that no longer match.",
};
