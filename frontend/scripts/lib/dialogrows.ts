// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import ts from "typescript";
import { isField, isPrimitiveField, keyIs } from "./dialogfields";
import {
  attr,
  boxProps,
  childrenHosts,
  childrenSlot,
  contextOf,
  cross,
  defOf,
  descendants,
  elementsIn,
  expanded,
  flowOf,
  initOf,
  isDialogTag,
  isZero,
  keyOf,
  modalBoxes,
  type Owners,
  opening,
  type Placed,
  type Prop,
  primitiveKey,
  rootsOf,
  upper,
  type Values,
  variantsOf,
  whereOf,
  within,
} from "./dialoglayout";
import {
  childOfJsx,
  climb,
  enclosingIn,
  hostedAt,
  NO_LINKS,
  type Placement,
  parentOf,
  placementIn,
  unplacedAt,
  type View,
  viewAt,
  walk,
} from "./dialogplacement";
import { type Handed, handedAt, handedThrough } from "./dialogprops";

// A dialog's rows sit one stack step apart: none flush, none doubled by its own
// margin. Stacks and margins are read from the sheets in each box's context.

type Verdict = "spaced" | "flush" | "doubled" | "unplaced";
type Row = { at: string; where: string; verdict: Verdict; by?: string };
// The class lists of the boxes from an element's parent out to its dialog.
type Chain = string[][];
// A component on the way to a call, under the boxes and props it entered with.
type Entry = { def: ts.Node; key: string };
type Guest = {
  def: ts.Node;
  outer: Chain[];
  via: readonly Entry[];
  handed: Handed;
  key: string;
};
type Keys = { keys: ReadonlySet<string>; modal: string };
type Gate = Keys & { owners: Owners; outer: Chain[]; host?: Handed };

const HEADING = primitiveKey("heading", "Heading");
const DIALOG: Chain[] = [[["modal"]]];

// A component that renders a dialog around its children.
const wraps = ({ keys }: Pick<Keys, "keys">, n: ts.Node) =>
  isDialogTag(n, keys);
// `role="dialog"` alone makes any element a dialog box.
const isDialog = (g: Pick<Keys, "keys">, n: ts.Node) =>
  literal(n, "role") === "dialog" || wraps(g, n);
export const dialogsIn = (source: ts.SourceFile, keys: ReadonlySet<string>) =>
  elementsIn(source).filter((n) => isDialog({ keys }, n));
const inForce = (g: Gate, n: ts.Node) =>
  g.host && within(n, g.host.def) ? g.host : undefined;
function literal(n: ts.Node, name: string) {
  const a = attr(n, name);
  const e = a && initOf(a);
  return e && ts.isStringLiteral(e) ? e.text : undefined;
}

// A component counts by the field rows it renders at its own root.
function width(g: Gate, n: ts.Node) {
  if (isField(n, inForce(g, n)?.given)) return 1;
  const def = defOf(n);
  if (!def || wraps(g, n)) return 0;
  const { given } = handedAt(n, def, inForce(g, n));
  return rootsOf(def).filter((r) => isField(r, given)).length;
}

