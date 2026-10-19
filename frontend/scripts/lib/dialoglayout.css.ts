// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { rulesIn, withoutComments } from "../../src/testing/css";
import {
  alternativesOf,
  classesOf,
  compounds,
  selectorList,
  splitTopLevel,
  subjectsOf,
} from "./css-rules";

export type Prop =
  | "top"
  | "bottom"
  | "display"
  | "direction"
  | "rowGap"
  | "columnGap";

// A rule's layout with the ancestors it needs: `.a > .b` lays out a `.b` only
// under an `.a`, and a heavier rule there outweighs a bare `.b`.
export type Placed = {
  subject: Set<string>;
  context: Set<string>[];
  child: boolean;
  weight: number;
  order: number;
  place?: string;
  selector: string;
  props: Partial<Record<Prop, string>>;
};

export type Owners = {
  gap: Set<string>;
  band: Set<string>;
  row: Set<string>;
  top: Set<string>;
  placed: Map<string, Placed[]>;
  loose: Set<string>;
  context: Set<string>;
};

export const isZero = (v: string) =>
  /^(0[a-z%]*|none|auto|normal|unset|initial|inherit|revert(-layer)?)$/.test(
    v.replace(/!important/, "").trim(),
  );

const decl = (body: string, prop: string) =>
  body.match(new RegExp(`(?:^|[;\\s])${prop}\\s*:\\s*([^;]+)`))?.[1];

type Edges = { top?: string; bottom?: string };

// `!important` marks the declaration, so every side it fans out to keeps it.
function boxOf(value: string): string[] {
  const important = /!\s*important/.test(value);
  const v = splitTopLevel(value.replace(/!\s*important/, ""), " \t\n");
  return important ? v.map((side) => `${side} !important`) : v;
}

const SIDE =
  /(?:^|[;\s])(margin|padding)(-top|-bottom|-block|-block-start|-block-end)?\s*:\s*([^;]+)/g;

// In source order, so a later shorthand overrides an earlier longhand.
function sides(body: string, box: "margin" | "padding"): Edges {
  const out: Edges = {};
  for (const [, prop, side = "", value] of body.matchAll(SIDE)) {
    if (prop !== box) continue;
    const v = boxOf(value);
    if (["", "-block", "-top", "-block-start"].includes(side)) out.top = v[0];
    if (["-bottom", "-block-end"].includes(side)) out.bottom = v[0];
    if (side === "") out.bottom = v[2] ?? v[0];
    if (side === "-block") out.bottom = v[1] ?? v[0];
  }
  return out;
}

const BORDER =
  /(?:^|[;\s])border-(top|bottom|block-start|block-end)\s*:\s*([^;]+)/g;

function borders(body: string): Edges {
  const out: Edges = {};
  for (const [, side, value] of body.matchAll(BORDER)) {
    out[/top|start/.test(side) ? "top" : "bottom"] = value;
  }
  return out;
}

const GAP = /(?:^|[;\s])(row-gap|column-gap|gap)\s*:\s*([^;]+)/g;

function gaps(body: string) {
  const out: { rowGap?: string; columnGap?: string } = {};
  for (const [, prop, value] of body.matchAll(GAP)) {
    const box = boxOf(value);
    if (prop !== "column-gap") out.rowGap = box[0];
    if (prop !== "row-gap") out.columnGap = box.at(-1);
  }
  return out;
}

function layoutOf(body: string): Placed["props"] {
  const flow = decl(body, "flex-direction") ?? decl(body, "flex-flow");
  const props: Placed["props"] = {
    ...sides(body, "margin"),
    ...gaps(body),
    display: decl(body, "display"),
    direction: flow && (/^\s*column/.test(flow) ? "column" : "row"),
  };
  const set = Object.entries(props).filter(([, v]) => v !== undefined);
  return Object.fromEntries(set);
}

/** How a box with these props lays out its children: a gap `down` between
 * stacked rows, or `across` between ones side by side. */
export function flowOf(props: Placed["props"]) {
  const shown = (props.display ?? "").replace("!important", "");
  const flex = /flex\b/.test(shown);
  const column = props.direction?.trim() === "column";
  const gap = (v?: string) => !isZero(v || "0");
  return {
    row: flex && !column,
    down: (/grid\b/.test(shown) || (flex && column)) && gap(props.rowGap),
    across: flex && !column && gap(props.columnGap),
  };
}

const ID = 1e6;

const CLASS = 1e3;

const TYPE = 1;

const LEGACY_ELEMENT = /^(before|after|first-line|first-letter)$/;

function pseudoWeight(name: string, args: string | undefined): number {
  if (LEGACY_ELEMENT.test(name)) return TYPE;
  if (name === "where") return 0;
  const heaviest = (list: string) =>
    Math.max(0, ...selectorList(list).map(weightOf));
  if (args !== undefined && /^(is|not|has|matches|-webkit-any)$/.test(name)) {
    return heaviest(args);
  }
  const of = args && /^nth-(last-)?(child|of-type)$/.test(name);
  const among = of ? /\bof\s+([\s\S]+)$/.exec(args)?.[1] : undefined;
  return CLASS + (among ? heaviest(among) : 0);
}

