// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import ts from "typescript";
import { classVariants, type Given } from "../../src/testing/classnames";
import {
  attr,
  attrs,
  boxProps,
  callOf,
  capped,
  childrenHosts,
  childrenSlot,
  cross,
  declarations,
  definition,
  defOf,
  descendants,
  domainOf,
  elementsIn,
  enclosing,
  expanded,
  type Forwarded,
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
  modalBoxes,
  namesHanded,
  type Owners,
  opening,
  type Placed,
  type Prop,
  presetOf,
  primitiveKey,
  propReadBy,
  rendered,
  returnedBy,
  rootsOf,
  spreads,
  tag,
  upper,
  type Values,
  whereOf,
  within,
} from "./dialoglayout";

// A dialog's rows sit one stack step apart: none flush, none doubled by its own
// margin. Stacks and margins are read from the sheets in each box's context.

type Verdict = "spaced" | "flush" | "doubled" | "unplaced";
type Row = { at: string; where: string; verdict: Verdict; by?: string };
// The class lists of the boxes from an element's parent out to its dialog.
type Chain = string[][];
// What a call hands the props of the component it calls: by name, and as
// the reader its body's class lists take.
type Held = readonly (string | undefined)[] | undefined;
type Props = (name: string) => Held;
type Handed = { def: ts.Node; props: Props; given: Given; keyed: string[] };
// A component on the way to a call, under the boxes and props it entered with.
type Entry = { def: ts.Node; key: string };
type Guest = {
  def: ts.Node;
  outer: Chain[];
  via: readonly Entry[];
  handed: Handed;
  key: string;
};
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
type Gate = Keys & { owners: Owners; outer: Chain[]; host?: Handed };

export const FIELD_ROWS = new Map([
  ["Field", "atoms"],
  ["Checkbox", "atoms"],
  ["Radio", "atoms"],
  ["ChoiceList", "choicelist"],
]);
// Design-system controls that draw their own label and are not form rows.
export const NOT_ROWS = new Map([
  ["InlineText", "edits a value in place, its label hidden"],
  ["ChipValueList", "picks a list filter's values"],
]);
const rowKeys = [...FIELD_ROWS].map(([name, file]) => primitiveKey(file, name));
const HEADING = primitiveKey("heading", "Heading");
const NO_LINKS: Links = new Map();
const DIALOG: Chain[] = [[["modal"]]];

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
export const dialogsIn = (source: ts.SourceFile, keys: ReadonlySet<string>) =>
  elementsIn(source).filter((n) => isDialog({ keys }, n));
const variantsOf = (n: ts.Node, given?: Given) =>
  classVariants(
    n.getSourceFile(),
    attr(n, "className")?.initializer,
    16,
    given,
  );
// What one expression hands a prop: its literals, or what the caller is handed
// for the props the expression reads or spreads.
function handedValue(
  x: ts.Expression,
  name: string,
  def: ts.Node,
  outer?: Handed,
): Held {
  const listed = literalsIn(x);
  if (listed || !outer) return listed;
  const preset = presetOf(def, name);
  const reads = propReadBy(outer.def);
  const forwarded = spreads(outer.def, x, name);
  const read = forwarded ? name : reads(x);
  if (read !== undefined) {
    const values = outer.props(read);
    const set = forwarded ? values?.filter((v) => v !== undefined) : values;
    return set?.map((v) => v ?? preset);
  }
  if (!descendants(x).some((d) => reads(d) !== undefined)) return undefined;
  return classVariants(x.getSourceFile(), x, 16, outer.given).map((v) =>
    v.join(" "),
  );
}
// `<Card inset />` hands `true`; a spread beside it could hand anything.
const bare = (at: ts.Node, name: string) => {
  const a = attr(at, name);
  const all = opening(at)?.attributes.properties.length ?? 0;
  return !!a && !a.initializer && attrs(at).length === all;
};
// What a call hands one prop: its literal branches, the caller's own value
// where it hands one on, or the default for one left out.
function handedThrough(
  at: ts.Node,
  def: ts.Node,
  name: string,
  outer?: Handed,
): Held {
  const forwarded: Forwarded | undefined =
    outer &&
    ((x) => {
      if (!spreads(outer.def, x, name)) return undefined;
      const values = outer.props(name);
      return !values || values.includes(undefined) ? "maybe" : "set";
    });
  const handed = handedTo(at, name, forwarded);
  if (handed === "any") return bare(at, name) ? ["true"] : undefined;
  const each = handed.map((x) =>
    x ? handedValue(x, name, def, outer) : [presetOf(def, name)],
  );
  if (each.some((v) => v === undefined)) return undefined;
  return each.flatMap((v) => v ?? []);
}
const propAt = (at: ts.Node, def: ts.Node, name: string, outer?: Handed) => {
  const domain = domainOf(def, name);
  return (
    handedThrough(at, def, name, outer) ?? (domain && [...domain, undefined])
  );
};
const classReads = new WeakMap<ts.Node, string[]>();
function classProps(def: ts.Node): string[] {
  const known = classReads.get(def);
  if (known) return known;
  const reads = propReadBy(def);
  const decides = (n: ts.Node) =>
    ts.isJsxSpreadAttribute(n) ||
    ts.isVariableDeclaration(n) ||
    (ts.isJsxAttribute(n) &&
      (n.name.getText() === "className" || upper(callOf(n))));
  const names = new Set<string>();
  for (const n of descendants(def)) {
    const name = reads(n);
    if (name === undefined) continue;
    for (let p = n.parent; p && p !== def; p = p.parent) {
      if (decides(p)) names.add(name);
    }
  }
  const out = [...names].sort();
  classReads.set(def, out);
  return out;
}
const forwardsAll = (def: ts.Node) =>
  descendants(def).some(
    (n) => ts.isJsxSpreadAttribute(n) && spreads(def, n.expression),
  );
