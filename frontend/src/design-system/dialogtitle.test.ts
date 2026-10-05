// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { existsSync, readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  alternativesOf,
  classesOf,
  compounds,
  selectorList,
  splitTopLevel,
  subjectsOf,
} from "../../scripts/lib/css-rules";
import {
  extensionLayers,
  filesMatching,
  parseSource,
  resolveRelative,
  sourceFileAt,
} from "../../scripts/lib/source-tree";
import { rulesIn } from "../testing/css";

const srcDir = join(dirname(fileURLToPath(import.meta.url)), "..");
const extensionsDir = join(srcDir, "..", "..", "extensions");
const underTree = (pattern: RegExp) =>
  filesMatching(srcDir, pattern).concat(
    extensionLayers(extensionsDir).flatMap((l) => filesMatching(l, pattern)),
  );

const EXCEPTIONS = [
  {
    file: "screens/onboarding-conversation/connect-dialog.tsx",
    classes: "ob-connect-dialog-title",
    reason: "the intro and body under it set their own margin-top",
  },
  {
    file: "design-system/modal.stories.tsx",
    classes: "t-h2",
    reason: "a title and its subtitle pair tight inside the band",
  },
];
const BOTTOM =
  /(?:^|[;\s])(?:(?:margin|padding)-(?:bottom|block-end)|border-bottom)\s*:([^;]+)/g;
const TOP =
  /(?:^|[;\s])(?:(?:margin|padding)-(?:top|block-start)|border-top)\s*:([^;]+)/g;

type Owners = {
  gap: Set<string>;
  band: Set<string>;
  row: Set<string>;
  top: Set<string>;
  placed: Map<string, Placed[]>;
  loose: Set<string>;
};
type Prop = "top" | "bottom" | "display" | "direction" | "rowGap" | "columnGap";
// A rule's layout with the ancestors it needs: `.a > .b` lays out a `.b` only
// under an `.a`, and a heavier rule there outweighs a bare `.b`.
type Placed = {
  subject: Set<string>;
  context: Set<string>[];
  child: boolean;
  weight: number;
  order: number;
  place?: string;
  selector: string;
  props: Partial<Record<Prop, string>>;
};
type Title = { where: string; classes: string[]; owner: string | null };

const isZero = (v: string) =>
  /^(0[a-z%]*|none|auto|normal|unset|initial|inherit|revert(-layer)?)$/.test(
    v.replace(/!important/, "").trim(),
  );
const decl = (body: string, prop: string) =>
  body.match(new RegExp(`(?:^|[;\\s])${prop}\\s*:\\s*([^;]+)`))?.[1];
const display = (body: string, kind: string) =>
  new RegExp(`(^|[;\\s])display\\s*:\\s*(inline-)?${kind}\\b`).test(body);

const MARGIN =
  /(?:^|[;\s])margin(-top|-bottom|-block|-block-start|-block-end)?\s*:\s*([^;]+)/g;
function edges(body: string) {
  const out: { top?: string; bottom?: string } = {};
  for (const [, side = "", value] of body.matchAll(MARGIN)) {
    const box = splitTopLevel(value, " \t\n");
    const block = side === "-block";
    if (["", "-block", "-top", "-block-start"].includes(side)) out.top = box[0];
    if (["-bottom", "-block-end"].includes(side)) out.bottom = box[0];
    if (side === "") out.bottom = box[2] ?? box[0];
    if (block) out.bottom = box[1] ?? box[0];
  }
  return out;
}
function layoutOf(body: string): Placed["props"] {
  const gap = splitTopLevel(decl(body, "gap") ?? "", " \t\n");
  const flow = decl(body, "flex-direction") ?? decl(body, "flex-flow");
  const props: Placed["props"] = {
    ...edges(body),
    display: decl(body, "display"),
    direction: flow && (/^\s*column/.test(flow) ? "column" : "row"),
    rowGap: decl(body, "row-gap") ?? gap[0],
    columnGap: decl(body, "column-gap") ?? gap.at(-1),
  };
  const set = Object.entries(props).filter(([, v]) => v !== undefined);
  return Object.fromEntries(set);
}
// A subject behind a sibling combinator or a state pseudo-class lays out an
// element only sometimes, so it neither sets nor clears anything.
function placedOf(selector: string, body: string, order: number): Placed[] {
  const props = layoutOf(body);
  if (Object.keys(props).length === 0) return [];
  return selectorList(selector)
    .flatMap(alternativesOf)
    .flatMap((alternative) => {
      const links = compounds(alternative);
      const [subject = "", place] = (links.at(-1) ?? "").split(
        /:(first-child|last-child)$/,
      );
      if (subject.includes(":") || /[+~]/.test(alternative)) return [];
      const context = links.slice(0, -1).map(classesOf);
      const weight = links.reduce((n, l) => n + classesOf(l).size, 0);
      const child = />\s*[^\s>]+$/.test(alternative);
      const rule = { subject: classesOf(subject), context, child, weight };
      const at = { order, props, place, selector: alternative };
      const any = subject.trim() === "*";
      return rule.subject.size > 0 || any ? [{ ...rule, ...at }] : [];
    });
}

