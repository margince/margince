// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, relative } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  attr,
  boxProps,
  callOf,
  capped,
  childrenHosts,
  childrenSlot,
  classes,
  cross,
  declarations,
  defOf,
  descendants,
  dialogSet,
  domainOf,
  elementsIn,
  enclosing,
  expanded,
  flowOf,
  fnOf,
  givenTo,
  handedJsx,
  handedTo,
  isDialogTag,
  isZero,
  jsxOf,
  keyOf,
  literalsIn,
  markupFiles,
  modalBoxes,
  type Owners,
  opening,
  ownersIn,
  type Placed,
  type Prop,
  presetOf,
  primitiveKey,
  propNames,
  rendered,
  returnedBy,
  rootsOf,
  sheetTexts,
  srcDir,
  tag,
  textCensus,
  upper,
  type Values,
  weightOf,
  whereOf,
  within,
} from "../../scripts/lib/dialoglayout";
import { parseSource, sourceFileAt } from "../../scripts/lib/source-tree";
import { classVariants, type Given } from "../testing/classnames";

// A dialog's rows sit one stack step apart: none flush, none doubled by its own
// margin. Stacks and margins are read from the sheets in each box's context.

type Verdict = "spaced" | "flush" | "doubled" | "unplaced";
type Row = { at: string; where: string; verdict: Verdict; by?: string };
// The class lists of the boxes from an element's parent out to its dialog.
type Chain = string[][];
// A component judged under the boxes its call sits in; `via` holds the
// components on the way to that call, so a recursion ends.
type Guest = { def: ts.Node; outer: Chain[]; via: ReadonlySet<ts.Node> };
// A component definition, mapped to the call that rendered it this time.
type Links = Map<ts.Node, ts.Node>;
type Slot = { roots: ts.Node[]; links: Links };
// Rows rendered away from where they are written: a JSX variable or call
// (`inline`), and JSX handed to a prop (`slots`), placed where it renders.
type Placement = {
  inline: Map<ts.Node, ts.Node[]>;
  slots: Map<ts.Node, Slot[]>;
  siteOf: Map<ts.Node, ts.Node>;
  frameOf: Map<ts.Node, Links>;
  unplaced: Map<ts.Node, ts.Node>;
};
type View = { placement: Placement; links: Links };
type Keys = { keys: ReadonlySet<string>; modal: string };
type Gate = Keys & { owners: Owners; outer: Chain[] };

const FIELD_ROWS = new Map([
  ["Field", "atoms"],
  ["Checkbox", "atoms"],
  ["Radio", "atoms"],
  ["ChoiceList", "choicelist"],
]);
// Design-system controls that draw their own label and are not form rows.
const NOT_ROWS = new Map([
  ["InlineText", "edits a value in place, its label hidden"],
  ["ChipValueList", "picks a list filter's values"],
]);
const rowKeys = [...FIELD_ROWS].map(([name, file]) => primitiveKey(file, name));
const HEADING = primitiveKey("heading", "Heading");
const NO_LINKS: Links = new Map();

const keyIs = (n: ts.Node, key: string) => {
  const def = defOf(n);
  return !!def && keyOf(def) === key;
};
const childOfJsx = (n: ts.Node) =>
  !!n.parent && (ts.isJsxElement(n.parent) || ts.isJsxFragment(n.parent));
// A component that renders a dialog around its children; `role` alone marks a box.
const wraps = ({ keys }: Pick<Keys, "keys">, n: ts.Node) =>
  upper(n) && isDialogTag(n.getSourceFile(), tag(n), keys);
const isDialog = (g: Pick<Keys, "keys">, n: ts.Node) =>
  literal(n, "role") === "dialog" || wraps(g, n);
const dialogsIn = (source: ts.SourceFile, keys: ReadonlySet<string>) =>
  elementsIn(source).filter((n) => isDialog({ keys }, n));
// A call and the component it calls, whose props the call hands values.
type Call = { at: ts.Node; def: ts.Node };
const variantsOf = (n: ts.Node, call?: Call) =>
  classVariants(
    n.getSourceFile(),
    attr(n, "className")?.initializer,
    16,
    call && givenAt(call),
  );
// What a component's props hold at one call: their literal branches, or the
// default when the call leaves one out.
function givenAt({ at, def }: Call): Given {
  const names = propNames(def);
  return (name) => {
    if (!names.has(name)) return undefined;
    const a = attr(at, name);
    if (!a) return [presetOf(def, name)];
    const init = a.initializer;
    const x = init && ts.isJsxExpression(init) ? init.expression : init;
    if (!x) return ["true"];
    const domain = domainOf(def, name);
    return literalsIn(x) ?? (domain && [...domain, undefined]);
  };
}
// Only a label hidden in every render: `labelHidden={x}` shows it in one.
const inPlace = (n: ts.Node) => {
  const hidden = attr(n, "labelHidden");
  const e = hidden?.initializer;
  return (
    !!hidden &&
    (!e || (ts.isJsxExpression(e) && e.expression?.getText() === "true"))
  );
};
function literal(n: ts.Node, name: string) {
  const init = attr(n, name)?.initializer;
  const e = init && ts.isJsxExpression(init) ? init.expression : init;
  return e && ts.isStringLiteral(e) ? e.text : undefined;
}

// Where a component renders its prop `name`, following it through the
// components that hand it on.
function slotSite(
  call: ts.Node,
  name: string,
  path: Links,
  depth = 0,
): { site: ts.Node; links: Links } | undefined {
  const def = defOf(call);
  if (!def || depth > 4) return undefined;
  const links: Links = new Map(path).set(def, call);
  const given = givenTo(def);
  const site = descendants(def).find(
    (x) =>
      ((ts.isJsxExpression(x) && childOfJsx(x)) || ts.isReturnStatement(x)) &&
      !!x.expression &&
      rendered(x.expression).some((r) => given(r, name)),
  );
  if (site) return { site, links };
  for (const a of descendants(def).filter(ts.isJsxAttribute)) {
    const e = a.initializer;
    if (!e || !ts.isJsxExpression(e) || !e.expression) continue;
    if (!given(e.expression, name)) continue;
    const found = slotSite(callOf(a), a.name.getText(), links, depth + 1);
    if (found) return found;
  }
  return undefined;
}

