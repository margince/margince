import { describe, expect, it } from "vitest";
import {
  alternativesOf,
  compounds,
  type Rule,
  splitTopLevel,
} from "../../scripts/lib/css-rules";
import {
  OVERLAY_RUNG,
  PAGE_RUNGS,
  UNDERLAY_HOSTS,
} from "../../scripts/lib/surface-ladder";
import {
  ALIAS,
  allRules,
  type Declaration,
  declarations,
  flatFill,
  IMAGE,
  ladderPath,
  MEMBERS,
  OPAQUE,
  PSEUDO_ELEMENT,
  probe,
  ref,
  STATE,
  within,
  written,
} from "../../scripts/lib/surface-sheets";
import { themes } from "./tokens-testing";

// A layer stacked under a pane shows through it unless the pane's host paints
// the ground first. Read from the same sheets as surfaces.test.ts.

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
function underlayHosts(rules: readonly Rule[]): string[] {
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
  return [...new Set([...stacked, ...painted])].sort();
}

function knockoutFills(rules: readonly Rule[]): string[] {
  const layered = rules.map((rule) => ({
    selector: rule.selector,
    layers: declarations(rule.body)
      .filter(({ property }) => property.startsWith("background"))
      .flatMap(({ value }) => splitTopLevel(value, ",")),
  }));
  const knockouts = layered.filter(({ layers }) =>
    ["padding-box", "border-box"].every((box) => layers.join().includes(box)),
  );
  expect(knockouts.length).toBeGreaterThan(0);
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

describe("a layer under a pane stays under it", () => {
  it("gives every rung an opaque ground in both themes", () => {
    for (const rung of [...PAGE_RUNGS, OVERLAY_RUNG]) {
      for (const [theme, palette] of Object.entries(themes)) {
        const name = rung.ground.match(/^var\((--[\w-]+)\)$/)?.[1] ?? "";
        expect(palette[name], `${rung.ground} in ${theme}`).toMatch(OPAQUE);
      }
    }
  });

  it("reads a layer's host through a var(), a calc() and a state", () => {
    expect(
      underlayHosts([
        probe(":root", "--under: -1"),
        probe(".f::before:hover", "z-index: -1"),
        probe(".g", `z-index: ${ref("--under")}`),
        probe(".h::after", "z-index: calc(-1 * 2)"),
        probe(".k:hover", "background: radial-gradient(red, blue)"),
        probe(".flat", "background: linear-gradient(red, red)"),
        probe(".up::before", "z-index: calc(2 - 1)"),
        probe(".m::after:focus-visible", "z-index: -1"),
        probe(".n:focus-within", "background: radial-gradient(red, blue)"),
      ]),
    ).toEqual([".f", ".h", ".k", ".m", ".n", ":has(> .g)"]);
  });

  it("paints the ground of a pane on every host the tree stacks a layer in", () => {
    // auth-* belongs to auth.css, and AmbientWaves on that surface hosts its panes.
    const elsewhere = new Map([[".auth-surface", ":has(> .ambient-waves)"]]);
    const derived = underlayHosts(allRules);
    expect(derived.filter((host) => !elsewhere.has(host))).toEqual(
      [...UNDERLAY_HOSTS].sort(),
    );
    for (const [host, by] of elsewhere) {
      expect(derived).toContain(host);
      expect(UNDERLAY_HOSTS.some((one) => one === by)).toBe(true);
    }
  });

  it("reads an opaque fill for a layer that hides the border-box one under it", () => {
    const planted = probe(
      ".probe",
      "background: linear-gradient(var(--ground), var(--ground)) padding-box, " +
        "linear-gradient(var(--pane) , var(--pane)) padding-box, " +
        "linear-gradient(red, blue) border-box",
    );
    expect(knockoutFills([planted])).toEqual([".probe: var(--pane) in dark"]);
    expect(knockoutFills(allRules)).toEqual([]);
  });
});