function ownersIn(sheets: readonly string[]): Owners {
  const owners: Owners = {
    gap: new Set(),
    band: new Set(),
    row: new Set(),
    top: new Set(),
    placed: new Map(),
    loose: new Set(),
  };
  for (const [order, { selector, body }] of sheets.flatMap(rulesIn).entries()) {
    for (const rule of placedOf(selector, body, order)) {
      for (const name of rule.subject.size > 0 ? rule.subject : ["*"]) {
        owners.placed.set(name, [...(owners.placed.get(name) ?? []), rule]);
      }
    }
    const box = (p: string) => splitTopLevel(decl(body, p) ?? "", " \t\n");
    const rowGap = decl(body, "row-gap") ?? box("gap")[0] ?? "0";
    const direction = decl(body, "flex-direction") ?? decl(body, "flex-flow");
    const column = /^\s*column/.test(direction ?? "");
    const flex = display(body, "flex");
    const gap = (display(body, "grid") || (flex && column)) && !isZero(rowGap);
    const bottom = [
      ...[...body.matchAll(BOTTOM)].map((m) => m[1]),
      ...["margin-block", "padding-block"].map((p) => box(p).at(-1)),
      ...["margin", "padding"].map((p) => box(p)[box(p).length > 2 ? 2 : 0]),
    ].some((v) => v !== undefined && !isZero(v));
    const top = [
      ...[...body.matchAll(TOP)].map((m) => m[1]),
      ...["margin-block", "padding-block", "margin", "padding"].map(
        (p) => box(p)[0],
      ),
    ].some((v) => v !== undefined && !isZero(v));
    const names = selectorList(selector)
      .flatMap(subjectsOf)
      .flatMap((subject) => [...classesOf(subject)]);
    for (const name of names) {
      if (gap) owners.gap.add(name);
      if (bottom) owners.band.add(name);
      if (top) owners.top.add(name);
      if (/^(absolute|fixed)/.test(decl(body, "position") ?? "")) {
        owners.loose.add(name);
      }
      if (flex && !column) owners.row.add(name);
    }
  }
  return owners;
}

const opening = (n: ts.Node) => {
  if (ts.isJsxElement(n)) return n.openingElement;
  return ts.isJsxSelfClosingElement(n) ? n : undefined;
};
const tag = (n: ts.Node) => opening(n)?.tagName.getText() ?? "";
const attrs = (n: ts.Node) => {
  const all: readonly ts.JsxAttributeLike[] =
    opening(n)?.attributes.properties ?? [];
  return all.filter(ts.isJsxAttribute);
};
const attr = (n: ts.Node, name: string) =>
  attrs(n).find((a) => a.name.getText() === name);
function descendants(root: ts.Node): ts.Node[] {
  const out: ts.Node[] = [];
  const visit = (n: ts.Node) => {
    out.push(n);
    ts.forEachChild(n, visit);
  };
  ts.forEachChild(root, visit);
  return out;
}
const elementsIn = (root: ts.Node) => descendants(root).filter(opening);
const enclosing = (n: ts.Node) => {
  for (let p = n.parent; p; p = p.parent) if (ts.isJsxElement(p)) return p;
  return undefined;
};
function keys(a: ts.JsxAttribute | undefined): string[] {
  const init = a?.initializer;
  const e = init && ts.isJsxExpression(init) ? init.expression : init;
  if (e && ts.isTemplateExpression(e)) {
    return e.templateSpans.map((span) => span.expression.getText());
  }
  return e ? [e.getText()] : [];
}
function classes(n: ts.Node): string[] {
  const init = attr(n, "className")?.initializer;
  return (init ? [init, ...descendants(init)] : [])
    .flatMap((x) =>
      ts.isStringLiteral(x) || ts.isTemplateLiteralToken(x)
        ? x.text.split(/\s+/)
        : [],
    )
    .filter(Boolean);
}
const imports = (source: ts.SourceFile) =>
  source.statements.filter(ts.isImportDeclaration);
function localNames(source: ts.SourceFile, name: string): Set<string> {
  const bound = imports(source).flatMap((d) => {
    const named = d.importClause?.namedBindings;
    if (!named || !ts.isNamedImports(named)) return [];
    const same = named.elements.filter(
      (e) => (e.propertyName ?? e.name).text === name,
    );
    return same.map((e) => e.name.text);
  });
  return new Set([name, ...bound]);
}
const followed = (heading: ts.Node, parent: ts.Node) => {
  const kids = ts.isJsxElement(parent)
    ? parent.children.filter((c) =>
        ts.isJsxText(c)
          ? c.text.trim() !== ""
          : !ts.isJsxExpression(c) || c.expression,
      )
    : [];
  const mine = kids.findIndex(
    (c) => c.pos <= heading.pos && heading.end <= c.end,
  );
  return mine >= 0 && mine < kids.length - 1;
};

function componentSource(source: ts.SourceFile, name: string) {
  const from = imports(source).find((s) =>
    new RegExp(`\\b${name}\\b`).test(s.importClause?.getText() ?? ""),
  );
  if (!from) return source;
  const spec = from.moduleSpecifier.getText().slice(1, -1);
  const base = join(dirname(source.fileName), spec);
  const file = [`${base}.tsx`, `${base}.ts`, join(base, "index.tsx")].find(
    existsSync,
  );
  return file ? parseSource(file, readFileSync(file, "utf8")) : undefined;
}