function placeInline(source: ts.SourceFile, p: Placement) {
  for (const x of descendants(source)) {
    if (!ts.isJsxExpression(x) || !x.expression || !childOfJsx(x)) continue;
    const e = x.expression;
    const callee = ts.isCallExpression(e) ? e.expression : undefined;
    const named = ts.isIdentifier(e) ? e : callee;
    const decl = named && declarations(source).get(named.getText());
    const value =
      decl && ts.isIdentifier(e) && ts.isVariableDeclaration(decl)
        ? jsxOf(decl.initializer)
        : [];
    const roots = callee && decl ? returnedBy(fnOf(decl)) : value;
    if (roots.length === 0) continue;
    p.inline.set(x, roots);
    for (const r of roots) if (!p.siteOf.has(r)) p.siteOf.set(r, x);
  }
}
function placeSlots(source: ts.SourceFile, p: Placement) {
  for (const a of descendants(source).filter(ts.isJsxAttribute)) {
    const roots = handedJsx(a);
    const call = callOf(a);
    if (roots.length === 0 || !upper(call)) continue;
    const found = slotSite(call, a.name.getText(), new Map());
    if (!found) {
      for (const r of roots) p.unplaced.set(r, call);
      continue;
    }
    const slot = { roots, links: found.links };
    p.slots.set(found.site, [...(p.slots.get(found.site) ?? []), slot]);
    for (const r of roots) {
      p.siteOf.set(r, found.site);
      p.frameOf.set(r, found.links);
    }
  }
}
const placements = new WeakMap<ts.SourceFile, Placement>();
function placementIn(source: ts.SourceFile): Placement {
  const known = placements.get(source);
  if (known) return known;
  const p: Placement = {
    inline: new Map(),
    slots: new Map(),
    siteOf: new Map(),
    frameOf: new Map(),
    unplaced: new Map(),
  };
  placeInline(source, p);
  placeSlots(source, p);
  placements.set(source, p);
  return p;
}

// Ancestors as rendered, not as written: a slotted root climbs to its site
// and a definition's root to its call, each under the frame that placed it.
function* climb(v: View, n: ts.Node): Generator<{ at: ts.Node; v: View }> {
  let view = v;
  for (let p = n, steps = 0; ; steps++) {
    const frame = view.placement.frameOf.get(p);
    if (frame && frame !== view.links) view = { ...view, links: frame };
    const next =
      view.placement.siteOf.get(p) ?? view.links.get(p)?.parent ?? p.parent;
    if (!next) return;
    if (steps > 4096) throw new Error(`${whereOf(n)}: renders inside itself`);
    p = next;
    yield { at: p, v: view };
  }
}
function parentOf(v: View, n: ts.Node) {
  for (const step of climb(v, n)) if (ts.isJsxElement(step.at)) return step;
  return undefined;
}
const enclosingIn = (v: View, n: ts.Node) => parentOf(v, n)?.at;
const hostedAt = (v: View, n: ts.Node) => [
  ...(v.placement.inline.get(n) ?? []),
  ...(v.placement.slots.get(n) ?? [])
    .filter((s) => s.links === v.links)
    .flatMap((s) => s.roots),
];
// Everything under `root` as rendered, what an expression hosts included.
function walk(v: View, root: ts.Node): ts.Node[] {
  const out: ts.Node[] = [];
  const seen = new Set<ts.Node>();
  const visit = (n: ts.Node) => {
    if (seen.has(n)) return;
    seen.add(n);
    out.push(n);
    for (const r of hostedAt(v, n)) visit(r);
    ts.forEachChild(n, visit);
  };
  ts.forEachChild(root, visit);
  return out;
}
function viewAt(placement: Placement, n: ts.Node): View {
  for (let p: ts.Node | undefined = n; p; p = p.parent) {
    const links = placement.frameOf.get(p);
    if (links) return { placement, links };
  }
  return { placement, links: NO_LINKS };
}
function unplacedAt(placement: Placement, n: ts.Node) {
  for (let p: ts.Node | undefined = n; p; p = p.parent) {
    if (placement.unplaced.has(p)) return true;
  }
  return false;
}

// A field whose label is hidden edits a value in place among its text,
// rather than standing as a row of a form.
const isField = (n: ts.Node) =>
  (rowKeys.some((k) => keyIs(n, k)) && !inPlace(n)) ||
  classes(n).includes("form-row");
// A component counts by the field rows it renders at its own root.
function width(g: Gate, n: ts.Node) {
  if (isField(n)) return 1;
  const def = defOf(n);
  if (!def || wraps(g, n)) return 0;
  return rootsOf(def).filter(isField).length;
}

// What a prop on `e` can be: its literal branches, what the caller hands
// when `e` forwards a prop of its own, and otherwise any value.
function valuesOf(e: ts.Node, name: string, callers: ts.Node[]): Values {
  const handed = handedTo(e, name);
  const each = handed === "any" ? [] : handed.map((x) => valueAt(x, callers));
  return handed === "any" || each.includes("any") ? "any" : each.flat();
}
function valueAt(x: ts.Expression | undefined, callers: ts.Node[]): Values {
  const listed = x ? literalsIn(x) : [undefined];
  if (listed) return listed;
  const [caller, ...rest] = callers;
  const def = caller && defOf(caller);
  if (!x || !def || !givenTo(def)(x)) return "any";
  const handed = ts.isPropertyAccessExpression(x) ? x.name.text : x.getText();
  const outer = valuesOf(caller, handed, rest);
  const preset = presetOf(def, handed);
  return outer === "any" ? outer : outer.map((v) => v ?? preset);
}
// The box classes a dialog element draws: Modal's for each value its props
// can take, or, for a wrapper, those of the dialog it renders around them.
function dialogBoxes(g: Gate, e: ts.Node, callers: ts.Node[]): string[][] {
  if (keyIs(e, g.modal)) {
    const given = boxProps().map((p): [string, Values] => [
      p,
      valuesOf(e, p, callers),
    ]);
    return modalBoxes(new Map(given));
  }
  const def = defOf(e);
  const inner = def && childrenHosts(def).find((h) => isDialog(g, h));
  if (!inner || callers.length > 4) {
    throw new Error(
      `${whereOf(e)}: no Modal found around this dialog's children; render the wrapper's Modal around {children}`,
    );
  }
  return dialogBoxes(g, inner, [e, ...callers]);
}
// A component's classes land on the root holding its children; a dialog's
// box stands outside that root unless it is that root.
function boxesAt(g: Gate, p: ts.Node): Chain[] {
  const modal = keyIs(p, g.modal);
  const slot = upper(p) && !modal ? childrenSlot(p) : undefined;
  const def = defOf(p);
  const inner = slot && def ? variantsOf(slot, { at: p, def }) : [[]];
  const own = cross(variantsOf(p), inner, p);
  if (!wraps(g, p)) return own.map((cls) => [cls]);
  const boxes = dialogBoxes(g, p, []);
  if (modal) return cross(own, boxes, p).map((cls) => [cls]);
  return own.flatMap((cls) => boxes.map((box) => [cls, box]));
}
function ancestry(g: Gate, v: View, n: ts.Node): Chain[] {
  let chains: Chain[] = [[]];
  for (let s = parentOf(v, n); s; s = parentOf(s.v, s.at)) {
    const p = s.at;
    const boxes = boxesAt(g, p);
    chains = capped(
      chains.flatMap((c) => boxes.map((b) => [...c, ...b])),
      p,
    );
    if (isDialog(g, p)) return chains;
  }
  return capped(
    chains.flatMap((c) => g.outer.map((o) => [...c, ...o])),
    n,
  );
}

