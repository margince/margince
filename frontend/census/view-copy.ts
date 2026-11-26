// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What an MCP app view shows in English, read for the mcp-view-copy rule.

import { dirname, join, resolve } from "node:path";
import { type Document, Element, type Node, Window } from "happy-dom";
import ts from "typescript";
import { declaredValues, rulesOfText } from "../scripts/lib/css-rules";
import { type CensusFile, srcDir } from "./tree";

export type Copy = Readonly<{ at: string; text: string }>;

// A JSON file a view imports carries copy this reader does not see, unless it
// is the list of tools a view may call.
const TOOL_LIST = join(srcDir, "mcp-apps", "actions.json");

const COMPARISONS = new Set([
  ts.SyntaxKind.EqualsEqualsEqualsToken,
  ts.SyntaxKind.ExclamationEqualsEqualsToken,
  ts.SyntaxKind.EqualsEqualsToken,
  ts.SyntaxKind.ExclamationEqualsToken,
]);

const PAGE_ATTRIBUTE = /^(content|alt|title|placeholder|aria-.+)$/;
const NOT_READ = new Set(["SCRIPT", "STYLE"]);
const QUOTED = /"((?:[^"\\]|\\.)*)"|'((?:[^'\\]|\\.)*)'/g;

const collapsed = (text: string) => text.replace(/\s+/g, " ").trim();

let sharedWindow: Window | undefined;

function parsed(html: string): Document {
  const window = sharedWindow ?? new Window();
  sharedWindow = window;
  return new window.DOMParser().parseFromString(html, "text/html");
}

function decoded(text: string): string {
  return text.includes("&") ? (parsed(text).body.textContent ?? "") : text;
}

// A literal compared against, matched by a case, or written as a type is a
// wire value, which keeps the spelling the wire has.
function isWireValue(node: ts.Node): boolean {
  const parent = node.parent;
  return (
    (ts.isBinaryExpression(parent) &&
      COMPARISONS.has(parent.operatorToken.kind)) ||
    (ts.isCaseClause(parent) && parent.expression === node) ||
    ts.isLiteralTypeNode(parent) ||
    ts.isImportDeclaration(parent) ||
    ts.isExportDeclaration(parent) ||
    ts.isExternalModuleReference(parent)
  );
}

function templateText(node: ts.TemplateExpression): string {
  return (
    node.head.text +
    node.templateSpans.map((span) => `{}${span.literal.text}`).join("")
  );
}

function stringText(node: ts.Node): string | undefined {
  if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) {
    return ts.isJsxAttribute(node.parent) ? decoded(node.text) : node.text;
  }
  if (ts.isTemplateExpression(node)) return templateText(node);
  if (ts.isParenthesizedExpression(node)) return stringText(node.expression);
  return undefined;
}

const isPlus = (node: ts.Node): node is ts.BinaryExpression =>
  ts.isBinaryExpression(node) &&
  node.operatorToken.kind === ts.SyntaxKind.PlusToken;

function plusParts(grouped: ts.Expression): ts.Expression[] {
  let node = grouped;
  while (ts.isParenthesizedExpression(node)) node = node.expression;
  return isPlus(node)
    ? [...plusParts(node.left), ...plusParts(node.right)]
    : [node];
}

function moduleCopy(file: CensusFile, source: ts.SourceFile): Copy[] {
  const found: Copy[] = [];
  const record = (node: ts.Node, text: string) => {
    const { line, character } = source.getLineAndCharacterOfPosition(
      node.getStart(source),
    );
    found.push({ at: `${file.rel}:${line + 1}:${character + 1}`, text });
  };
  const visit = (node: ts.Node): void => {
    if (isPlus(node) && !isPlus(node.parent)) {
      const parts = plusParts(node);
      const texts = parts.map(stringText);
      if (texts.some((text) => text !== undefined)) {
        record(node, texts.map((text) => text ?? "{}").join(""));
        parts.filter((_, i) => texts[i] === undefined).forEach(visit);
        return;
      }
    }
    if (ts.isJsxText(node)) {
      const text = collapsed(decoded(node.text));
      if (text !== "") record(node, text);
      return;
    }
    const text = ts.isParenthesizedExpression(node)
      ? undefined
      : stringText(node);
    if (text !== undefined) {
      if (!isWireValue(node)) record(node, text);
      if (!ts.isTemplateExpression(node)) return;
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

export function jsonImports(file: CensusFile): string[] {
  return (file.source?.statements ?? []).flatMap((statement) =>
    ts.isImportDeclaration(statement) &&
    ts.isStringLiteral(statement.moduleSpecifier) &&
    statement.moduleSpecifier.text.endsWith(".json") &&
    resolve(dirname(file.path), statement.moduleSpecifier.text) !== TOOL_LIST
      ? [`${file.rel} imports ${statement.moduleSpecifier.text}`]
      : [],
  );
}

function pageCopy(file: CensusFile): Copy[] {
  const found: Copy[] = [];
  const visit = (node: Node): void => {
    const parent = node.parentElement;
    if (node.nodeType === node.TEXT_NODE && parent) {
      const text = collapsed(node.textContent ?? "");
      if (text !== "" && !NOT_READ.has(parent.tagName)) {
        found.push({ at: `${file.rel} <${parent.localName}>`, text });
      }
    }
    if (node instanceof Element) {
      for (const attribute of node.attributes) {
        if (PAGE_ATTRIBUTE.test(attribute.name)) {
          found.push({
            at: `${file.rel} <${node.localName} ${attribute.name}>`,
            text: collapsed(attribute.value ?? ""),
          });
        }
      }
    }
    node.childNodes.forEach(visit);
  };
  visit(parsed(file.text));
  return found;
}

function sheetCopy(file: CensusFile): Copy[] {
  return rulesOfText(file.path, file.text).flatMap((rule) =>
    declaredValues(rule.body, "content").flatMap((value) =>
      [...value.matchAll(QUOTED)]
        .map((match) => match[1] ?? match[2] ?? "")
        .filter((text) => text !== "")
        .map((text) => ({ at: `${file.rel} ${rule.selector}`, text })),
    ),
  );
}

export function copyOf(file: CensusFile): Copy[] {
  if (file.source) return moduleCopy(file, file.source);
  if (file.path.endsWith(".html")) return pageCopy(file);
  if (file.path.endsWith(".css")) return sheetCopy(file);
  return [];
}