const valuesOf = (e: ts.Node, name: string, outer?: Handed): Values => {
  const def = defOf(e);
  return (def && handedThrough(e, def, name, outer)) ?? "any";
};
// The box classes a dialog element draws. Modal's are those of each value its
// props can take; a wrapper's, those of the dialog it renders around them.
function dialogBoxes(
  g: Gate,
  e: ts.Node,
  outer?: Handed,
  depth = 0,
): string[][] {
  if (keyIs(e, g.modal)) {
    const given = boxProps().map((p): [string, Values] => [
      p,
      valuesOf(e, p, outer),
    ]);
    return modalBoxes(new Map(given));
  }
  const def = defOf(e);
  const inner = def && childrenHosts(def).find((h) => isDialog(g, h));
  if (!def || !inner || depth > 4) {
    throw new Error(
      `${whereOf(e)}: no Modal found around this dialog's children; render the wrapper's Modal around {children}`,
    );
  }
  return dialogBoxes(g, inner, handedAt(e, def, outer), depth + 1);
}
// A component's classes land on the root holding its children; a dialog's
// box stands outside that root unless it is that root.
function boxesAt(g: Gate, p: ts.Node): Chain[] {
  const modal = keyIs(p, g.modal);
  const slot = upper(p) && !modal ? childrenSlot(p) : undefined;
  const def = defOf(p);
  const handed = slot && def && handedAt(p, def, inForce(g, p));
  const inner = slot && handed ? variantsOf(slot, handed.given) : [[]];
  const own = cross(variantsOf(p, inForce(g, p)?.given), inner, p);
  if (!wraps(g, p)) return own.map((cls) => [cls]);
  const boxes = dialogBoxes(g, p, inForce(g, p));
  if (modal) return cross(own, boxes, p).map((cls) => [cls]);
  return own.flatMap((cls) => boxes.map((box) => [cls, box]));
}
function ancestry(g: Gate, v: View, n: ts.Node): Chain[] {
  let chains: Chain[] = [[]];
  for (let s = parentOf(v, n); s; s = parentOf(s.v, s.at)) {
    const p = s.at;
    chains = cross(chains, contextOf(g.owners, boxesAt(g, p)), p);
    if (isDialog(g, p)) return chains;
  }
  return cross(chains, g.outer, n);
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
// What a row's own box can be: each branch of its classes. A component's is
// its root elements' classes joined with the ones it is handed.
function boxesOf(
  g: Gate,
  n: ts.Node,
  hops = 3,
  scope = inForce(g, n),
): string[][] {
  const own = variantsOf(n, scope?.given).flatMap((v) =>
    expanded(g.owners, v, n),
  );
  const def = defOf(n);
  if (!def || hops === 0 || wraps(g, n)) return own;
  const roots = rootsOf(def);
  // With no root to read (`const Line = inline ? "span" : "p"`), the call's
  // own classes stand for its box.
  if (roots.length === 0) return own;
  const handed = handedAt(n, def, scope);
  return roots
    .flatMap((r) => boxesOf(g, r, hops - 1, handed))
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
const has = (g: Gate, n: ts.Node | undefined, set: Set<string>) =>
  !!n &&
  opening(n) !== undefined &&
  variantsOf(n, inForce(g, n)?.given).every((v) => v.some((c) => set.has(c)));
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
  const overhead =
    !prev || has(g, prev, g.owners.band) || has(g, n, g.owners.top);
  const underfoot =
    !next || has(g, next, g.owners.top) || has(g, n, g.owners.band);
  return overhead && underfoot ? "spaced" : "flush";
}

const addRendered = (inside: Set<ts.Node>, v: View, r: ts.Node) => {
  for (const e of [r, ...walk(v, r)].filter(opening)) inside.add(e);
};
function addPlaced(g: Gate, placement: Placement, inside: Set<ts.Node>) {
  const plain = { placement, links: NO_LINKS };
  for (const [r, links] of placement.frameOf) {
    const v = { placement, links };
    const inDialog = [...climb(v, r)].some((s) => isDialog(g, s.at));
    if (inDialog) addRendered(inside, v, r);
  }
  for (const [r, call] of placement.unplaced) {
    const def = defOf(call);
    if (def && elementsIn(def).some((e) => isDialog(g, e))) {
      addRendered(inside, plain, r);
    }
  }
}
// The gate follows a component into itself once more, and only under boxes or
// props it was not entered with. So a recursion is judged one level down.
const enters = (via: readonly Entry[], def: ts.Node, key: string) => {
  const before = via.filter((e) => e.def === def);
  return before.length < 2 && before.every((e) => e.key !== key);
};
function guestsIn(
  g: Gate,
  placement: Placement,
  inside: Set<ts.Node>,
  hosted?: Guest,
) {
  const via = hosted
    ? [...hosted.via, { def: hosted.def, key: hosted.key }]
    : [];
  const guests: Guest[] = [];
  for (const n of inside) {
    const def = defOf(n);
    if (!def || wraps(g, n) || isPrimitiveField(n)) continue;
    // An unplaced prop is not known to render inside the boxes `g.outer` names.
    const at = unplacedAt(placement, n) ? { ...g, outer: DIALOG } : g;
    const outer = ancestry(at, viewAt(placement, n), n);
    const handed = handedAt(n, def, inForce(g, n));
    const values = handed.keyed.map((p) => [p, handed.props(p) ?? null]);
    const key = JSON.stringify([outer, values]);
    if (!enters(via, def, key)) continue;
    guests.push({ def, outer, via, handed, key });
  }
  return guests;
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
  for (const r of hosted ? [hosted.def] : dialogs)
    addRendered(inside, plain, r);
  if (!hosted) addPlaced(g, placement, inside);
  const guests = guestsIn(g, placement, inside, hosted);
  return { dialogs: dialogs.length, inside, guests };
}

export const hints: Record<Exclude<Verdict, "spaced">, string> = {
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

// Well above the tree's own count: a fan-out fails before it is slow.
export const SCOPE_CAP = 15_000;
const fanOut = (q: Guest) =>
  new Error(
    `${whereOf(q.via[0]?.def ?? q.def)}: more than ${SCOPE_CAP} guest scopes to judge in the components this one calls, last at ${whereOf(q.def)}; call them under fewer distinct boxes or props, or raise SCOPE_CAP in scripts/lib/dialogrows.ts once the tree's own count has grown`,
  );
// A component a dialog renders is judged as part of that dialog under each
// chain of boxes it is called in. So is what it renders in turn.
export function rowsAcross(
  entry: readonly ts.SourceFile[],
  owners: Owners,
  keys: Keys,
) {
  const base: Gate = { ...keys, owners, outer: DIALOG };
  const rows = new Map<string, Row>();
  const seen = new Set<string>();
  const judged = new Set<ts.Node>();
  let dialogs = 0;
  let depth = 0;
  const queue: Guest[] = [];
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
    for (const q of queue.splice(0)) {
      const key = `${keyOf(q.def)}#${q.key}`;
      if (seen.has(key)) continue;
      seen.add(key);
      if (seen.size > SCOPE_CAP) throw fanOut(q);
      judged.add(q.def);
      const g = { ...base, outer: q.outer, host: q.handed };
      take(rowsIn(g, q.def.getSourceFile(), q));
    }
  }
  const scopes = seen.size;
  return { rows: [...rows.values()], dialogs, depth, scopes, judged };
}
