// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What an MCP app view shows in English, read for the mcp-view-copy rule.

import { basename } from "node:path";
import { JSDOM } from "jsdom";
import ts from "typescript";
import { declaredValues, rulesOfText } from "../scripts/lib/css-rules";
import type { CensusFile } from "./tree";

export type Copy = Readonly<{ at: string; text: string }>;

// A JSON file a view imports carries copy this reader does not see, unless it
// is the list of tools a view may call.
const JSON_WITHOUT_COPY = new Set(["actions.json"]);

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

let entityDocument: Document | undefined;

function decoded(text: string): string {
  if (!text.includes("&")) return text;
  entityDocument ??= new JSDOM("").window.document;
  const holder = entityDocument.createElement("template");
  holder.innerHTML = text;
  return holder.content.textContent ?? "";
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

function plusParts(node: ts.Expression): ts.Expression[] {
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
    !JSON_WITHOUT_COPY.has(basename(statement.moduleSpecifier.text))
      ? [`${file.rel} imports ${statement.moduleSpecifier.text}`]
      : [],
  );
}

function pageCopy(file: CensusFile): Copy[] {
  const dom = new JSDOM(file.text, { includeNodeLocations: true });
  const { document, Node, NodeFilter } = dom.window;
  const lineOf = (node: globalThis.Node) =>
    dom.nodeLocation(node)?.startLine ?? 0;
  const found: Copy[] = [];
  const walker = document.createTreeWalker(
    document,
    NodeFilter.SHOW_ELEMENT | NodeFilter.SHOW_TEXT,
  );
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    const parent = node.parentElement;
    if (node.nodeType === Node.TEXT_NODE) {
      const text = collapsed(node.textContent ?? "");
      if (text !== "" && parent && !NOT_READ.has(parent.tagName)) {
        const tag = parent.localName;
        found.push({ at: `${file.rel}:${lineOf(node)} <${tag}>`, text });
      }
      continue;
    }
    if (!(node instanceof dom.window.Element)) continue;
    const element = node;
    for (const attribute of element.attributes) {
      if (PAGE_ATTRIBUTE.test(attribute.name)) {
        found.push({
          at: `${file.rel}:${lineOf(element)} <${element.localName} ${attribute.name}>`,
          text: collapsed(attribute.value),
        });
      }
    }
  }
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