function titlesIn(path: string, text: string, owners: Owners) {
  const source = parseSource(path, text);
  const titles: Title[] = [];
  const unresolved: string[] = [];
  const at = (n: ts.Node) => {
    const file = n.getSourceFile();
    const { line } = file.getLineAndCharacterOfPosition(n.getStart());
    return `${relative(srcDir, file.fileName)}:${line + 1}`;
  };
  const judge = (heading: ts.Node, modal: ts.Node) => {
    const parent = enclosing(heading) ?? modal;
    const around = parent === modal ? [] : classes(parent);
    const next = followed(heading, parent);
    const own = classes(heading).filter((c) => owners.band.has(c));
    let owner: string | null = null;
    const band = !next || around.some((c) => owners.row.has(c));
    if (band && around.some((c) => owners.band.has(c))) owner = "head band";
    if (next && around.some((c) => owners.gap.has(c))) owner = "gap container";
    if (own.length > 0)
      owner = own.includes("modal-title") ? "modal-title" : "own margin";
    titles.push({ where: at(heading), classes: classes(heading), owner });
  };
  const MODAL = localNames(source, "Modal");
  const HEADING = localNames(source, "Heading");
  const modals = elementsIn(source).filter(
    (n) => ts.isJsxElement(n) && MODAL.has(tag(n)),
  );
  for (const modal of modals) {
    const ids = keys(attr(modal, "labelledBy"));
    const labels = (a?: ts.JsxAttribute) =>
      keys(a).some((k) => ids.includes(k));
    const byId = (n: ts.Node) => labels(attr(n, "id"));
    const inside = elementsIn(modal);
    const first = inside.find((n) => enclosing(n) === modal);
    const found = inside.filter(
      (n) => HEADING.has(tag(n)) && (n === first || byId(n)),
    );
    const hop = (n: ts.Node) =>
      attrs(n)
        .filter((a) => a.name.getText() !== "id" && labels(a))
        .flatMap((a) => {
          const prop = a.name.getText();
          const target = componentSource(source, tag(n));
          if (!target) return [];
          const named = localNames(target, "Heading");
          return elementsIn(target).filter(
            (h) =>
              named.has(tag(h)) &&
              keys(attr(h, "id")).some(
                (k) => k === prop || k.endsWith(`.${prop}`),
              ),
          );
        });
    const components = inside.filter((n) => /^[A-Z]/.test(tag(n)));
    const own = found.length > 0 ? found : components.flatMap(hop);
    for (const heading of own) judge(heading, modal);
    const paragraph = inside.some((n) => tag(n) === "p" && byId(n));
    if (own.length === 0 && !paragraph) unresolved.push(at(modal));
  }
  return { modals: modals.length, titles, unresolved };
}

const FIELD_ROWS = ["Field", "ChoiceList", "Checkbox", "Radio"];
const DIALOGS = ["Modal", "ConfirmModal"];
type Row = {
  where: string;
  verdict: "spaced" | "flush" | "doubled";
  by?: string;
};

function bindings(source: ts.SourceFile) {
  const out = new Map<string, { spec: string; name: string }>();
  for (const s of source.statements) {
    if (!ts.isImportDeclaration(s) && !ts.isExportDeclaration(s)) continue;
    const spec = s.moduleSpecifier;
    const named = ts.isImportDeclaration(s)
      ? s.importClause?.namedBindings
      : s.exportClause;
    if (!spec || !ts.isStringLiteral(spec) || !named) continue;
    if (!ts.isNamedImports(named) && !ts.isNamedExports(named)) continue;
    for (const e of named.elements) {
      out.set(e.name.text, {
        spec: spec.text,
        name: (e.propertyName ?? e.name).text,
      });
    }
  }
  return out;
}
const declared = new Map<ts.SourceFile, Map<string, ts.Node>>();
function declarations(source: ts.SourceFile) {
  const known = declared.get(source);
  if (known) return known;
  const out = new Map<string, ts.Node>();
  for (const n of descendants(source)) {
    const named = ts.isFunctionDeclaration(n) || ts.isVariableDeclaration(n);
    if (named && n.name && !out.has(n.name.getText()))
      out.set(n.name.getText(), n);
  }
  declared.set(source, out);
  return out;
}
// Where a component tag is defined, through imports and re-exports.
function definition(
  source: ts.SourceFile,
  name: string,
  hops = 3,
): ts.Node | undefined {
  const own = declarations(source).get(name);
  if (own || hops === 0) return own;
  const bound = bindings(source).get(name);
  const file = bound && resolveRelative(source.fileName, bound.spec);
  return file
    ? definition(sourceFileAt(file), bound.name, hops - 1)
    : undefined;
}
const definedName = (def: ts.Node) =>
  (ts.isFunctionDeclaration(def) || ts.isVariableDeclaration(def)) && def.name
    ? def.name.getText()
    : "";
const within = (n: ts.Node, outer: ts.Node) =>
  outer.pos <= n.pos && n.end <= outer.end;