const subset = (need: Set<string>, cls: string[]) =>
  [...need].every((c) => cls.includes(c));
// Ancestors match innermost first, each further out than the last.
function applies(r: Placed, cls: string[], chain: Chain) {
  let from = 0;
  return (
    subset(r.subject, cls) &&
    [...r.context].reverse().every((need, i) => {
      const end = r.child && i === 0 ? 1 : chain.length;
      const at = chain.slice(from, end).findIndex((a) => subset(need, a));
      from += at + 1;
      return at >= 0;
    })
  );
}
// The rule that sets `prop` on a box with these classes under this chain:
// `!important` first, then the heavier, then the later.
function resolve(
  owners: Owners,
  cls: string[],
  chain: Chain,
  prop: Prop,
  skip: (r: Placed) => boolean = (r) => r.place !== undefined,
) {
  const rank = (r: Placed) => (/!important/.test(r.props[prop] ?? "") ? 1 : 0);
  return [...cls, "*"]
    .flatMap((c) => owners.placed.get(c) ?? [])
    .filter((r) => r.props[prop] !== undefined && !skip(r))
    .filter((r) => applies(r, cls, chain))
    .sort(
      (a, b) => rank(a) - rank(b) || a.weight - b.weight || a.order - b.order,
    )
    .at(-1);
}
function spaces(owners: Owners, cls: string[], chain: Chain, across: boolean) {
  const get = (prop: Prop) => resolve(owners, cls, chain, prop)?.props[prop];
  const flow = flowOf({
    display: get("display"),
    direction: get("direction"),
    rowGap: get("rowGap"),
    columnGap: get("columnGap"),
  });
  return flow.down || (across && flow.across);
}
// `across` counts a row's gap too; a margin doubles only a gap it stacks on.
// A box with a conditional class stacks only if every branch does.
function stacks(g: Gate, v: View, parent: ts.Node, across = true) {
  const outer = ancestry(g, v, parent);
  return boxesAt(g, parent).every(([box, ...outside]) =>
    outer.every((chain) =>
      expanded(g.owners, box, parent).every((cls) =>
        spaces(g.owners, cls, [...outside, ...chain], across),
      ),
    ),
  );
}
// What a row's own box can be: each branch of its classes, or of a
// component's root elements' classes joined with the ones it is handed.
function boxesOf(g: Gate, n: ts.Node, hops = 3, call?: Call): string[][] {
  const own = variantsOf(n, call).flatMap((v) => expanded(g.owners, v, n));
  const def = defOf(n);
  if (!def || hops === 0 || wraps(g, n)) return own;
  return rootsOf(def)
    .flatMap((r) => boxesOf(g, r, hops - 1, { at: n, def }))
    .flatMap((cls) => own.map((o) => [...cls, ...o]));
}
const inFlow = (owners: Owners) => (cls: string[]) =>
  !cls.some((c) => owners.loose.has(c));
// The rule that margins `n` vertically inside its stack, if any. A rule
// written for a child OF this stack already counts the gap.
function margined(g: Gate, v: View, n: ts.Node) {
  const step = parentOf(v, n);
  const peers = step ? peersOf(step.v, n, step.at) : [];
  const elsewhere = (r: Placed) =>
    (r.place === "first-child" && peers[0] !== n) ||
    (r.place === "last-child" && peers.at(-1) !== n);
  const edge = (cls: string[], chain: Chain, side: "top" | "bottom") => {
    const won = resolve(g.owners, cls, chain, side, elsewhere);
    const stack = won?.child ? won.context.at(-1) : undefined;
    const aware = !!stack && stack.size > 0 && subset(stack, chain[0] ?? []);
    return aware || isZero(won?.props[side] ?? "0") ? undefined : won;
  };
  const boxes = boxesOf(g, n).filter(inFlow(g.owners));
  return ancestry(g, v, n)
    .flatMap((chain) =>
      boxes.flatMap((cls) => [
        edge(cls, chain, "top"),
        edge(cls, chain, "bottom"),
      ]),
    )
    .find((m) => m !== undefined)?.selector;
}
// Whether any rule margins one of these boxes, whatever box holds it.
function marginedAnywhere(g: Gate, n: ts.Node) {
  return boxesOf(g, n)
    .filter(inFlow(g.owners))
    .some((cls) =>
      [...cls, "*"]
        .flatMap((c) => g.owners.placed.get(c) ?? [])
        .some(
          (r) =>
            subset(r.subject, cls) &&
            [r.props.top, r.props.bottom].some((m) => !!m && !isZero(m)),
        ),
    );
}
const has = (n: ts.Node | undefined, set: Set<string>) =>
  !!n &&
  opening(n) !== undefined &&
  variantsOf(n).every((v) => v.some((c) => set.has(c)));
const ternaries = new WeakMap<ts.SourceFile, ts.ConditionalExpression[]>();
function exclusive(a: ts.Node, b: ts.Node) {
  const file = a.getSourceFile();
  const all =
    ternaries.get(file) ?? descendants(file).filter(ts.isConditionalExpression);
  ternaries.set(file, all);
  return all.some(
    (c) =>
      (within(a, c.whenTrue) && within(b, c.whenFalse)) ||
      (within(a, c.whenFalse) && within(b, c.whenTrue)),
  );
}
const mapped = (k: ts.Node) =>
  ts.isFunctionLike(k) &&
  ts.isCallExpression(k.parent) &&
  /\.(flatMap|map)$/.test(k.parent.expression.getText());
function content(v: View, k: ts.Node, parent: ts.Node) {
  if (opening(k)) return enclosingIn(v, k) === parent;
  if (!childOfJsx(k) || enclosingIn(v, k) !== parent) return false;
  if (ts.isJsxText(k)) return k.text.trim() !== "";
  return (
    ts.isJsxExpression(k) &&
    !!k.expression &&
    hostedAt(v, k).length === 0 &&
    !descendants(k).some(opening)
  );
}
const peersOf = (v: View, n: ts.Node, parent: ts.Node) =>
  walk(v, parent).filter(
    (k) => content(v, k, parent) && (k === n || !exclusive(k, n)),
  );
