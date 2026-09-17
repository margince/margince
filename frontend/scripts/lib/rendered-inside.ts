// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import ts from "typescript";
import {
  classesOf,
  compounds,
  inks,
  type Rule,
  subjectClasses,
} from "./css-rules";
import { parseSource } from "./source-tree";

// WHO RENDERS INSIDE WHOM, read off the components rather than off the
// selectors.
//
// A CSS-only scan has no DOM. `.token-remove` is rendered inside `.token` by
// TokenInput and TokenList, and its rule names no `token` class in any
// compound — so a subtree search over selectors alone never measured the
// remove control's ink on the chip's ground, and the scan below passed
// without having looked at it. The components are the only place that
// containment is written down, so this is where it is read from.
//
// The answer is per DESCENDANT ELEMENT, keeping the classes one element
// carries together: `<span className="pn-relay-due t-caption">` is one thing
// with two names, and a rule selecting either of them paints it. Flattening
// the two into a set of names lost that, and with it the ability to see that
// `.pn-relay-owner .pn-relay-due` is what actually inks the `.t-caption`
// rule's element.
//
// Containment is not chained ACROSS files, because a class is not a place —
// `.t-caption` renders inside a filter pill in one screen and inside a card
// in another, and treating "inside x" as transitive over class names put
// every typography class inside every chip in the tree.
//
// A JSX attribute whose value this cannot read statically — a computed
// className — contributes nothing rather than guessing, which is why the
// emptiness check at the call site is not optional.
export function renderedInside(root: string): Map<string, Set<string>[]> {
  const inside = new Map<string, Set<string>[]>();
  for (const file of componentFiles(root)) {
    const source = parseSource(file, readFileSync(file, "utf8"));
    const walk = (node: ts.Node, hosts: readonly string[]) => {
      // The classes hang off the ELEMENT, not off the attribute: an
      // attribute's own children are its value, and the children rendered
      // inside it are the element's.
      const own = classNamesOfElement(node);
      if (own.length > 0) {
        for (const host of hosts) {
          inside.set(host, [...(inside.get(host) ?? []), new Set(own)]);
        }
      }
      const below = own.length > 0 ? [...hosts, ...own] : hosts;
      ts.forEachChild(node, (child) => walk(child, below));
    };
    walk(source, []);
  }
  return inside;
}

function componentFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      return entry.name === "node_modules" || entry.name === "dist"
        ? []
        : componentFiles(path);
    }
    return entry.name.endsWith(".tsx") ? [path] : [];
  });
}

// The classes a JSX element carries, read off its opening tag.
function classNamesOfElement(node: ts.Node): string[] {
  const opening = ts.isJsxElement(node)
    ? node.openingElement
    : ts.isJsxSelfClosingElement(node)
      ? node
      : undefined;
  if (opening === undefined) return [];
  return opening.attributes.properties.flatMap((attribute) =>
    classNamesOn(attribute),
  );
}

// The classes a JSX element carries, from a literal className. A template
// literal contributes its literal spans — `` `token ${extra}` `` carries
// `token` — because the parts written down are still written down.
function classNamesOn(node: ts.Node): string[] {
  if (
    !ts.isJsxAttribute(node) ||
    !ts.isIdentifier(node.name) ||
    node.name.text !== "className"
  ) {
    return [];
  }
  const value = node.initializer;
  if (value === undefined) return [];
  const text = ts.isStringLiteral(value)
    ? value.text
    : ts.isJsxExpression(value) && value.expression !== undefined
      ? literalSpansOf(value.expression)
      : "";
  return text.split(/\s+/).filter(Boolean);
}

// The literal text of an expression, ignoring every interpolation: a computed
// class is a class this walk does not know, and inventing one would be worse
// than missing it.
function literalSpansOf(expression: ts.Expression): string {
  if (
    ts.isStringLiteral(expression) ||
    ts.isNoSubstitutionTemplateLiteral(expression)
  ) {
    return expression.text;
  }
  if (ts.isTemplateExpression(expression)) {
    return [
      expression.head.text,
      ...expression.templateSpans.map((span) => span.literal.text),
    ].join(" ");
  }
  // A conditional or a join() still writes its class names down in its
  // branches; only the interpolations are unknown.
  const spans: string[] = [];
  ts.forEachChild(expression, (child) => {
    if (ts.isExpression(child)) spans.push(literalSpansOf(child));
  });
  return spans.join(" ");
}

// landsOn answers whether a rule paints one descendant element: its subject
// must be a subset of that element's own classes, and every ancestor compound
// it names must be an element inside the same chip. Without the second half,
// `.schedule-preset .t-caption` reads as the relay chip's caption — a rule
// from a screen that chip never appears on.
export function landsOn(
  rule: Rule,
  element: Set<string>,
  siblings: readonly Set<string>[],
): boolean {
  const parts = compounds(rule.selector);
  const subject = subjectClasses(rule.selector);
  if (subject.size === 0) return false;
  if (![...subject].every((name) => element.has(name))) return false;
  return parts.slice(0, -1).every((part) => {
    const carried = [...classesOf(part)];
    if (carried.length === 0) return false;
    return siblings.some((other) => carried.every((name) => other.has(name)));
  });
}

// overriddenInside answers whether some rule sets this element's ink with the
// chip named in its own selector, which is what the cascade then shows.
export function overriddenInside(
  all: Rule[],
  chip: Rule,
  rule: Rule,
  inside: Map<string, Set<string>[]>,
): boolean {
  const host = subjectClasses(chip.selector);
  const below = [...host].flatMap((name) => inside.get(name) ?? []);
  const painted = below.filter((element) =>
    [...subjectClasses(rule.selector)].every((name) => element.has(name)),
  );
  return all.some((other) => {
    if (other === rule || other === chip) return false;
    if (inks(other.body).length === 0) return false;
    const parts = compounds(other.selector);
    const ancestors = parts
      .slice(0, -1)
      .flatMap((part) => [...classesOf(part)]);
    if (![...host].every((name) => ancestors.includes(name))) return false;
    const subject = subjectClasses(other.selector);
    return painted.some((element) =>
      [...subject].every((name) => element.has(name)),
    );
  });
}