// `hosted` names the components in this file that some dialog renders.
function rowsIn(
  path: string,
  text: string,
  own: Owners,
  hosted: ReadonlySet<string> = new Set(),
) {
  const source = parseSource(path, text);
  const names = (list: string[]) =>
    new Set(list.flatMap((n) => [...localNames(source, n)]));
  const dialogs = names(DIALOGS);
  const isField = (n: ts.Node, file: ts.SourceFile) =>
    FIELD_ROWS.some((f) => localNames(file, f).has(tag(n))) ||
    classes(n).includes("form-row");
  // A component counts by the field rows it renders at its own root.
  const width = (n: ts.Node) => {
    if (isField(n, source)) return 1;
    const def = /^[A-Z]/.test(tag(n)) ? definition(source, tag(n)) : undefined;
    if (!def || dialogs.has(tag(n))) return 0;
    const file = def.getSourceFile();
    return elementsIn(def).filter((f) => {
      const parent = enclosing(f);
      return isField(f, file) && !(parent && within(parent, def));
    }).length;
  };
  const slotOf = (component: ts.Node) => {
    const def = definition(source, tag(component));
    const slot =
      def &&
      descendants(def).find(
        (n) =>
          ts.isJsxExpression(n) &&
          /^(props\.)?children$/.test(n.expression?.getText() ?? ""),
      );
    return slot && enclosing(slot);
  };
  const ancestry = (n: ts.Node) => {
    const out: string[][] = [];
    for (let p = enclosing(n); p; p = enclosing(p)) {
      const slot = /^[A-Z]/.test(tag(p)) && slotOf(p);
      out.push([...classes(p), ...(slot ? classes(slot) : [])]);
      if (dialogs.has(tag(p))) break;
    }
    return [...out, ["modal"]];
  };
  const subset = (need: Set<string>, cls: string[]) =>
    [...need].every((c) => cls.includes(c));
  // The heaviest, then latest, rule that sets `prop` on a box with these
  // classes under this chain.
  const resolve = (
    cls: string[],
    chain: string[][],
    prop: Prop,
    skip: (r: Placed) => boolean = (r) => r.place !== undefined,
  ) => {
    const applies = (r: Placed) =>
      [...r.subject].every((c) => cls.includes(c)) &&
      r.context.every((need, i) =>
        r.child && i === r.context.length - 1
          ? subset(need, chain[0] ?? [])
          : chain.some((a) => subset(need, a)),
      );
    return [...cls, "*"]
      .flatMap((c) => own.placed.get(c) ?? [])
      .filter((r) => r.props[prop] !== undefined && !skip(r) && applies(r))
      .sort((a, b) => a.weight - b.weight || a.order - b.order)
      .at(-1);
  };
  // A gap class built as `ds-gap-${gap}` reads as every class so prefixed.
  const spaces = (built: string[], chain: string[][], across: boolean) => {
    const keys = [...own.placed.keys()];
    const cls = built.flatMap((c) =>
      c.endsWith("-") ? keys.filter((k) => k.startsWith(c)) : [c],
    );
    const get = (prop: Prop) => resolve(cls, chain, prop)?.props[prop] ?? "";
    const shown = get("display");
    const column = get("direction") === "column";
    const flex = /flex/.test(shown);
    const down =
      (/grid/.test(shown) || (flex && column)) && !isZero(get("rowGap") || "0");
    const side = flex && !column && !isZero(get("columnGap") || "0");
    return down || (across && side);
  };
  // `across` counts a row's gap too; a margin doubles only a gap it stacks on.
  const stacks = (parent: ts.Node, across = true) => {
    const chain = ancestry(parent);
    if (spaces(classes(parent), chain, across)) return true;
    const slot = /^[A-Z]/.test(tag(parent)) && slotOf(parent);
    return !!slot && spaces(classes(slot), chain, across);
  };
  // What a row's own box is: its classes, or a component's root elements'.
  const rootsOf = (n: ts.Node, file: ts.SourceFile, hops = 3): string[][] => {
    const def = /^[A-Z]/.test(tag(n)) ? definition(file, tag(n)) : undefined;
    if (!def || hops === 0) return [classes(n)];
    const roots = elementsIn(def).filter((e) => {
      const parent = enclosing(e);
      return !(parent && within(parent, def));
    });
    return roots
      .flatMap((r) => rootsOf(r, def.getSourceFile(), hops - 1))
      .map((cls) => [...cls, ...classes(n)]);
  };
  // The rule that margins `n` vertically inside its stack, if any. A rule
  // written for a child OF this stack already counts the gap.
  const margined = (n: ts.Node) => {
    const chain = ancestry(n);
    const parent = enclosing(n);
    const peers = parent ? peersOf(n, parent) : [];
    const elsewhere = (r: Placed) =>
      (r.place === "first-child" && peers[0] !== n) ||
      (r.place === "last-child" && peers.at(-1) !== n);
    const edge = (cls: string[], side: "top" | "bottom") => {
      const won = resolve(cls, chain, side, elsewhere);
      const stack = won?.child ? won.context.at(-1) : undefined;
      const aware = !!stack && stack.size > 0 && subset(stack, chain[0]);
      return aware || isZero(won?.props[side] ?? "0") ? undefined : won;
    };
    const inFlow = (cls: string[]) => !cls.some((c) => own.loose.has(c));
    return rootsOf(n, source)
      .filter(inFlow)
      .flatMap((cls) => [edge(cls, "top"), edge(cls, "bottom")])
      .find((m) => m !== undefined)?.selector;
  };
  const has = (n: ts.Node | undefined, set: Set<string>) =>
    !!n && opening(n) !== undefined && classes(n).some((c) => set.has(c));
  const ternaries = descendants(source).filter(ts.isConditionalExpression);
  const exclusive = (a: ts.Node, b: ts.Node) =>
    ternaries.some(
      (c) =>
        (within(a, c.whenTrue) && within(b, c.whenFalse)) ||
        (within(a, c.whenFalse) && within(b, c.whenTrue)),
    );
  const mapped = (k: ts.Node) =>
    ts.isFunctionLike(k) &&
    ts.isCallExpression(k.parent) &&
    /\.(flatMap|map)$/.test(k.parent.expression.getText());
  const content = (k: ts.Node, parent: ts.Node) => {
    if (opening(k)) return enclosing(k) === parent;
    const child = ts.isJsxElement(k.parent) || ts.isJsxFragment(k.parent);
    if (!child || enclosing(k) !== parent) return false;
    if (ts.isJsxText(k)) return k.text.trim() !== "";
    return (
      ts.isJsxExpression(k) && !!k.expression && !descendants(k).some(opening)
    );
  };
  const peersOf = (n: ts.Node, parent: ts.Node) =>
    descendants(parent).filter(
      (k) => content(k, parent) && (k === n || !exclusive(k, n)),
    );
  const judge = (n: ts.Node, rows: number): Row["verdict"] => {
    const parent = enclosing(n);
    if (!parent) return "spaced";
    if (stacks(parent)) {
      return stacks(parent, false) && margined(n) ? "doubled" : "spaced";
    }
    const peers = peersOf(n, parent);
    const at = peers.indexOf(n);
    const [prev, next] = [peers[at - 1], peers[at + 1]];
    const between: ts.Node[] = [];
    for (let k = n.parent; k !== parent; k = k.parent) between.push(k);
    if (rows > 1 || between.some(mapped)) return "flush";
    if (!prev && !next && !dialogs.has(tag(parent))) return judge(parent, 1);
    const above = !prev || has(prev, own.band) || has(n, own.top);
    const below = !next || has(next, own.top) || has(n, own.band);
    return above && below ? "spaced" : "flush";
  };
  const modals = elementsIn(source).filter(
    (n) => ts.isJsxElement(n) && dialogs.has(tag(n)),
  );
  const bodies = [...hosted].flatMap((h) => declarations(source).get(h) ?? []);
  const inside = new Set([...modals, ...bodies].flatMap(elementsIn));
  const guests: { file: string; name: string }[] = [];
  for (const n of inside) {
    const def = /^[A-Z]/.test(tag(n)) ? definition(source, tag(n)) : undefined;
    if (!def || dialogs.has(tag(n)) || isField(n, source)) continue;
    const file = def.getSourceFile().fileName;
    if (file !== path) guests.push({ file, name: definedName(def) });
    else for (const e of elementsIn(def)) inside.add(e);
  }
  const rows: Row[] = [];
  // A row handed to a prop renders where the component puts it, not here.
  const slotted = (n: ts.Node) => {
    for (let k = n.parent; k && !ts.isJsxElement(k); k = k.parent) {
      if (ts.isJsxAttribute(k)) return true;
    }
    return false;
  };
  // The title's space is the title gate's to judge.
  const titled = localNames(source, "Heading");
  for (const n of inside) {
    const parent = enclosing(n);
    if (slotted(n) || !parent) continue;
    const { line } = source.getLineAndCharacterOfPosition(n.getStart());
    const where = `${relative(srcDir, path)}:${line + 1}`;
    const count = width(n);
    const verdict = count > 0 ? judge(n, count) : undefined;
    const by = stacks(parent, false) ? margined(n) : undefined;
    if (verdict) rows.push({ where, verdict, by });
    else if (by && !titled.has(tag(n)))
      rows.push({ where, verdict: "doubled", by });
  }
  return { dialogs: modals.length, rows, guests };
}