function judge(g: Gate, v: View, n: ts.Node, rows: number): Verdict {
  const step = parentOf(v, n);
  if (!step) return "spaced";
  const parent = step.at;
  if (stacks(g, step.v, parent)) {
    return stacks(g, step.v, parent, false) && margined(g, v, n)
      ? "doubled"
      : "spaced";
  }
  const peers = peersOf(step.v, n, parent);
  const at = peers.indexOf(n);
  const [prev, next] = [peers[at - 1], peers[at + 1]];
  const between = [...climb(v, n)].map((s) => s.at);
  const mappedBetween = between.slice(0, between.indexOf(parent)).some(mapped);
  if (rows > 1 || mappedBetween) return "flush";
  if (!prev && !next && !isDialog(g, parent)) {
    return judge(g, step.v, parent, 1);
  }
  const overhead = !prev || has(prev, g.owners.band) || has(n, g.owners.top);
  const underfoot = !next || has(next, g.owners.top) || has(n, g.owners.band);
  return overhead && underfoot ? "spaced" : "flush";
}

// Every element a dialog, or a guest's body, renders, slotted rows included,
// and the components it hands on to as guests of their own.
function scopeOf(
  g: Gate,
  source: ts.SourceFile,
  placement: Placement,
  hosted?: Guest,
) {
  const plain = { placement, links: NO_LINKS };
  const dialogs = hosted ? [] : dialogsIn(source, g.keys);
  const inside = new Set<ts.Node>();
  const add = (v: View, r: ts.Node) => {
    for (const e of [r, ...walk(v, r)].filter(opening)) inside.add(e);
  };
  for (const r of hosted ? [hosted.def] : dialogs) add(plain, r);
  for (const [r, links] of hosted ? [] : placement.frameOf) {
    const v = { placement, links };
    const inDialog = [...climb(v, r)].some((s) => isDialog(g, s.at));
    if (inDialog) add(v, r);
  }
  for (const [r, call] of placement.unplaced) {
    const def = defOf(call);
    if (def && elementsIn(def).some((e) => isDialog(g, e))) add(plain, r);
  }
  const via = new Set(hosted ? [...hosted.via, hosted.def] : []);
  const guests: Guest[] = [];
  for (const n of inside) {
    const def = defOf(n);
    if (!def || via.has(def) || wraps(g, n) || isField(n)) continue;
    guests.push({ def, outer: ancestry(g, viewAt(placement, n), n), via });
  }
  return { dialogs: dialogs.length, inside, guests };
}

const hints: Record<Exclude<Verdict, "spaced">, string> = {
  flush: "stack the rows in a .form-stack, or a component that owns one",
  doubled: "drop the margin the stack's gap already pays",
  unplaced:
    "the gate cannot follow that prop to where it renders; hand the rows as children, or render the prop as {prop} or {prop(…)}",
};
function rowAt(g: Gate, placement: Placement, n: ts.Node): Row | undefined {
  const v = viewAt(placement, n);
  const step = parentOf(v, n);
  const where = whereOf(n);
  const at = `${n.getSourceFile().fileName}:${n.getStart()}`;
  const count = width(g, n);
  if (unplacedAt(placement, n)) {
    const loud = count > 0 || marginedAnywhere(g, n);
    return loud ? { at, where, verdict: "unplaced" } : undefined;
  }
  if (!step) return undefined;
  const verdict = count > 0 ? judge(g, v, n, count) : undefined;
  const by = stacks(g, step.v, step.at, false) ? margined(g, v, n) : undefined;
  if (verdict) return { at, where, verdict, by };
  // The title's space is the title gate's to judge.
  if (!by || keyIs(n, HEADING)) return undefined;
  return { at, where, verdict: "doubled", by };
}
function rowsIn(g: Gate, source: ts.SourceFile, hosted?: Guest) {
  const placement = placementIn(source);
  const { dialogs, inside, guests } = scopeOf(g, source, placement, hosted);
  const rows = [...inside].flatMap((n) => rowAt(g, placement, n) ?? []);
  return { dialogs, rows, guests };
}

// A component a dialog renders is judged as part of that dialog under each
// chain of boxes it is called in, and so is what it renders in turn.
function rowsAcross(
  entry: readonly ts.SourceFile[],
  owners: Owners,
  keys: Keys,
) {
  const base: Gate = { ...keys, owners, outer: [[["modal"]]] };
  const rows = new Map<string, Row>();
  const seen = new Set<string>();
  let dialogs = 0;
  let depth = 0;
  let queue: Guest[] = [];
  const take = (found: ReturnType<typeof rowsIn>) => {
    for (const r of found.rows) rows.set(`${r.at} ${r.verdict}`, r);
    queue.push(...found.guests);
  };
  for (const source of entry) {
    const found = rowsIn(base, source);
    dialogs += found.dialogs;
    take(found);
  }
  for (; queue.length > 0; depth++) {
    const round = queue;
    queue = [];
    for (const q of round) {
      const key = `${keyOf(q.def)}#${JSON.stringify(q.outer)}`;
      if (seen.has(key)) continue;
      seen.add(key);
      take(rowsIn({ ...base, outer: q.outer }, q.def.getSourceFile(), q));
    }
  }
  return { rows: [...rows.values()], dialogs, depth };
}

// A root that is already a row counts as one where the component is used,
// so it is not followed.
function drawsLabel(def: ts.Node, hops = 2): boolean {
  return rootsOf(def).some((r) => {
    if (tag(r) === "label") return true;
    const inside = elementsIn(r).filter((e) => enclosing(e) === r);
    if (inside.some((e) => ["label", "legend"].includes(tag(e)))) return true;
    const inner = defOf(r);
    if (!inner || isField(r)) return false;
    return hops > 0 && drawsLabel(inner, hops - 1);
  });
}
const exported = (def: ts.Node) =>
  (ts.isFunctionDeclaration(def) || ts.isVariableDeclaration(def)) &&
  (ts.getCombinedModifierFlags(def) & ts.ModifierFlags.Export) !== 0;
const labelledIn = (source: ts.SourceFile) =>
  [...declarations(source)]
    .filter(([name, def]) => /^[A-Z]/.test(name) && exported(def))
    .filter(([, def]) => drawsLabel(def))
    .map(([name]) => name);

// Whichever test asks first builds the tree's census, inside its own timeout.
const TREE_BUILD = 60_000;
const once = <T>(make: () => T) => {
  let made: { value: T } | undefined;
  return () => {
    made ??= { value: make() };
    return made.value;
  };
};
const files = once(markupFiles);
const sources = once(() => files().map((f) => sourceFileAt(f)));
const owners = once(() => ownersIn(sheetTexts()));
const tree = once(() => rowsAcross(sources(), owners(), dialogSet()));
const census = once(() =>
  textCensus(files().map((f) => readFileSync(f, "utf8"))),
);

