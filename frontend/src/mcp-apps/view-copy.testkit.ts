import { readFileSync } from "node:fs";
import { dirname, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import {
  filesMatching,
  MODULE_FILE,
  sourceFileAt,
} from "../../scripts/lib/source-tree";

// The views ship English outside the i18n catalog, so their copy is read off
// the source, the pages' titles and labels included.

const viewsDir = dirname(fileURLToPath(import.meta.url));

// Tests, fixtures and Storybook hosts are not drawn by a shipped view.
const NOT_SHIPPED =
  /\.(test|stories|testkit)\.tsx?$|\/fixture\.ts$|\/story-hosts\.tsx$/;

const PAGE_TITLE = /<title>([^<]*)<\/title>/g;
const PAGE_LABEL = /\b(aria-label|alt|placeholder|title)="([^"]*)"/g;

/** viewCopy maps where each string stands, `mcp-apps/<file>:<line>:<column>`,
 *  to its text. A template's substitutions read as `{}`, as a placeholder. */
export function viewCopy(dir = viewsDir): Record<string, string> {
  const copy: Record<string, string> = {};
  const at = (path: string) =>
    `mcp-apps/${relative(dir, path).split(sep).join("/")}`;
  for (const path of filesMatching(dir, MODULE_FILE)) {
    if (NOT_SHIPPED.test(path.split(sep).join("/"))) continue;
    const source = sourceFileAt(path);
    const visit = (node: ts.Node): void => {
      const text = literalText(node);
      if (text !== undefined) {
        const { line, character } = source.getLineAndCharacterOfPosition(
          node.getStart(source),
        );
        copy[`${at(path)}:${line + 1}:${character + 1}`] = text;
      }
      ts.forEachChild(node, visit);
    };
    visit(source);
  }
  for (const path of filesMatching(dir, /\.html$/)) {
    const page = readFileSync(path, "utf8");
    for (const title of page.matchAll(PAGE_TITLE)) {
      copy[`${at(path)}:<title>`] = title[1] ?? "";
    }
    for (const label of page.matchAll(PAGE_LABEL)) {
      const line = page.slice(0, label.index).split("\n").length;
      copy[`${at(path)}:${line}:${label[1]}`] = label[2] ?? "";
    }
  }
  return copy;
}

function literalText(node: ts.Node): string | undefined {
  if (ts.isJsxText(node)) {
    const text = node.text.trim();
    return text === "" ? undefined : text;
  }
  if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) {
    const module =
      ts.isImportDeclaration(node.parent) ||
      ts.isExportDeclaration(node.parent);
    return module ? undefined : node.text;
  }
  if (ts.isTemplateExpression(node)) {
    return (
      node.head.text +
      node.templateSpans.map((span) => `{}${span.literal.text}`).join("")
    );
  }
  return undefined;
}