const sheets = underTree(/\.css$/).map((f) => readFileSync(f, "utf8"));
const owners = ownersIn(sheets);
// Stories are in: a reader copies them. Tests are out: their Modals are fixtures.
const texts = underTree(/\.(tsx|jsx)$/)
  .filter((f) => !/\.test\.(tsx|jsx)$/.test(f))
  .map((f) => ({ f, text: readFileSync(f, "utf8") }));
const corpus = texts.map(({ f, text }) => titlesIn(f, text, owners));
const titles = corpus.flatMap((c) => c.titles);
const fieldCorpus = texts.map(({ f, text }) => rowsIn(f, text, owners));
// A component a dialog renders from another file is judged in its own file, as
// part of that dialog. One hop: what it renders in turn is not followed.
const hosts = new Map<string, Set<string>>();
for (const { file, name } of fieldCorpus.flatMap((c) => c.guests)) {
  hosts.set(file, (hosts.get(file) ?? new Set()).add(name));
}
const guestRows = texts
  .filter(({ f }) => hosts.has(f))
  .flatMap(({ f, text }) => rowsIn(f, text, owners, hosts.get(f)).rows);
const fieldRows = [
  ...new Map(
    [...fieldCorpus.flatMap((c) => c.rows), ...guestRows].map((r) => [
      r.where,
      r,
    ]),
  ).values(),
];
const dialogCensus = texts.reduce((n, { text }) => {
  const aliases = [...text.matchAll(/\b(?:Confirm)?Modal as (\w+)/g)];
  const names = [...DIALOGS, ...aliases.map((m) => m[1])];
  const opened = text.match(new RegExp(`<(${names.join("|")})\\b`, "g"));
  return n + (opened?.length ?? 0);
}, 0);
const tagCensus = texts.reduce((n, { text }) => {
  const names = [
    "Modal",
    ...[...text.matchAll(/\bModal as (\w+)/g)].map((m) => m[1]),
  ];
  return (
    n + (text.match(new RegExp(`<(${names.join("|")})\\b`, "g"))?.length ?? 0)
  );
}, 0);
const matches = (e: (typeof EXCEPTIONS)[number], t: Title) =>
  t.where.startsWith(`${e.file}:`) && t.classes.join(" ") === e.classes;
