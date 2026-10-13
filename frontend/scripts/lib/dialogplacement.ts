// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Where each element renders: JSX held in a variable, returned by a call or
// handed to a prop, placed at the site that renders it.

import ts from "typescript";
import {
  callOf,
  declarations,
  defOf,
  descendants,
  fnOf,
  givenTo,
  handedJsx,
  jsxOf,
  rendered,
  returnedBy,
  upper,
  whereOf,
} from "./dialoglayout";

// A component definition, mapped to the call that rendered it this time.
export type Links = Map<ts.Node, ts.Node>;
type Slot = { roots: ts.Node[]; links: Links };
// Rows rendered away from where they are written: a JSX variable or call
// (`inline`), and JSX handed to a prop (`slots`), placed where it renders.
export type Placement = {
  inline: Map<ts.Node, ts.Node[]>;
  slots: Map<ts.Node, Slot[]>;
  siteOf: Map<ts.Node, ts.Node>;
  frameOf: Map<ts.Node, Links>;
  unplaced: Map<ts.Node, ts.Node>;
};
export type View = { placement: Placement; links: Links };
export const NO_LINKS: Links = new Map();
export const childOfJsx = (n: ts.Node) =>
  !!n.parent && (ts.isJsxElement(n.parent) || ts.isJsxFragment(n.parent));
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
export function placementIn(source: ts.SourceFile): Placement {
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
export function* climb(
  v: View,
  n: ts.Node,
): Generator<{ at: ts.Node; v: View }> {
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
export function parentOf(v: View, n: ts.Node) {
  for (const step of climb(v, n)) if (ts.isJsxElement(step.at)) return step;
  return undefined;
}
export const enclosingIn = (v: View, n: ts.Node) => parentOf(v, n)?.at;
export const hostedAt = (v: View, n: ts.Node) => [
  ...(v.placement.inline.get(n) ?? []),
  ...(v.placement.slots.get(n) ?? [])
    .filter((s) => s.links === v.links)
    .flatMap((s) => s.roots),
];
// Everything under `root` as rendered, what an expression hosts included.
export function walk(v: View, root: ts.Node): ts.Node[] {
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
export function viewAt(placement: Placement, n: ts.Node): View {
  for (let p: ts.Node | undefined = n; p; p = p.parent) {
    const links = placement.frameOf.get(p);
    if (links) return { placement, links };
  }
  return { placement, links: NO_LINKS };
}
export function unplacedAt(placement: Placement, n: ts.Node) {
  for (let p: ts.Node | undefined = n; p; p = p.parent) {
    if (placement.unplaced.has(p)) return true;
  }
  return false;
}
