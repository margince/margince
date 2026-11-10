// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { type Rule, selectorList } from "../../scripts/lib/css-rules";
import { parseSource } from "../../scripts/lib/source-tree";
import { frontendRoot, plantedFindings, srcDir } from "../tree";
import {
  baseInked,
  rule,
  svgTextClasses,
  type TextClasses,
} from "./state-ink.census";

const planted = (text: string) => {
  const path = join(frontendRoot, "probe.tsx");
  return { path, source: parseSource(path, text) };
};

// The SVG text classes the probes below read, as the components draw them.
const text: TextClasses = {
  exact: new Set(["rmap-name"]),
  prefixes: ["rmap-pill-"],
  unread: [],
};

// A template placeholder, spelled into probe source without being one here.
const slot = (name: string) => `$${"{"}${name}}`;

function probe(selector: string, body: string): Rule[] {
  return selectorList(selector).map((one) => ({
    file: join(frontendRoot, "probe.css"),
    selector: one,
    body,
  }));
}

describe("text in a state colour", () => {
  it.each([
    ["an identifier", "<text className={label} />"],
    ["a member", "<text className={styles.label} />"],
    ["a helper call", '<tspan className={cx("a", b)} />'],
    ["a suffix template", `<textPath className={\`${slot("tone")}-label\`} />`],
    ["a whole-class expression", `<text className={\`a ${slot("tone")}\`} />`],
    ["a spread", "<text {...rest} />"],
    ["a createElement", 'createElement("text", { className: "a" })'],
    ["a createElementNS", 'document.createElementNS(svgNs, "tspan")'],
  ])("refuses to pass %s on SVG text unread", (_, markup) => {
    const probe = svgTextClasses([planted(`const x = ${markup};`)]);
    expect(probe.unread).toHaveLength(1);
  });

  it("reads a literal, a prefix template and literal branches without complaint", () => {
    const probe = svgTextClasses([
      planted(
        `const x = <><text className="a" /><text className={\`p-${slot("t")}\`} /><text className={on ? "b" : "c"} /></>;`,
      ),
    ]);
    expect(probe.unread).toEqual([]);
    expect([...probe.exact].sort()).toEqual(["a", "b", "c"]);
    expect(probe.prefixes).toEqual(["p-"]);
  });

  it.each([
    [".p", "color: var(--warning);", 1],
    [".p:hover", "color: var(--danger);", 1],
    [":where(.q) .p", "color: var(--success);", 1],
    [":is(.p, .q)", "color: var(--info);", 2],
    [".p, .q svg", "color: var(--danger);", 1],
    [".p", "color: var(--warningText); color: var(--warning);", 1],
    [".p", "color: var(--danger, #c00);", 1],
    [".p", "color: color-mix(in srgb, var(--discovery) 80%, black);", 1],
    [".p text", "fill: var(--danger);", 1],
    [".p tspan", "-webkit-text-fill-color: var(--warning);", 1],
    [".rmap-gap .rmap-name", "fill: var(--warning);", 1],
    [".rmap-pill-waiting", "fill: var(--warning);", 1],
    [".p svg", "color: var(--success);", 0],
    [".p > path:hover", "color: var(--danger);", 0],
    [".p circle", "fill: var(--success);", 0],
    [".rmap-name", "fill: var(--warningText);", 0],
    [".p", "color: var(--dangerText);", 0],
    [
      ".p",
      "background: var(--danger); border-color: var(--danger); --x-color: var(--danger);",
      0,
    ],
  ])("reads %s { %s } as %i base-inked text", (selector, body, count) => {
    expect(baseInked(probe(selector, body), text)).toHaveLength(count);
  });
});

describe("the state-ink rule", () => {
  it("reports planted findings through its own corpus and across", () => {
    expect(
      plantedFindings(rule, [
        {
          path: join(srcDir, "features", "planted.css"),
          area: "src",
          text: ".p { color: var(--warning); }\n.q { color: var(--warningText); }",
        },
        {
          path: join(srcDir, "features", "planted.tsx"),
          area: "src",
          text: "const x = <text className={label} />;",
        },
      ]),
    ).toEqual([
      "src/features/planted.tsx:1 {label}: SVG text class not read",
      "src/features/planted.css .p { var(--warning) }",
    ]);
  });
});