const exempt = (t: Title) => EXCEPTIONS.some((e) => matches(e, t));

const PRELUDE =
  'import { Modal as Dialog } from "./modal";\nimport { Heading as H } from "./heading";';
const BODY = `function Body({ titleId }) { return <div>${h("id={titleId}")}</div>; }`;
function h(attributes: string) {
  return `<Heading size="large" ${attributes}>A</Heading>`;
}
const planted = (jsx: string, own = owners) => {
  const text = `${PRELUDE}\nconst A = () => (${jsx});\n${BODY}`;
  const { titles: found, unresolved } = titlesIn("planted.tsx", text, own);
  if (unresolved.length > 0) return "unresolved";
  return found.map((t) => t.owner ?? "orphan").join(" ") || "no title";
};
const modal = (inner: string, by = '"t"') =>
  `<Modal labelledBy=${by}>${inner}</Modal>`;
const T = h('id="t"');
type Case = { jsx: string; verdict: string; sheet: string };
const TEMPLATE = "className={`t-h2 $" + "{x} modal-title`}";
const MULTILINE =
  '<Modal\n  labelledBy="t"\n>\n  <Heading\n    size="large"\n    id="t"\n    className="modal-title"\n  >\n    A\n  </Heading>\n</Modal>';
const ROW = ".frow { display: flex; gap: var(--space-3) }";
const COLUMN =
  ".x { display: flex; flex-direction: column; gap: var(--space-3) }";

describe("a dialog's Heading title has an owner for the space under it (a <p> title is out of scope)", () => {
  it("walks every Modal in the tree and resolves each to its title", () => {
    const modals = corpus.reduce((n, c) => n + c.modals, 0);
    expect(tagCensus).toBeGreaterThan(0);
    expect(modals).toBe(tagCensus);
    expect(corpus.flatMap((c) => c.unresolved)).toEqual([]);
  });

  it("derives the owning containers from the stylesheets", () => {
    expect(owners.gap).toContain("form-stack");
    expect(owners.band).toContain("drawer-head");
    expect(owners.band).toContain("modal-title");
    const own = ownersIn([
      ".row { display: flex; gap: var(--space-3) }",
      ".grid { display: grid; gap: var(--space-3) }",
      ".band { padding: 0 0 var(--space-4) }",
      ".a:has(> .inner) { padding-bottom: var(--space-3) }",
      ".none { margin-bottom: 0 !important; padding-bottom: unset }",
    ]);
    expect([[...own.gap], [...own.band]]).toEqual([["grid"], ["band", "a"]]);
  });

  it.each`
    spec                                                 | jsx                                                             | verdict            | sheet
    ${"catches a title with no class"}                   | ${modal(`${T}<p />`)}                                           | ${"orphan"}        | ${""}
    ${"passes a title wearing modal-title"}              | ${modal(h('id="t" className="modal-title"'))}                   | ${"modal-title"}   | ${""}
    ${"passes a title opening a .form-stack"}            | ${modal(`<form className="form-stack">${T}<p /></form>`)}       | ${"gap container"} | ${""}
    ${"catches a title alone in a column gap container"} | ${modal(`<div className="x">${T}</div><p />`)}                  | ${"orphan"}        | ${COLUMN}
    ${"catches a title in a flex row with a gap"}        | ${modal(`<div className="frow">${T}<button /></div><p />`)}     | ${"orphan"}        | ${ROW}
    ${"catches a title in a bare wrapper"}               | ${modal(`<div>${T}</div>`)}                                     | ${"orphan"}        | ${""}
    ${"catches a band with content under the title"}     | ${modal(`<div className="band">${T}<p /></div>`)}               | ${"orphan"}        | ${".band { padding-bottom: var(--space-3) }"}
    ${"catches a band that :has() paints on its parent"} | ${modal(`<div className="x">${T}</div><p />`)}                  | ${"orphan"}        | ${".a:has(> .x) { padding-bottom: var(--space-3) }"}
    ${"treats only a <p> as a title outside scope"}      | ${modal('<p id="t">A</p>')}                                     | ${"no title"}      | ${""}
    ${"calls a <div id> title carrier unresolved"}       | ${modal(`<div id="t">${h("")}</div><p />`)}                     | ${"unresolved"}    | ${""}
    ${"calls a title it cannot reach unresolved"}        | ${modal("<Other />")}                                           | ${"unresolved"}    | ${""}
    ${"reads Modal imported under another name"}         | ${`<Dialog labelledBy="t">${T}<p /></Dialog>`}                  | ${"orphan"}        | ${""}
    ${"reads Heading imported under another name"}       | ${modal('<H size="large" id="t">A</H><p />')}                   | ${"orphan"}        | ${""}
    ${"reads an opening tag over several lines"}         | ${MULTILINE}                                                    | ${"modal-title"}   | ${""}
    ${"reads a className built by a template"}           | ${modal(h(`id={t} ${TEMPLATE}`), "{t}")}                        | ${"modal-title"}   | ${""}
    ${"reads a className built by cx()"}                 | ${modal(h('id={t} className={cx("a", "modal-title")}'), "{t}")} | ${"modal-title"}   | ${""}
    ${"follows an id prop one component away"}           | ${modal("<Body titleId={t} />", "{t}")}                         | ${"orphan"}        | ${""}
  `("$spec", ({ jsx, verdict, sheet }: Case) => {
    expect(planted(jsx, sheet ? ownersIn([sheet]) : owners)).toBe(verdict);
  });

  it("has an owner for every title in the tree", () => {
    const orphans = titles.filter((t) => t.owner === null && !exempt(t));
    expect(
      orphans.map((t) => t.where),
      "give the title modal-title, or open a gap container with it",
    ).toEqual([]);
  });

  it("keeps each exception on the one title that still needs it", () => {
    for (const e of EXCEPTIONS) {
      const still = titles.filter((t) => exempt(t) && t.owner === null);
      const here = still.filter((t) => matches(e, t));
      expect(here, e.reason).toHaveLength(1);
    }
  });
});

