// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  alternativesOf,
  appStylesheets,
  classesOf,
  colorValues,
  declaredValues,
  type Rule,
  rulesOf,
  selectorList,
  splitTopLevel,
  subjectOf,
} from "../../scripts/lib/css-rules";
import {
  classNameLiterals,
  extensionFrontendFiles,
  filesMatching,
  sourceFileAt,
} from "../../scripts/lib/source-tree";
import { states } from "./tokens-testing";

// A state's base is tuned to be SEEN. Text drawn in a state colour takes its
// `--<state>Text`, the member tokens.test.ts measures at 4.5:1 on every ground.

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");

const BASE_INK = new RegExp(`var\\(\\s*--(?:${states.join("|")})\\s*[,)]`);

// SVG geometry paints a shape and is exempt; SVG text is read like any other.
const GEOMETRY =
  /^(?:svg|path|circle|ellipse|rect|line|polyline|polygon|use|g)(?![\w-])/i;
const SVG_TEXT = /^(?:text|tspan)(?![\w-])/i;

type TextClasses = Readonly<{ exact: Set<string>; prefixes: string[] }>;

// The classes the app puts on an SVG `<text>` or `<tspan>`, read off the TSX.
function svgTextClasses(): TextClasses {
  const exact = new Set<string>();
  const prefixes: string[] = [];
  const modules = [
    ...filesMatching(join(frontendRoot, "src"), /\.tsx$/),
    ...extensionFrontendFiles(join(frontendRoot, "..", "extensions")).filter(
      (path) => path.endsWith(".tsx"),
    ),
  ].filter((path) => !/\.(test|stories|testkit)\.tsx$/.test(path));
  for (const path of modules) {
    const source = sourceFileAt(path);
    const visit = (node: ts.Node) => {
      if (
        ts.isJsxOpeningLikeElement(node) &&
        SVG_TEXT.test(node.tagName.getText(source))
      ) {
        const className = node.attributes.properties
          .filter(ts.isJsxAttribute)
          .find((attribute) => attribute.name.getText(source) === "className");
        for (const literal of classNameLiterals(className)) {
          for (const token of literal.split(/\s+/).filter(Boolean)) {
            if (token.endsWith("-")) prefixes.push(token);
            else exact.add(token);
          }
        }
      }
      ts.forEachChild(node, visit);
    };
    visit(source);
  }
  return { exact, prefixes };
}

function paintsText(subject: string, text: TextClasses): boolean {
  if (SVG_TEXT.test(splitTopLevel(subject, ":")[0] ?? "")) return true;
  return [...classesOf(subject)].some(
    (name) =>
      text.exact.has(name) ||
      text.prefixes.some((prefix) => name.startsWith(prefix)),
  );
}

type Finding = Readonly<{ at: string; value: string }>;

function baseInked(all: readonly Rule[], text: TextClasses): Finding[] {
  return all.flatMap((rule) =>
    alternativesOf(rule.selector).flatMap((selector) => {
      const subject = subjectOf(selector);
      const svgText = paintsText(subject, text);
      const letters = [
        ...colorValues(rule.body),
        ...(svgText
          ? [
              ...declaredValues(rule.body, "fill"),
              ...declaredValues(rule.body, "-webkit-text-fill-color"),
            ]
          : []),
      ].filter((value) => BASE_INK.test(value));
      if (letters.length === 0) return [];
      const head = splitTopLevel(subject, ":")[0] ?? "";
      if (!svgText && GEOMETRY.test(head)) return [];
      const at = `${relative(frontendRoot, rule.file)} ${selector}`;
      return letters.map((value) => ({ at, value }));
    }),
  );
}

function probe(selector: string, body: string): Rule[] {
  return selectorList(selector).map((one) => ({
    file: join(frontendRoot, "probe.css"),
    selector: one,
    body,
  }));
}

describe("text in a state colour", () => {
  const sheets = appStylesheets(frontendRoot);
  const all = rulesOf(sheets);
  const text = svgTextClasses();

  it("reads the SVG text classes off the components that draw them", () => {
    expect(text.exact.size + text.prefixes.length).toBeGreaterThan(3);
    expect(text.exact.has("rmap-name")).toBe(true);
    expect(text.prefixes).toContain("rmap-pill-");
  });

  it("is lettered in the state's Text token, in every sheet the app ships", () => {
    expect(sheets.length).toBeGreaterThan(200);
    const textInks = all.filter((rule) =>
      colorValues(rule.body).some((value) =>
        new RegExp(`var\\(--(?:${states.join("|")})Text\\)`).test(value),
      ),
    );
    expect(textInks.length).toBeGreaterThan(70);
    expect(
      baseInked(all, text).map(({ at, value }) => `${at} { ${value} }`),
    ).toEqual([]);
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
