// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A layer stacked under a pane shows through it unless the pane's host paints
// the ground first. Read from every sheet the bundle ships.

import {
  alternativesOf,
  compounds,
  type Rule,
  rulesOfText,
  splitTopLevel,
} from "../../scripts/lib/css-rules";
import { UNDERLAY_HOSTS } from "../../scripts/lib/surface-ladder";
import {
  ALIAS,
  type Declaration,
  declarations,
  flatFill,
  IMAGE,
  ladderPath,
  MEMBERS,
  OPAQUE,
  PSEUDO_ELEMENT,
  STATE,
  within,
  written,
} from "../../scripts/lib/surface-sheets";
import { themes } from "../../src/design-system/tokens-testing";
import { byCodeUnit } from "../../src/lib/codeunitorder";
import type { CensusFile, CensusRule } from "../tree";

function customValues(rules: readonly Rule[]): Map<string, string[]> {
  const values = new Map<string, string[]>();
  for (const rule of rules) {
    for (const { property, value } of declarations(rule.body)) {
      if (property.startsWith("--")) {
        values.set(property, [...(values.get(property) ?? []), value]);
      }
    }
  }
  return values;
}

// Each value a var() may take, from every sheet that declares the property.
function expanded(
  value: string,
  values: ReadonlyMap<string, readonly string[]>,
  depth = 0,
): string[] {
  const reference = /var\((--[\w-]+)(?:,\s*([^()]*))?\)/.exec(value);
  if (reference === null) return [value];
  const [whole, name = "", fallback] = reference;
  const options = [
    ...(values.get(name) ?? []),
    ...(fallback ? [fallback] : []),
  ];
  if (options.length === 0 || depth > 8) {
    throw new Error(`${value} reads ${name}, which no sheet declares`);
  }
  return options.flatMap((option) =>
    expanded(value.replace(whole, option), values, depth + 1),
  );
}

function belowZero(expression: string): boolean {
  const factors = expression.replace(/calc\(|[()\s]/g, "").split(/[*/]/);
  return (
    factors.every((factor) => /^-?[\d.]+$/.test(factor)) &&
    factors.filter((factor) => factor.startsWith("-")).length % 2 === 1
  );
}

const paintsAGradient = ({ property, value }: Declaration) =>
  /^background(?:-image)?$/.test(property) &&
  splitTopLevel(value, ",").some((layer) =>
    splitTopLevel(layer, " ").some(
      (word) => IMAGE.test(word) && flatFill(word) === undefined,
    ),
  );

// A pseudo-element lies under its host's content, an element under its parent's, and a gradient under its own element's.
export function underlayHosts(rules: readonly Rule[]): string[] {
  const values = customValues(rules);
  const outside = rules.filter((rule) => rule.file !== ladderPath);
  const hostOf = (layer: string) => {
    const host = layer.replace(STATE, "");
    const holder = host.replace(new RegExp(PSEUDO_ELEMENT, "g"), "");
    return holder === host ? `:has(> ${host})` : holder;
  };
  const stacked = outside
    .filter((rule) =>
      declarations(rule.body).some(
        ({ property, value }) =>
          property === "z-index" && expanded(value, values).some(belowZero),
      ),
    )
    .flatMap((rule) => alternativesOf(rule.selector).map(hostOf));
  const painted = outside
    .filter((rule) => declarations(rule.body).some(paintsAGradient))
    .flatMap((rule) => alternativesOf(rule.selector))
    .filter((alternative) => {
      const self = written(alternative);
      return (
        !PSEUDO_ELEMENT.test(compounds(alternative).at(-1) ?? "") &&
        !MEMBERS.some(
          (member) => member.own.size > 0 && within(member.own, self.own),
        )
      );
    })
    .map((alternative) => alternative.replace(STATE, ""));
  return [...new Set([...stacked, ...painted])].sort(byCodeUnit);
}

/** Each knockout fill that is not opaque in some theme, or the lack of any. */
export function knockoutFills(rules: readonly Rule[]): string[] {
  const layered = rules.map((rule) => ({
    selector: rule.selector,
    layers: declarations(rule.body)
      .filter(({ property }) => property.startsWith("background"))
      .flatMap(({ value }) => splitTopLevel(value, ",")),
  }));
  const knockouts = layered.filter(({ layers }) =>
    ["padding-box", "border-box"].every((box) => layers.join().includes(box)),
  );
  if (knockouts.length === 0) return ["no knockout background was read"];
  return knockouts.flatMap(({ selector, layers }) => {
    const fills = layers.flatMap((layer) =>
      splitTopLevel(layer, " ").flatMap((word) => flatFill(word) ?? []),
    );
    if (fills.length === 0) return [`${selector}: no flat fill`];
    return fills
      .filter((fill) => fill !== "var(--ground)")
      .flatMap((fill) =>
        Object.entries(themes)
          .filter(
            ([, palette]) =>
              !OPAQUE.test(palette[ALIAS.exec(fill)?.[1] ?? fill] ?? ""),
          )
          .map(([theme]) => `${selector}: ${fill} in ${theme}`),
      );
  });
}

// auth-* belongs to auth-core.css, and AmbientWaves on that surface hosts its panes.
const ELSEWHERE = new Map([[".auth-surface", ":has(> .ambient-waves)"]]);

/** Where the derived hosts and UNDERLAY_HOSTS disagree. */
export function hostFindings(derived: readonly string[]): string[] {
  const declared = new Set<string>(UNDERLAY_HOSTS);
  return [
    ...derived
      .filter((host) => !ELSEWHERE.has(host) && !declared.has(host))
      .map((host) => `${host} stacks a layer and is not in UNDERLAY_HOSTS`),
    ...UNDERLAY_HOSTS.filter((host) => !derived.includes(host)).map(
      (host) => `${host} is in UNDERLAY_HOSTS and stacks no layer`,
    ),
    ...[...ELSEWHERE].flatMap(([host, by]) => [
      ...(derived.includes(host) ? [] : [`${host} no longer stacks a layer`]),
      ...(declared.has(by) ? [] : [`${by} is not in UNDERLAY_HOSTS`]),
    ]),
  ];
}

const shipped = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  file.rel.endsWith(".css");

export const rule: CensusRule = {
  name: "surfaces.layers",
  corpus: (tree) => tree.files.filter(shipped),
  floors: (corpus) => [
    ...(corpus.length > 250 ? [] : ["fewer than 250 stylesheets were read"]),
    ...(corpus.some((file) => file.path === ladderPath)
      ? []
      : ["surfaces.css was not read"]),
  ],
  across: (corpus) => {
    const rules = corpus.flatMap((file) => rulesOfText(file.path, file.text));
    return [...hostFindings(underlayHosts(rules)), ...knockoutFills(rules)];
  },
  advice:
    "A layer stacked under a pane needs its host in UNDERLAY_HOSTS " +
    "(scripts/lib/surface-ladder.ts), so the host paints the ground first. " +
    "A knockout background fills with a token that is opaque in both themes.",
};