const AT = join(srcDir, "design-system", "planted.tsx");
const PRELUDE = `import { Modal } from "./modal";
import { ConfirmModal } from "./confirmmodal";
import { Checkbox, Field } from "./atoms";
import { ChoiceList } from "./choicelist";
import { Heading } from "./heading";
import { Stack } from "./stack";`;
const HELPERS = `function Rows() { return <><Field /><Field /></>; }
function Body() { return <div><p /><Field /></div>; }
function Inline() { return <Field labelHidden />; }
function Cited() { const cite = () => <i className="m" />; return <p>{cite()}</p>; }
function Bare() { const cite = () => <i className="m" />; return cite(); }
function Refusals() { return <div className="m" />; }
function Wrapped() { return <div className="s"><p /><div className="m" /></div>; }
function Nest() { return <div className="s"><p /><div className="m"><Nest /></div></div>; }
function Sheet({ children }) { return <Modal intent="form">{children}</Modal>; }
function Side({ children, intent = "drawer" }) { return <Modal intent={intent}>{children}</Modal>; }
function Shell({ body }) { return <Modal intent="form"><div>{body}</div></Modal>; }
function Framed({ body }) { return <div className="form-stack">{body}</div>; }
function Outer({ body }) { return <Framed body={body} />; }
function Explain({ body }) { return <Modal intent="form"><div className="form-stack">{body(1)}</div></Modal>; }
function Opened({ body, open }) { return <Modal intent="form">{open && body(1)}</Modal>; }
function Kept({ body }) { const shown = body(1); return <Modal intent="form">{shown}</Modal>; }
function Line({ control }) { return <div>{control}</div>; }
function fields() { return <><p /><Field /></>; }
const held = <><p /><Field /></>;
export { Modal as Pane };`;
const keysOf = (source: ts.SourceFile) => dialogSet([source], dialogSet());
// Fixture text writes an interpolation as `#{…}`, so no string here reads as one.
const interpolated = (text: string) => text.replaceAll("#{", "${");
const plantedRows = (jsx: string, own = owners()) => {
  const text = `${PRELUDE}\nconst A = () => (${interpolated(jsx)});\n${HELPERS}`;
  const source = parseSource(AT, text);
  const { rows } = rowsAcross([source], own, keysOf(source));
  return rows.map((r) => r.verdict).join(" ") || "no rows";
};
const TITLE = '<Heading className="modal-title">A</Heading>';
const ACTIONS = '<div className="actions" />';
const inModal = (inner: string) =>
  `<Modal intent="form">${TITLE}${inner}${ACTIONS}</Modal>`;
const STACK =
  ".s { display: flex; flex-direction: column; gap: var(--space-3) }";
const FORM_STACK = STACK.replace(".s", ".form-stack");
const MARGIN = ".m { margin-top: var(--space-2) }";
const MARGINED_STACK =
  '<Modal intent="form"><div className="s"><Field /><Field className="m" /></div></Modal>';
const SCOPED =
  ".f .pb { display: flex; flex-direction: column; gap: var(--space-3) }";
const PARAGRAPH_STACK = '<div className="s"><p /><Field /></div>';
const DRAWER_STACK = `.modal-drawer ${STACK}`;
const DRAWER_BLOCK = `${STACK} .modal-drawer .s { display: block }`;
const HOSTED = inModal('<div className="f"><Wrapped /></div>');
const HOSTED_STACK = `.f ${STACK} ${MARGIN}`;
const HOSTED_MARGIN = `${STACK} .f ${MARGIN}`;
type RowCase = { jsx: string; verdict: string; sheet: string };

// Three files, so a guest's guest is two files away from its dialog.
function plantedAcross(
  host = "<B />",
  rows = "<div><p /><Field /></div>",
  own = owners(),
) {
  const dir = mkdtempSync(join(tmpdir(), "dialogrows-"));
  const ds = relative(dir, join(srcDir, "design-system"));
  writeFileSync(
    join(dir, "a.tsx"),
    `import { Modal } from "${ds}/modal";\nimport { B } from "./b";\nexport const A = () => <Modal intent="form">${host}</Modal>;\n`,
  );
  writeFileSync(
    join(dir, "b.tsx"),
    `import { C } from "./c";\nexport function B() { return <C />; }\n`,
  );
  writeFileSync(
    join(dir, "c.tsx"),
    `import { Field } from "${ds}/atoms";\nexport function C() { return ${rows}; }\n`,
  );
  const a = sourceFileAt(join(dir, "a.tsx"));
  const { rows: found, depth } = rowsAcross([a], own, keysOf(a));
  return { verdicts: found.map((r) => r.verdict).join(" "), depth };
}

// A modal.tsx whose modalClass names and orders its parameters its own way.
function plantedModal(body: string) {
  const dir = mkdtempSync(join(tmpdir(), "dialogbox-"));
  const path = join(dir, "modal.tsx");
  writeFileSync(
    path,
    `const KINDS = ["sheet", "card"] as const; type Kind = (typeof KINDS)[number]; export function Modal({ kind, size = "default", placement = "center", children }) { return <div className={modalClass(kind, placement, size)}>{children}</div>; }
function modalClass(kind: Kind | undefined, where: "center" | "right", width: "default" | "wide") { ${body} }\n`,
  );
  return path;
}
const DRAWER_CLASS = `if (kind === "sheet") return "modal sheet"; if (where === "right") return width === "wide" ? "modal drawer wide" : "modal drawer"; return "modal";`;
const classCall = (expression: string) => {
  const code = `const x = <i className={${interpolated(expression)}} />;`;
  const source = parseSource(AT, code);
  const init = descendants(source).find(ts.isJsxAttribute)?.initializer;
  return () => classVariants(source, init);
};