function handedAt(at: ts.Node, def: ts.Node, outer?: Handed): Handed {
  const known = new Map<string, Held>();
  const props: Props = (name) => {
    if (!known.has(name)) known.set(name, propAt(at, def, name, outer));
    return known.get(name);
  };
  const reads = propReadBy(def);
  const given: Given = (n) => {
    const name = reads(n);
    return name === undefined ? undefined : props(name);
  };
  const handedOn = (x: ts.Expression) =>
    outer && spreads(outer.def, x) ? outer.keyed : [];
  // A body that spreads its props on may hand any of them to a class.
  const written = forwardsAll(def) ? namesHanded(at, handedOn) : [];
  const keyed = [...new Set([...classProps(def), ...written])].sort();
  return { def, props, given, keyed };
}
const inForce = (g: Gate, n: ts.Node) =>
  g.host && within(n, g.host.def) ? g.host : undefined;
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

function inlineRoots(source: ts.SourceFile, e: ts.Expression): ts.Node[] {
  const callee = ts.isCallExpression(e) ? e.expression : undefined;
  const named = ts.isIdentifier(e) ? e : callee;
  const decl = named && declarations(source).get(named.getText());
  if (!decl) return [];
  if (callee) return returnedBy(fnOf(decl));
  return ts.isVariableDeclaration(decl) ? jsxOf(decl.initializer) : [];
}
function placeInline(source: ts.SourceFile, p: Placement) {
  for (const x of descendants(source)) {
    if (!ts.isJsxExpression(x) || !x.expression || !childOfJsx(x)) continue;
    const roots = inlineRoots(source, x.expression);
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

// Ancestors as rendered, not as written. A slotted root climbs to its site
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
const isPrimitiveField = (n: ts.Node) =>
  rowKeys.some((k) => keyIs(n, k)) && !inPlace(n);
const isField = (n: ts.Node, given?: Given) =>
  isPrimitiveField(n) || variantsOf(n, given).flat().includes("form-row");
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
  // A binding that is no function (`const Line = inline ? "span" : "p"`) is
  // read as the element it names.
  if (roots.length === 0 && !ts.isFunctionLike(fnOf(def))) return own;
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
      const g = { ...base, outer: q.outer, host: q.handed };
      take(rowsIn(g, q.def.getSourceFile(), q));
    }
  }
  return { rows: [...rows.values()], dialogs, depth, scopes: seen.size };
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
export const labelledIn = (source: ts.SourceFile) =>
  [...declarations(source)]
    .filter(([name, def]) => /^[A-Z]/.test(name) && exported(def))
    .filter(([name]) => {
      const drawn = definition(source, name);
      return !!drawn && drawsLabel(drawn);
    })
    .map(([name]) => name);