const ROWS_AT = join(srcDir, "design-system", "planted.tsx");
const ROWS_HELPER = `function Rows() { return <><Field /><Field /></>; }
function Body() { return <div><p /><Field /></div>; }
function Refusals() { return <div className="m" />; }`;
const plantedRows = (jsx: string, own = owners) => {
  const text = `import { ConfirmModal } from "./confirmmodal";\nimport { Stack } from "./stack";\nconst A = () => (${jsx});\n${ROWS_HELPER}`;
  const { rows } = rowsIn(ROWS_AT, text, own);
  return rows.map((r) => r.verdict).join(" ") || "no rows";
};
const TT = '<Heading className="modal-title">A</Heading>';
const ACT = '<div className="actions" />';
const inModal = (inner: string) => `<Modal>${TT}${inner}${ACT}</Modal>`;
const STACK =
  ".s { display: flex; flex-direction: column; gap: var(--space-3) }";
const M = ".m { margin-top: var(--space-2) }";
const MS =
  '<Modal><div className="s"><Field /><Field className="m" /></div></Modal>';
const SCOPED =
  ".f .pb { display: flex; flex-direction: column; gap: var(--space-3) }";
type RowCase = { jsx: string; verdict: string; sheet: string };

describe("a dialog's field rows have an owner for the space between them", () => {
  it("walks every Modal and ConfirmModal in the tree", () => {
    expect(fieldCorpus.reduce((n, c) => n + c.dialogs, 0)).toBe(dialogCensus);
    expect(fieldRows.length).toBeGreaterThan(0);
    expect(guestRows.length).toBeGreaterThan(0);
  });

  it.each`
    spec                                                    | jsx                                                                                                         | verdict                   | sheet
    ${"catches a field under a paragraph"}                  | ${inModal("<p /><Field />")}                                                                                | ${"flush"}                | ${""}
    ${"catches a ChoiceList over a Checkbox"}               | ${inModal("<ChoiceList /><Checkbox />")}                                                                    | ${"flush flush"}          | ${""}
    ${"passes a lone field between title and actions"}      | ${inModal("<Field />")}                                                                                     | ${"spaced"}               | ${""}
    ${"passes fields in a .form-stack"}                     | ${inModal('<div className="form-stack"><p /><Field /><Field /></div>')}                                     | ${"spaced spaced"}        | ${""}
    ${"passes fields whose ConfirmModal stacks them"}       | ${"<ConfirmModal><p /><Field /><Field /></ConfirmModal>"}                                                   | ${"spaced spaced"}        | ${""}
    ${"reads through a fragment and a condition"}           | ${inModal("<>{a && <Field />}<Field /></>")}                                                                | ${"flush flush"}          | ${""}
    ${"reads one field a map repeats"}                      | ${inModal("<div>{xs.map((x) => <Field key={x} />)}</div>")}                                                 | ${"flush"}                | ${""}
    ${"lets two branches of a ternary stand alone"}         | ${inModal("{a ? <Field /> : <Field />}")}                                                                   | ${"spaced spaced"}        | ${""}
    ${"lifts a lone field out of a bare wrapper"}           | ${inModal("<p /><form><Field /></form>")}                                                                   | ${"flush"}                | ${""}
    ${"passes fields side by side in a .form-row"}          | ${inModal('<div className="form-stack"><div className="form-row"><Field /><Field /></div></div>')}          | ${"spaced spaced spaced"} | ${""}
    ${"follows a component that renders field rows"}        | ${inModal("<Rows />")}                                                                                      | ${"flush"}                | ${""}
    ${"passes that component inside a stack"}               | ${"<ConfirmModal><Rows /></ConfirmModal>"}                                                                  | ${"spaced"}               | ${""}
    ${"catches a field margin that doubles the gap"}        | ${'<Modal><div className="s"><Field className="x" /><Field /></div></Modal>'}                               | ${"doubled spaced"}       | ${`${STACK} .x { margin-top: var(--space-3) }`}
    ${"reads the stack from the stylesheet"}                | ${'<Modal><div className="s"><Field /><Field /></div></Modal>'}                                             | ${"flush flush"}          | ${".s { display: flex; flex-direction: column }"}
    ${"joins a stack and a gap spelled by two classes"}     | ${'<Modal><div className="c g"><Field /><Field /></div></Modal>'}                                           | ${"spaced spaced"}        | ${".c { display: flex; flex-direction: column } .g { gap: var(--space-3) }"}
    ${"reads the gap a Stack is given"}                     | ${inModal('<Stack gap="2"><p /><Field /></Stack>')}                                                         | ${"spaced"}               | ${""}
    ${"lifts a field out of a render prop"}                 | ${inModal("<p /><Gate>{() => <Field />}</Gate>")}                                                           | ${"flush"}                | ${""}
    ${"skips a row handed to a slot prop"}                  | ${inModal("<View slot={<><Field /><Field /></>} />")}                                                       | ${"no rows"}              | ${""}
    ${"reads the class a component is handed"}              | ${inModal('<Card className="s"><Field /><Field /></Card>')}                                                 | ${"spaced spaced"}        | ${STACK}
    ${"follows a component the dialog renders"}             | ${inModal("<Body />")}                                                                                      | ${"flush"}                | ${""}
    ${"catches any margin a dialog stack's child adds"}     | ${inModal('<div className="s"><p /><div className="m" /></div>')}                                           | ${"doubled"}              | ${`${STACK} ${M}`}
    ${"catches the action row's margin inside a stack"}     | ${inModal('<div className="form-stack"><Field /><div className="actions" /></div>')}                        | ${"spaced doubled"}       | ${""}
    ${"catches a margin on a component's root"}             | ${"<ConfirmModal><Refusals /></ConfirmModal>"}                                                              | ${"doubled"}              | ${`${STACK.replace(".s", ".form-stack")} ${M}`}
    ${"lets a heavier rule under the stack zero it"}        | ${MS}                                                                                                       | ${"spaced spaced"}        | ${`${STACK} ${M} .s > .m { margin-top: 0 }`}
    ${"lets a rule for a child of the stack count the gap"} | ${MS}                                                                                                       | ${"spaced spaced"}        | ${`${STACK} ${M} .s > .m { margin-top: var(--space-1) }`}
    ${"ignores a margin set under another context"}         | ${MS}                                                                                                       | ${"spaced spaced"}        | ${`${STACK} .other .m { margin-top: var(--space-2) }`}
    ${"ignores an element out of flow"}                     | ${MS}                                                                                                       | ${"spaced spaced"}        | ${`${STACK} ${M} .m { position: absolute }`}
    ${"reads :last-child against the row's place"}          | ${'<Modal><div className="s"><Field className="m" /><Field className="m" /></div></Modal>'}                 | ${"doubled spaced"}       | ${`${STACK} ${M} .m:last-child { margin-top: 0 }`}
    ${"ignores a vertical margin in a row"}                 | ${'<Modal><div className="r"><Field className="m" /><Field /></div></Modal>'}                               | ${"spaced spaced"}        | ${`.r { display: flex; gap: var(--space-3) } ${M}`}
    ${"leaves the title to the title gate"}                 | ${'<Modal><div className="form-stack"><Heading className="modal-title">A</Heading><Field /></div></Modal>'} | ${"spaced"}               | ${""}
    ${"reads a stack scoped to another host as no stack"}   | ${'<Modal><div className="pb"><p /><Field /></div></Modal>'}                                                | ${"flush"}                | ${SCOPED}
    ${"reads that stack under its host"}                    | ${'<Modal><div className="f"><div className="pb"><p /><Field /></div></div></Modal>'}                       | ${"spaced"}               | ${SCOPED}
    ${"lets the dialog stack zero every row's margin"}      | ${'<Modal><div className="s"><Field /><Field className="m" /></div></Modal>'}                               | ${"spaced spaced"}        | ${`${STACK} ${M} .modal .s > * { margin-block: 0 }`}
    ${"lets a later two-class rule beat that zero"}         | ${'<Modal><div className="s"><Field /><Field className="m" /></div></Modal>'}                               | ${"spaced doubled"}       | ${`${STACK} .modal .s > * { margin-block: 0 } .modal .m { margin-top: var(--space-2) }`}
  `("$spec", ({ jsx, verdict, sheet }: RowCase) => {
    expect(plantedRows(jsx, sheet ? ownersIn([sheet]) : owners)).toBe(verdict);
  });

  it("spaces every field row in the tree", () => {
    const loose = fieldRows.filter((r) => r.verdict !== "spaced");
    expect(
      loose.map((r) => `${r.where} ${r.verdict}${r.by ? ` by ${r.by}` : ""}`),
      "stack the rows in a .form-stack, and drop a margin the stack's gap already pays",
    ).toEqual([]);
  });
});
