// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A control drawn smaller than a thumb gets its press target on touch from a
// `::before` layer that draws nothing: `X::before` inside `@media (pointer:
// coarse)`, with `X { position: relative }` in the same block. Every such
// layer is at least TOUCH_TARGET_PX on each axis it fixes.

import { declaredValues, splitTopLevel } from "../../scripts/lib/css-rules";
import { type CssRule, rulesIn, TOUCH_TARGET_PX } from "../../src/testing/css";
import { type CensusFile, type CensusRule, isAppSheet } from "../tree";

const COARSE = "@media (pointer: coarse)";

export type TouchLayer = { control: string; rel: string; body: string };

const coarse = (rule: CssRule) => rule.conditions.includes(COARSE);
const blockOf = (rule: CssRule) => rule.conditions.join(" ");

/** Every touch layer the sheets declare. */
export function touchLayers(sheets: readonly CensusFile[]): TouchLayer[] {
  return sheets.flatMap((sheet) => {
    const rules = rulesIn(sheet.text);
    return rules.filter(coarse).flatMap((layer) => {
      const control = /^(.+)::before$/.exec(layer.selector)?.[1];
      const holds = rules.some(
        (rule) =>
          coarse(rule) &&
          blockOf(rule) === blockOf(layer) &&
          rule.selector === control &&
          declaredValues(rule.body, "position").includes("relative"),
      );
      return control !== undefined && holds
        ? [{ control, rel: sheet.rel, body: layer.body }]
        : [];
    });
  });
}

type Properties = ReadonlyMap<string, string>;

// Every custom property the sheets declare, the first declaration kept.
function propertiesOf(sheets: readonly CensusFile[]): Properties {
  const found = new Map<string, string>();
  for (const rule of sheets.flatMap((sheet) => rulesIn(sheet.text))) {
    for (const [, name, value] of rule.body.matchAll(
      /(--[\w-]+)\s*:\s*([^;]+)/g,
    )) {
      if (!found.has(name)) found.set(name, value.trim());
    }
  }
  return found;
}

/**
 * A CSS length in px, from numbers, px, %, + - * /, calc(), max(), min() and
 * var(). A `%` is a share of `side`, the control's own size on that axis.
 * Anything else reads as NaN, which no floor passes.
 */
export function lengthOf(
  text: string,
  side: number,
  properties: Properties,
): number {
  const tokens =
    text.match(/\d*\.?\d+(?:px|%)?|--[\w-]+|[a-z]+\(|[()+*/,-]/g) ?? [];
  let at = 0;
  const next = () => tokens[at++] ?? "";
  const peek = () => tokens[at] ?? "";
  const args = (): number[] => {
    const out = [sum()];
    while (peek() === ",") {
      next();
      out.push(sum());
    }
    next();
    return out;
  };
  const atom = (): number => {
    const token = next();
    if (token === "(" || token === "calc(") return args()[0] ?? Number.NaN;
    if (token === "max(") return Math.max(...args());
    if (token === "min(") return Math.min(...args());
    if (token === "var(") {
      const value = properties.get(next());
      next();
      return value === undefined
        ? Number.NaN
        : lengthOf(value, side, properties);
    }
    if (token === "-") return -atom();
    if (token.endsWith("%")) return (Number.parseFloat(token) * side) / 100;
    return Number.parseFloat(token);
  };
  const product = (): number => {
    let value = atom();
    while (peek() === "*" || peek() === "/") {
      value = next() === "*" ? value * atom() : value / atom();
    }
    return value;
  };
  function sum(): number {
    let value = product();
    while (peek() === "+" || peek() === "-") {
      value = next() === "+" ? value + product() : value - product();
    }
    return value;
  }
  return sum();
}

// One axis of a layer: its own size, or the two insets that leave its box.
// The `inset` shorthand runs top, right, bottom, left.
function axisOf(body: string, axis: "block" | "inline") {
  const one = (property: string) => declaredValues(body, property)[0];
  const parts = (property: string) => splitTopLevel(one(property) ?? "", " \n");
  const [top, right = top, bottom = top, left = right] = parts("inset");
  const pair = parts(`inset-${axis}`);
  const block = axis === "block";
  return {
    size: one(`${axis}-size`),
    start:
      one(`inset-${axis}-start`) ??
      pair[0] ??
      one(block ? "top" : "left") ??
      (block ? top : left),
    end:
      one(`inset-${axis}-end`) ??
      pair[1] ??
      pair[0] ??
      one(block ? "bottom" : "right") ??
      (block ? bottom : right),
  };
}

/** The layer's size on one axis, over a control `side` px long on it. */
export function reach(
  body: string,
  axis: "block" | "inline",
  side: number,
  properties: Properties,
): number {
  const { size, start, end } = axisOf(body, axis);
  if (size !== undefined) return lengthOf(size, side, properties);
  if (start === undefined || end === undefined) return Number.NaN;
  return (
    side - lengthOf(start, side, properties) - lengthOf(end, side, properties)
  );
}

// The height is read over a control of --controlHeight, the height every
// control stands at. A width that grows with the control is its own width plus
// a reach, which a sheet cannot measure, so only a fixed width is held.
function shortfall(layer: TouchLayer, properties: Properties): string[] {
  const height = lengthOf("var(--controlHeight)", 0, properties);
  const tall = reach(layer.body, "block", height, properties);
  const wide = reach(layer.body, "inline", 0, properties);
  const fixedWidth = wide === reach(layer.body, "inline", height, properties);
  const where = `${layer.rel} ${layer.control}::before`;
  const said = (px: number, measure: string) =>
    Number.isNaN(px)
      ? `${where} has a ${measure} the census cannot read`
      : `${where} is ${px}px ${measure === "height" ? "tall" : "wide"}`;
  return [
    ...(tall >= TOUCH_TARGET_PX ? [] : [said(tall, "height")]),
    ...(!fixedWidth || wide >= TOUCH_TARGET_PX ? [] : [said(wide, "width")]),
  ];
}

export const rule: CensusRule = {
  name: "touchtargets",
  corpus: (tree) => tree.files.filter(isAppSheet),
  floors: (corpus) => {
    const found = touchLayers(corpus).map((layer) => layer.control);
    return [
      ...(found.length >= 5 ? [] : ["fewer than 5 touch layers were read"]),
      ...[".lt-select", ".info-tip"]
        .filter((control) => !found.includes(control))
        .map((control) => `the touch layer of ${control} was not read`),
    ];
  },
  across: (corpus) => {
    const properties = propertiesOf(corpus);
    return touchLayers(corpus).flatMap((layer) => shortfall(layer, properties));
  },
  advice: `A touch layer under a coarse pointer is at least ${TOUCH_TARGET_PX}px on each axis it fixes.`,
};
