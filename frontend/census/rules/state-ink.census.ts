// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A state's base is tuned to be seen. Text drawn in a state colour takes its
// `--<state>Text`, the member tokens.test.ts measures at 4.5:1 on every ground.

import { relative } from "node:path";
import ts from "typescript";
import {
  alternativesOf,
  classesOf,
  colorValues,
  declaredValues,
  type Rule,
  splitTopLevel,
  subjectOf,
} from "../../scripts/lib/css-rules";
import { classNameLiterals } from "../../scripts/lib/source-tree";
import { states } from "../../src/design-system/tokens-testing";
import {
  type CensusFile,
  type CensusRule,
  frontendRoot,
  sheetRules,
} from "../tree";

const BASE_INK = new RegExp(`var\\(\\s*--(?:${states.join("|")})\\s*[,)]`);

// SVG geometry paints a shape and is exempt; SVG text is read like any other.
const GEOMETRY =
  /^(?:svg|path|circle|ellipse|rect|line|polyline|polygon|use|g)(?![\w-])/i;
const SVG_TEXT = /^(?:text|tspan|textPath)(?![\w-])/i;

export type TextClasses = Readonly<{
  exact: Set<string>;
  prefixes: string[];
  // Where a class could not be read: an unread class is text nobody checked.
  unread: string[];
}>;

// Whether every class an expression can yield is spelled in it. That holds for
// a literal, or a template whose expressions only ever finish a class its text
// began.
function readable(node: ts.Expression): boolean {
  if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) {
    return true;
  }
  if (ts.isParenthesizedExpression(node)) return readable(node.expression);
  if (ts.isConditionalExpression(node)) {
    return readable(node.whenTrue) && readable(node.whenFalse);
  }
  if (!ts.isTemplateExpression(node)) return false;
  const pieces = [
    node.head.text,
    ...node.templateSpans.map((s) => s.literal.text),
  ];
  return node.templateSpans.every(
    (_, at) => /\S$/.test(pieces[at]) && !/^\S/.test(pieces[at + 1]),
  );
}

// SVG text built outside JSX has classes this census cannot read.
function createsSvgText(call: ts.CallExpression): boolean {
  const callee = ts.isPropertyAccessExpression(call.expression)
    ? call.expression.name.text
    : call.expression.getText();
  return (
    /^createElement(NS)?$/.test(callee) &&
    call.arguments.some(
      (argument) =>
        ts.isStringLiteralLike(argument) &&
        /^(?:text|tspan|textPath)$/i.test(argument.text),
    )
  );
}

// The classes the app puts on SVG text, read off the TSX.
export function svgTextClasses(
  sources: readonly Readonly<{ path: string; source: ts.SourceFile }>[],
): TextClasses {
  const exact = new Set<string>();
  const prefixes: string[] = [];
  const unread: string[] = [];
  for (const { path, source } of sources) {
    const visit = (node: ts.Node) => {
      if (
        ts.isJsxOpeningLikeElement(node) &&
        SVG_TEXT.test(node.tagName.getText(source))
      ) {
        const where = `${relative(frontendRoot, path)}:${source.getLineAndCharacterOfPosition(node.getStart()).line + 1}`;
        const props = node.attributes.properties;
        if (props.some(ts.isJsxSpreadAttribute)) unread.push(`${where} spread`);
        const className = props
          .filter(ts.isJsxAttribute)
          .find((attribute) => attribute.name.getText(source) === "className");
        const value = className?.initializer;
        if (
          value !== undefined &&
          ts.isJsxExpression(value) &&
          (value.expression === undefined || !readable(value.expression))
        ) {
          unread.push(`${where} ${value.getText(source)}`);
        }
        for (const literal of classNameLiterals(className)) {
          for (const token of literal.split(/\s+/).filter(Boolean)) {
            if (token.endsWith("-")) prefixes.push(token);
            else exact.add(token);
          }
        }
      }
      if (ts.isCallExpression(node) && createsSvgText(node)) {
        unread.push(
          `${relative(frontendRoot, path)}:${source.getLineAndCharacterOfPosition(node.getStart()).line + 1} ${node.getText(source)}`,
        );
      }
      ts.forEachChild(node, visit);
    };
    visit(source);
  }
  return { exact, prefixes, unread };
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

export function baseInked(all: readonly Rule[], text: TextClasses): Finding[] {
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

const isSheet = (file: CensusFile) => file.rel.endsWith(".css");
const isAppModule = (file: CensusFile) =>
  /\.tsx?$/.test(file.rel) &&
  !/\.(test|stories|testkit)\.tsx?$|\.d\.ts$/.test(file.rel);

const TEXT_INK = new RegExp(`var\\(--(?:${states.join("|")})Text\\)`);

const sourcesOf = (corpus: readonly CensusFile[]) =>
  corpus.flatMap((file) =>
    file.source && isAppModule(file)
      ? [{ path: file.path, source: file.source }]
      : [],
  );

export const rule: CensusRule = {
  name: "state-ink",
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        (file.area === "src" || file.area === "extensions") &&
        (isSheet(file) || isAppModule(file)),
    ),
  floors: (corpus) => {
    const sheets = corpus.filter(isSheet);
    const text = svgTextClasses(sourcesOf(corpus));
    const textInks = sheetRules(sheets).filter((one) =>
      colorValues(one.body).some((value) => TEXT_INK.test(value)),
    );
    return [
      ...(sheets.length > 200 ? [] : ["fewer than 200 stylesheets were read"]),
      ...(textInks.length > 70
        ? []
        : ["fewer than 70 rules letter text in a state's Text token"]),
      ...(text.exact.size + text.prefixes.length > 3
        ? []
        : ["fewer than 4 SVG text classes were read"]),
      ...(text.exact.has("rmap-name") ? [] : ["rmap-name was not read"]),
      ...(text.prefixes.includes("rmap-pill-")
        ? []
        : ["the rmap-pill- prefix was not read"]),
    ];
  },
  across: (corpus) => {
    const text = svgTextClasses(sourcesOf(corpus));
    return [
      ...text.unread.map((where) => `${where}: SVG text class not read`),
      ...baseInked(sheetRules(corpus.filter(isSheet)), text).map(
        ({ at, value }) => `${at} { ${value} }`,
      ),
    ];
  },
  advice:
    "Letter text in a state colour with the state's Text token " +
    "(var(--dangerText), not var(--danger)). Give SVG text a class this " +
    "census can read: a literal, or a template that only finishes a prefix.",
};