function closing(text: string, from: number): number {
  let end = from;
  for (let depth = 1; depth > 0 && end < text.length; end++) {
    if (text[end] === "(") depth++;
    if (text[end] === ")") depth--;
  }
  return end;
}

const SIMPLE =
  /^(?:#(?:[\w-]|\\.)+|\.(?:[\w-]|\\.)+|\[[^\]]*\]|[a-zA-Z_][\w-]*)/;

// One simple selector's length and weight, from `at`.
function tokenAt(selector: string, at: number): [number, number] {
  const rest = selector.slice(at);
  const simple = SIMPLE.exec(rest)?.[0];
  if (simple) {
    const weight = /^[.[]/.test(simple) ? CLASS : TYPE;
    return [simple.length, simple[0] === "#" ? ID : weight];
  }
  const pseudo = /^(::?)([\w-]+)(\()?/.exec(rest);
  if (!pseudo) return [1, 0];
  const end = pseudo[3] ? closing(rest, pseudo[0].length) : pseudo[0].length;
  const args = pseudo[3] ? rest.slice(pseudo[0].length, end - 1) : undefined;
  const element = pseudo[1] === "::";
  return [end, element ? TYPE : pseudoWeight(pseudo[2].toLowerCase(), args)];
}

/** Specificity as one number: ids, then classes, attributes and
 * pseudo-classes, then types and pseudo-elements. */
export function weightOf(selector: string): number {
  let weight = 0;
  for (let at = 0; at < selector.length; ) {
    const [length, w] = tokenAt(selector, at);
    at += length;
    weight += w;
  }
  return weight;
}

// A subject behind a sibling combinator or a state pseudo-class lays out an
// element only sometimes, so it neither sets nor clears anything.
function placedOf(selector: string, body: string, order: number): Placed[] {
  const props = layoutOf(body);
  if (Object.keys(props).length === 0) return [];
  return selectorList(selector).flatMap((written) => {
    const weight = weightOf(written);
    return alternativesOf(written).flatMap((alternative) => {
      const links = compounds(alternative);
      const [subject = "", place] = (links.at(-1) ?? "").split(
        /:(first-child|last-child)$/,
      );
      if (subject.includes(":") || /[+~]/.test(alternative)) return [];
      const context = links.slice(0, -1).map(classesOf);
      const child = />\s*[^\s>]+$/.test(alternative);
      const rule = { subject: classesOf(subject), context, child, weight };
      const at = { order, props, place, selector: alternative };
      const any = subject.trim() === "*";
      return rule.subject.size > 0 || any ? [{ ...rule, ...at }] : [];
    });
  });
}

// A rule under a conditional at-rule holds only on some screens, so a layout
// census counts it as neither setting nor clearing anything.
function unconditional(sheet: string): string {
  const css = withoutComments(sheet);
  let out = css;
  for (const m of css.matchAll(/@(?:media|container|supports)\b[^{]*\{/g)) {
    const start = m.index ?? 0;
    let depth = 1;
    let end = start + m[0].length;
    for (; end < css.length && depth > 0; end++) {
      if (css[end] === "{") depth++;
      if (css[end] === "}") depth--;
    }
    const blank = css.slice(start, end).replace(/[^\n]/g, " ");
    out = out.slice(0, start) + blank + out.slice(end);
  }
  return out;
}

// What a rule's body says about the box it paints, for the title gate's sets.
function traitsOf(body: string) {
  const margin = layoutOf(body);
  const [padding, border] = [sides(body, "padding"), borders(body)];
  const set = (v: string | undefined) => v !== undefined && !isZero(v);
  const flow = flowOf(margin);
  return {
    gap: flow.down,
    band: [margin.bottom, padding.bottom, border.bottom].some(set),
    top: [margin.top, padding.top, border.top].some(set),
    loose: /^(absolute|fixed)/.test(decl(body, "position") ?? ""),
    row: flow.row,
  };
}

const TRAITS: readonly ("gap" | "band" | "top" | "loose" | "row")[] = [
  "gap",
  "band",
  "top",
  "loose",
  "row",
];

export function index(owners: Owners, rule: Placed) {
  for (const name of rule.subject.size > 0 ? rule.subject : ["*"]) {
    owners.placed.set(name, [...(owners.placed.get(name) ?? []), rule]);
  }
  for (const need of rule.context) {
    for (const c of need) owners.context.add(c);
  }
}

export function ownersIn(sheets: readonly string[]): Owners {
  const owners: Owners = {
    gap: new Set(),
    band: new Set(),
    row: new Set(),
    top: new Set(),
    placed: new Map(),
    loose: new Set(),
    context: new Set(),
  };
  const held = sheets.flatMap((sheet) => rulesIn(unconditional(sheet)));
  for (const [order, { selector, body }] of held.entries()) {
    for (const rule of placedOf(selector, body, order)) {
      index(owners, rule);
    }
  }
  for (const { selector, body } of held) {
    const traits = traitsOf(body);
    const names = selectorList(selector)
      .flatMap(subjectsOf)
      .flatMap((subject) => [...classesOf(subject)]);
    for (const trait of TRAITS.filter((t) => traits[t])) {
      for (const name of names) owners[trait].add(name);
    }
  }
  return owners;
}
