// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What a call hands a component's props: the values each prop can take,
// read through the spreads and the callers that hand them on.

import ts from "typescript";
import { classVariants, type Given } from "../../src/testing/classnames";
import {
  attr,
  attrs,
  callOf,
  descendants,
  domainOf,
  initOf,
  literalsIn,
  type Member,
  opening,
  presetOf,
  propReadBy,
  spreads,
  upper,
} from "./dialoglayout";

// The object literals a spread can hand, through parentheses and conditions.
const spreadArms = (x: ts.Expression): ts.ObjectLiteralExpression[] => {
  if (ts.isParenthesizedExpression(x)) return spreadArms(x.expression);
  if (ts.isObjectLiteralExpression(x)) return [x];
  if (!ts.isConditionalExpression(x)) return [];
  const [a, b] = [spreadArms(x.whenTrue), spreadArms(x.whenFalse)];
  return a.length > 0 && b.length > 0 ? [...a, ...b] : [];
};
const memberKey = (m: Member) => {
  const key = m.name;
  if (!key || ts.isComputedPropertyName(key)) return undefined;
  return ts.isJsxNamespacedName(key) ? key.getText() : key.text;
};
type Reading = { values: ts.Expression[]; open: boolean } | "any";
/** How a spread the caller can read hands a prop on: always, or maybe. */
export type Forwarded = (spread: ts.Expression) => "set" | "maybe" | undefined;
// What one member hands `name`; `open` when it may leave the prop unset.
function memberReading(m: Member, name: string, fwd?: Forwarded): Reading {
  if (ts.isJsxSpreadAttribute(m) || ts.isSpreadAssignment(m)) {
    const how = fwd?.(m.expression);
    if (how) return { values: [m.expression], open: how === "maybe" };
    const arms = spreadArms(m.expression).map((o) =>
      lastNamed(o.properties, name, fwd),
    );
    const read = arms.filter((r) => r !== "any");
    if (arms.length === 0 || read.length < arms.length) return "any";
    const values = read.flatMap((r) => r.values);
    return { values, open: read.some((r) => r.open) };
  }
  const key = memberKey(m);
  if (key === undefined) return "any";
  if (key !== name) return { values: [], open: true };
  const x = initOf(m);
  return x ? { values: [x], open: false } : "any";
}
// The last member naming `name` wins, as JSX and object spreads apply in
// source order.
function lastNamed(
  members: readonly Member[],
  name: string,
  fwd?: Forwarded,
): Reading {
  const values: ts.Expression[] = [];
  for (const m of [...members].reverse()) {
    const read = memberReading(m, name, fwd);
    if (read === "any") return "any";
    values.push(...read.values);
    if (!read.open) return { values, open: false };
  }
  return { values, open: true };
}
/** Every value a call may hand prop `name`, `undefined` for a path that hands
 * none; "any" when a bare attribute or an unreadable spread hides the value.
 * A spread `fwd` reads stands as its own value. */
export function handedTo(
  e: ts.Node,
  name: string,
  fwd?: Forwarded,
): (ts.Expression | undefined)[] | "any" {
  const read = lastNamed(opening(e)?.attributes.properties ?? [], name, fwd);
  if (read === "any") return "any";
  return read.open ? [...read.values, undefined] : read.values;
}
type Named = (spread: ts.Expression) => readonly string[];
function memberNames(m: Member, fwd: Named): readonly string[] {
  if (!ts.isJsxSpreadAttribute(m) && !ts.isSpreadAssignment(m)) {
    const key = memberKey(m);
    return key === undefined ? [] : [key];
  }
  const arms = spreadArms(m.expression);
  if (arms.length === 0) return fwd(m.expression);
  return arms.flatMap((o) => o.properties.flatMap((p) => memberNames(p, fwd)));
}
/** The prop names a call writes: its attributes and the keys of the object
 * literals it spreads. `fwd` names what a spread it cannot read hands on. */
export function namesHanded(e: ts.Node, fwd: Named = () => []): string[] {
  const members = opening(e)?.attributes.properties ?? [];
  return [...new Set(members.flatMap((m) => memberNames(m, fwd)))];
}
export type Held = readonly (string | undefined)[] | undefined;
type Props = (name: string) => Held;
// The props one call hands a component, by name and as its class lists read
// them. `keyed` names the props whose values tell two calls apart.
export type Handed = {
  def: ts.Node;
  props: Props;
  given: Given;
  keyed: string[];
};
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
export function handedThrough(
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
export function handedAt(at: ts.Node, def: ts.Node, outer?: Handed): Handed {
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