describe("a dialog's field rows have an owner for the space between them", () => {
  it(
    "walks every dialog the text spells, wrappers included",
    () => {
      expect(tree().dialogs).toBe(census().dialogs);
      expect(dialogSet().keys.size).toBeGreaterThan(1);
      expect(tree().rows.length).toBeGreaterThan(0);
      expect(tree().depth).toBeGreaterThan(1);
    },
    TREE_BUILD,
  );

  it(
    "would count short of the text with only Modal and ConfirmModal",
    () => {
      const cut = new Set([
        dialogSet().modal,
        primitiveKey("confirmmodal", "ConfirmModal"),
      ]);
      const walked = sources().reduce(
        (n, s) => n + dialogsIn(s, cut).length,
        0,
      );
      expect(walked).toBeLessThan(census().dialogs);
    },
    TREE_BUILD,
  );

  it("spells a wrapper's wrapper, an alias and a role as dialogs", () => {
    const text = `import { Modal as Pane } from "./modal";
function Sheet({ children }) { return <Pane>{children}</Pane>; }
function Drawer({ children }) { return <Sheet>{children}</Sheet>; }
function Trigger({ children }) { return <Row>{children}<Modal /></Row>; }
// <Modal> in a comment
const Typed: FC<P> = ({ children }) => <Modal>{children}</Modal>;
const A = () => <><Drawer /><div role="dialog" /><Trigger /><Typed /></>;`;
    expect(textCensus([text]).dialogs).toBe(7);
    const trailing = `${text}\nconst B = () => <p />; // <Modal> after code`;
    expect(textCensus([trailing]).dialogs).toBe(7);
  });

  it("follows a guest's guest into a third file", () => {
    expect(plantedAcross()).toEqual({ verdicts: "flush", depth: 2 });
  });

  it("reads a guest's rows under the boxes its host puts it in", () => {
    const scoped = ownersIn([SCOPED]);
    const stack = '<div className="pb"><p /><Field /></div>';
    const host = '<div className="f"><B /></div>';
    expect(plantedAcross(host, stack, scoped).verdicts).toBe("spaced");
    expect(plantedAcross("<B />", stack, scoped).verdicts).toBe("flush");
  });

  it("names every design-system control that draws its own label", () => {
    const ds = files().filter(
      (f) => f.includes("/design-system/") && !f.endsWith(".stories.tsx"),
    );
    const found = ds.flatMap((f) => labelledIn(sourceFileAt(f)));
    expect(found.sort()).toEqual(
      [...FIELD_ROWS.keys(), ...NOT_ROWS.keys()].sort(),
    );
    const planted = parseSource(
      AT,
      "export function Pick() { return <label><input /></label>; }",
    );
    expect(labelledIn(planted)).toEqual(["Pick"]);
  });

  it("reads the Modal box classes by parameter position", () => {
    const path = plantedModal(DRAWER_CLASS);
    const given = new Map<string, Values>([
      ["placement", ["right"]],
      ["size", ["wide"]],
    ]);
    const drawer = ["modal", "drawer", "wide"];
    expect(modalBoxes(given, path)).toEqual([drawer]);
    given.set("kind", "any");
    expect(modalBoxes(given, path)).toEqual([["modal", "sheet"], drawer]);
    const any = new Map<string, Values>([["placement", "any"]]);
    expect(modalBoxes(any, path)).toEqual([["modal"], ["modal", "drawer"]]);
    const unread = plantedModal('switch (where) { default: return "modal"; }');
    expect(() => modalBoxes(new Map(), unread)).toThrow(/does not evaluate/);
    const empty = plantedModal('if (where === "right") return "modal";');
    expect(() => modalBoxes(new Map(), empty)).toThrow(/returns no class/);
  });

  it.each`
    expression                                    | variants
    ${'`q#{a ? " m" : ""}`'}                      | ${[["q", "m"], ["q"]]}
    ${"`tone-#{level}`"}                          | ${[["tone-*"]]}
    ${"`m#{k} base`"}                             | ${[["m*", "base"]]}
    ${"`a #{b} c`"}                               | ${[["a", "c"]]}
    ${'cx("a", open && `b-#{x ? "on" : "off"}`)'} | ${[["a"], ["a", "b-on"], ["a", "b-off"]]}
    ${'"tone-" + level'}                          | ${[["tone-*"]]}
    ${'"m" + (a ? "x y" : "z")'}                  | ${[["mx", "y"], ["mz"]]}
    ${'cx("a") + " b"'}                           | ${[["a", "b"]]}
    ${'"x-" + look(s)'}                           | ${[["x-*"]]}
    ${'`x #{["form-stack", "y"]}`'}               | ${[["x"]]}
    ${'`#{["a", "b"].join(" ")} c`'}              | ${[["a", "b", "c"]]}
  `("reads $expression as its branches", ({ expression, variants }) => {
    expect(classCall(expression)()).toEqual(variants);
  });

  it("refuses a class list past its branch cap", () => {
    const five = 'cx(a && "a", b && "b", c && "c", d && "d", e && "e")';
    expect(classCall(five)).toThrow(/more than 16 class branches/);
    const leaves = (n: number, p: string) =>
      `(${Array.from({ length: n - 1 }, (_, i) => `k === ${i} ? "${p}${i}" : `).join("")}"")`;
    expect(classCall(leaves(17, "c"))).toThrow(/more than 16/);
    expect(classCall(`${leaves(9, "a")} || ${leaves(9, "b")}`)).toThrow(
      /than 16/,
    );
  });

  it.each`
    selector                   | weight
    ${".a"}                    | ${1000}
    ${".a:where(.b .c)"}       | ${1000}
    ${":is(.a, .b .c)"}        | ${2000}
    ${":not(#x).a"}            | ${1_001_000}
    ${"div.a > p"}             | ${1002}
    ${".a::before"}            | ${1001}
    ${".a:hover"}              | ${2000}
    ${":nth-child(2 of .a.b)"} | ${3000}
  `("weighs $selector as $weight", ({ selector, weight }) => {
    expect(weightOf(selector)).toBe(weight);
  });

  it.each`
    spec                                                        | jsx                                                                                                                       | verdict                   | sheet
    ${"catches a field under a paragraph"}                      | ${inModal("<p /><Field />")}                                                                                              | ${"flush"}                | ${""}
    ${"catches a ChoiceList over a Checkbox"}                   | ${inModal("<ChoiceList /><Checkbox />")}                                                                                  | ${"flush flush"}          | ${""}
    ${"passes a lone field between title and actions"}          | ${inModal("<Field />")}                                                                                                   | ${"spaced"}               | ${""}
    ${"passes fields in a .form-stack"}                         | ${inModal('<div className="form-stack"><p /><Field /><Field /></div>')}                                                   | ${"spaced spaced"}        | ${""}
    ${"passes fields whose ConfirmModal stacks them"}           | ${"<ConfirmModal><p /><Field /><Field /></ConfirmModal>"}                                                                 | ${"spaced spaced"}        | ${""}
    ${"reads through a fragment and a condition"}               | ${inModal("<>{a && <Field />}<Field /></>")}                                                                              | ${"flush flush"}          | ${""}
    ${"reads one field a map repeats"}                          | ${inModal("<div>{xs.map((x) => <Field key={x} />)}</div>")}                                                               | ${"flush"}                | ${""}
    ${"lets two branches of a ternary stand alone"}             | ${inModal("{a ? <Field /> : <Field />}")}                                                                                 | ${"spaced spaced"}        | ${""}
    ${"lifts a lone field out of a bare wrapper"}               | ${inModal("<p /><form><Field /></form>")}                                                                                 | ${"flush"}                | ${""}
    ${"passes fields side by side in a .form-row"}              | ${inModal('<div className="form-stack"><div className="form-row"><Field /><Field /></div></div>')}                        | ${"spaced spaced spaced"} | ${""}
    ${"follows a component that renders field rows"}            | ${inModal("<Rows />")}                                                                                                    | ${"flush"}                | ${""}
    ${"passes that component inside a stack"}                   | ${"<ConfirmModal><Rows /></ConfirmModal>"}                                                                                | ${"spaced"}               | ${""}
    ${"catches a field margin that doubles the gap"}            | ${'<Modal intent="form"><div className="s"><Field className="x" /><Field /></div></Modal>'}                               | ${"doubled spaced"}       | ${`${STACK} .x { margin-top: var(--space-3) }`}
    ${"reads the stack from the stylesheet"}                    | ${'<Modal intent="form"><div className="s"><Field /><Field /></div></Modal>'}                                             | ${"flush flush"}          | ${".s { display: flex; flex-direction: column }"}
    ${"joins a stack and a gap spelled by two classes"}         | ${'<Modal intent="form"><div className="c g"><Field /><Field /></div></Modal>'}                                           | ${"spaced spaced"}        | ${".c { display: flex; flex-direction: column } .g { gap: var(--space-3) }"}
    ${"reads the gap a Stack is given"}                         | ${inModal('<Stack gap="2"><p /><Field /></Stack>')}                                                                       | ${"spaced"}               | ${""}
    ${"lifts a field out of a render prop"}                     | ${inModal("<p /><Gate>{() => <Field />}</Gate>")}                                                                         | ${"flush"}                | ${""}
    ${"calls a row handed to a prop it cannot place"}           | ${inModal("<View slot={<><Field /><Field /></>} />")}                                                                     | ${"unplaced unplaced"}    | ${""}
    ${"calls a margin in a prop it cannot place"}               | ${inModal('<View slot={<div className="m" />} />')}                                                                       | ${"unplaced"}             | ${MARGIN}
    ${"reads the class a component is handed"}                  | ${inModal('<Card className="s"><Field /><Field /></Card>')}                                                               | ${"spaced spaced"}        | ${STACK}
    ${"follows a component the dialog renders"}                 | ${inModal("<Body />")}                                                                                                    | ${"flush"}                | ${""}
    ${"catches any margin a dialog stack's child adds"}         | ${inModal('<div className="s"><p /><div className="m" /></div>')}                                                         | ${"doubled"}              | ${`${STACK} ${MARGIN}`}
    ${"catches the action row's margin inside a stack"}         | ${inModal('<div className="form-stack"><Field /><div className="actions" /></div>')}                                      | ${"spaced doubled"}       | ${""}
    ${"catches a margin on a component's root"}                 | ${"<ConfirmModal><Refusals /></ConfirmModal>"}                                                                            | ${"doubled"}              | ${`${FORM_STACK} ${MARGIN}`}
    ${"lets a heavier rule under the stack zero it"}            | ${MARGINED_STACK}                                                                                                         | ${"spaced spaced"}        | ${`${STACK} ${MARGIN} .s > .m { margin-top: 0 }`}
    ${"lets a rule for a child of the stack count the gap"}     | ${MARGINED_STACK}                                                                                                         | ${"spaced spaced"}        | ${`${STACK} ${MARGIN} .s > .m { margin-top: var(--space-1) }`}
    ${"ignores a margin set under another context"}             | ${MARGINED_STACK}                                                                                                         | ${"spaced spaced"}        | ${`${STACK} .other .m { margin-top: var(--space-2) }`}
    ${"ignores an element out of flow"}                         | ${MARGINED_STACK}                                                                                                         | ${"spaced spaced"}        | ${`${STACK} ${MARGIN} .m { position: absolute }`}
    ${"reads :last-child against the row's place"}              | ${'<Modal intent="form"><div className="s"><Field className="m" /><Field className="m" /></div></Modal>'}                 | ${"doubled spaced"}       | ${`${STACK} ${MARGIN} .m:last-child { margin-top: 0 }`}
    ${"ignores a vertical margin in a row"}                     | ${'<Modal intent="form"><div className="r"><Field className="m" /><Field /></div></Modal>'}                               | ${"spaced spaced"}        | ${`.r { display: flex; gap: var(--space-3) } ${MARGIN}`}
    ${"leaves the title to the title gate"}                     | ${'<Modal intent="form"><div className="form-stack"><Heading className="modal-title">A</Heading><Field /></div></Modal>'} | ${"spaced"}               | ${""}
    ${"reads a stack scoped to another host as no stack"}       | ${'<Modal intent="form"><div className="pb"><p /><Field /></div></Modal>'}                                                | ${"flush"}                | ${SCOPED}
    ${"reads that stack under its host"}                        | ${'<Modal intent="form"><div className="f"><div className="pb"><p /><Field /></div></div></Modal>'}                       | ${"spaced"}               | ${SCOPED}
    ${"lets the dialog stack zero every row's margin"}          | ${MARGINED_STACK}                                                                                                         | ${"spaced spaced"}        | ${`${STACK} ${MARGIN} .modal .s > * { margin-block: 0 }`}
    ${"lets a later two-class rule beat that zero"}             | ${MARGINED_STACK}                                                                                                         | ${"spaced doubled"}       | ${`${STACK} .modal .s > * { margin-block: 0 } .modal .m { margin-top: var(--space-2) }`}
    ${"reads a gap shorthand after its longhand"}               | ${'<Modal intent="form"><div className="s"><Field /><Field /></div></Modal>'}                                             | ${"spaced spaced"}        | ${".s { display: flex; flex-direction: column; row-gap: 0; gap: var(--space-3) }"}
    ${"reads a context's ancestors in order"}                   | ${'<Modal intent="form"><div className="o"><div className="b"><p /><Field /></div></div></Modal>'}                        | ${"flush"}                | ${".o .modal .b { display: flex; flex-direction: column; gap: var(--space-3) }"}
    ${"weighs :last-child above a later bare class"}            | ${MARGINED_STACK}                                                                                                         | ${"spaced doubled"}       | ${`${STACK} .m:last-child { margin-top: var(--space-2) } .m { margin-top: 0 }`}
    ${"weighs :is() by its heaviest argument"}                  | ${MARGINED_STACK}                                                                                                         | ${"spaced doubled"}       | ${`${STACK} :is(.s, .q .r) > * { margin-block: 0 } .s .m { margin-top: var(--space-2) }`}
    ${"reads a component that wraps Modal as a dialog"}         | ${"<Sheet><p /><Field /></Sheet>"}                                                                                        | ${"flush"}                | ${""}
    ${"reads Modal re-exported under another name"}             | ${'<Pane intent="form"><p /><Field /></Pane>'}                                                                            | ${"flush"}                | ${""}
    ${"reads an element with role=dialog as a dialog"}          | ${'<div role="dialog"><p /><Field /></div>'}                                                                              | ${"flush"}                | ${""}
    ${"reads a component with role=dialog as a dialog"}         | ${'<Card role="dialog"><p /><Field /></Card>'}                                                                            | ${"flush"}                | ${""}
    ${"reads a two-value margin with !important"}               | ${'<Modal intent="form"><div className="s"><Field className="m" /><Field /></div></Modal>'}                               | ${"spaced spaced"}        | ${`${STACK} .m { margin: 0 auto !important }`}
    ${"reads rows held in a JSX variable"}                      | ${inModal("{held}")}                                                                                                      | ${"flush"}                | ${""}
    ${"reads rows a function returns"}                          | ${inModal("{fields()}")}                                                                                                  | ${"flush"}                | ${""}
    ${"places a prop's rows where the dialog puts them"}        | ${"<Shell body={<><p /><Field /></>} />"}                                                                                 | ${"flush"}                | ${""}
    ${"places a prop's rows in the stack that holds them"}      | ${inModal("<Framed body={<><p /><Field /></>} />")}                                                                       | ${"spaced"}               | ${""}
    ${"follows a prop handed on to another component"}          | ${inModal("<Outer body={<><p /><Field /></>} />")}                                                                        | ${"spaced"}               | ${""}
    ${"places a render prop's rows where it is called"}         | ${"<Explain body={(n) => <><p /><Field /></>} />"}                                                                        | ${"spaced"}               | ${""}
    ${"places a render prop called under a condition"}          | ${"<Opened body={(n) => <><p /><Field /></>} />"}                                                                         | ${"flush"}                | ${""}
    ${"calls a render prop it cannot place unplaced"}           | ${"<Kept body={(n) => <><p /><Field /></>} />"}                                                                           | ${"unplaced"}             | ${""}
    ${"places a prop through a component inside its own prop"}  | ${inModal("<p /><Line control={<Line control={<Field />} />} />")}                                                        | ${"flush"}                | ${""}
    ${"reads the drawer's own box classes"}                     | ${`<Modal intent="drawer">${PARAGRAPH_STACK}</Modal>`}                                                                    | ${"spaced"}               | ${DRAWER_STACK}
    ${"judges each intent a ternary picks"}                     | ${`<Modal intent={a ? "drawer" : "confirm"}>${PARAGRAPH_STACK}</Modal>`}                                                  | ${"flush"}                | ${DRAWER_BLOCK}
    ${"judges every intent an unread value can be"}             | ${`<Modal intent={where}>${PARAGRAPH_STACK}</Modal>`}                                                                     | ${"flush"}                | ${DRAWER_BLOCK}
    ${"reads the intent a wrapper defaults to"}                 | ${`<Side>${PARAGRAPH_STACK}</Side>`}                                                                                      | ${"spaced"}               | ${DRAWER_STACK}
    ${"ignores a rule that holds only under a query"}           | ${`<Modal intent="form">${PARAGRAPH_STACK}</Modal>`}                                                                      | ${"spaced"}               | ${`${STACK} @media (min-width: 1px) { .s { display: block } }`}
    ${"lets !important outrank a heavier rule"}                 | ${`<Modal intent="form">${PARAGRAPH_STACK}</Modal>`}                                                                      | ${"spaced"}               | ${".s { display: flex !important; flex-direction: column; gap: var(--space-3) } .modal .s { display: block }"}
    ${"stacks only if every branch of a class does"}            | ${'<Modal intent="form"><div className={a ? "s" : "t"}><p /><Field /></div></Modal>'}                                     | ${"flush"}                | ${STACK}
    ${"doubles if any branch of a class margins"}               | ${'<Modal intent="form"><div className="s"><Field /><Field className={a && "m"} /></div></Modal>'}                        | ${"spaced doubled"}       | ${`${STACK} ${MARGIN}`}
    ${"doubles if a template's branch margins"}                 | ${'<Modal intent="form"><div className="s"><Field /><Field className={`f#{a ? " m" : ""}`} /></div></Modal>'}             | ${"spaced doubled"}       | ${`${STACK} ${MARGIN}`}
    ${"reads a class an interpolation cuts as each it can be"}  | ${'<Modal intent="form"><div className="s"><Field /><Field className={`m#{k}`} /></div></Modal>'}                         | ${"spaced doubled"}       | ${`${STACK} .mx { margin-top: var(--space-2) }`}
    ${"weighs :where() as nothing"}                             | ${MARGINED_STACK}                                                                                                         | ${"spaced doubled"}       | ${`${STACK} .modal :where(.s) > * { margin-block: 0 } ${MARGIN}`}
    ${"leaves a field editing in place among text alone"}       | ${inModal("<div><span /><Field labelHidden /></div>")}                                                                    | ${"no rows"}              | ${""}
    ${"still catches the same field with its label shown"}      | ${inModal("<div><span /><Field /></div>")}                                                                                | ${"flush"}                | ${""}
    ${"catches a field whose label shows in a branch"}          | ${inModal("<div><span /><Field labelHidden={a} /></div>")}                                                                | ${"flush"}                | ${""}
    ${"leaves a component editing in place alone"}              | ${inModal("<div><span /><Inline /></div>")}                                                                               | ${"no rows"}              | ${""}
    ${"keeps a local function's element where it is used"}      | ${"<ConfirmModal><Cited /></ConfirmModal>"}                                                                               | ${"no rows"}              | ${`${FORM_STACK} ${MARGIN}`}
    ${"roots a local function's element the component returns"} | ${"<ConfirmModal><Bare /></ConfirmModal>"}                                                                                | ${"doubled"}              | ${`${FORM_STACK} ${MARGIN}`}
    ${"reads a same-file stack under the box its call is in"}   | ${HOSTED}                                                                                                                 | ${"doubled"}              | ${HOSTED_STACK}
    ${"reads that stack as absent where its call has no box"}   | ${inModal("<Wrapped />")}                                                                                                 | ${"no rows"}              | ${HOSTED_STACK}
    ${"reads a same-file margin under the box its call is in"}  | ${HOSTED}                                                                                                                 | ${"doubled"}              | ${HOSTED_MARGIN}
    ${"reads that margin as absent where its call has no box"}  | ${inModal("<Wrapped />")}                                                                                                 | ${"no rows"}              | ${HOSTED_MARGIN}
    ${"judges a same-file component under each call's boxes"}   | ${inModal('<Wrapped /><div className="f"><Wrapped /></div>')}                                                             | ${"doubled"}              | ${HOSTED_STACK}
    ${"catches a margin on a same-file root in a form stack"}   | ${inModal('<div className="form-stack"><Refusals /></div>')}                                                              | ${"doubled"}              | ${`${FORM_STACK} ${MARGIN}`}
    ${"ends at a component that renders itself"}                | ${inModal("<Nest />")}                                                                                                    | ${"doubled"}              | ${`${STACK} ${MARGIN}`}
  `("$spec", ({ jsx, verdict, sheet }: RowCase) => {
    expect(plantedRows(jsx, sheet ? ownersIn([sheet]) : owners())).toBe(
      verdict,
    );
  });

  it(
    "spaces every field row in the tree",
    () => {
      const loose = tree().rows.filter((r) => r.verdict !== "spaced");
      expect(
        loose.map((r) => `${r.where} ${r.verdict}${r.by ? ` by ${r.by}` : ""}`),
        `flush: ${hints.flush}; doubled: ${hints.doubled}; unplaced: ${hints.unplaced}`,
      ).toEqual([]);
    },
    TREE_BUILD,
  );
});
