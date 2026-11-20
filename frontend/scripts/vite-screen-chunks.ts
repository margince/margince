// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import type { HtmlTagDescriptor, Plugin, Rolldown } from "vite";

export const SCREEN_CHUNKS_ID = "screen-chunks";

/** What the shell carries: every file once, and each screen's files by index. */
export type ScreenChunks = {
  files: string[];
  screens: Record<string, number[]>;
};

type TypeScript = typeof import("typescript");

function entry(
  ts: TypeScript,
  property: import("typescript").ObjectLiteralElementLike,
): [string, string] {
  const body =
    ts.isPropertyAssignment(property) &&
    ts.isArrowFunction(property.initializer)
      ? property.initializer.body
      : undefined;
  const specifier =
    body &&
    ts.isCallExpression(body) &&
    body.expression.kind === ts.SyntaxKind.ImportKeyword
      ? body.arguments[0]
      : undefined;
  if (
    !property.name ||
    !ts.isIdentifier(property.name) ||
    !specifier ||
    !ts.isStringLiteral(specifier)
  )
    throw new Error(
      `SCREEN_CODE has an entry that is not \`key: () => import("…")\`: ${property.getText()}`,
    );
  return [property.name.text, specifier.text];
}

export async function screenCodeSpecifiers(
  source: string,
): Promise<Map<string, string>> {
  const { default: ts } = await import("typescript");
  const file = ts.createSourceFile(
    "screencode.ts",
    source,
    ts.ScriptTarget.Latest,
    true,
  );
  const specifiers = new Map<string, string>();
  const visit = (node: import("typescript").Node) => {
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      node.name.text === "SCREEN_CODE" &&
      node.initializer
    ) {
      const literal = ts.isAsExpression(node.initializer)
        ? node.initializer.expression
        : node.initializer;
      if (!ts.isObjectLiteralExpression(literal))
        throw new Error("SCREEN_CODE is not an object literal");
      for (const property of literal.properties)
        specifiers.set(...entry(ts, property));
    }
    ts.forEachChild(node, visit);
  };
  visit(file);
  if (specifiers.size === 0) throw new Error("no SCREEN_CODE entry was read");
  return specifiers;
}

/** A chunk and every chunk it imports statically: what loading it fetches. */
function closure(
  bundle: Rolldown.OutputBundle,
  start: Rolldown.OutputChunk,
): string[] {
  const files: string[] = [];
  const walk = (chunk: Rolldown.OutputChunk) => {
    if (files.includes(chunk.fileName)) return;
    files.push(chunk.fileName);
    for (const imported of chunk.imports) {
      const next = bundle[imported];
      if (next?.type === "chunk") walk(next);
    }
  };
  walk(start);
  return files;
}

export function screenChunksTag(map: ScreenChunks): HtmlTagDescriptor {
  return {
    tag: "script",
    attrs: { type: "application/json", id: SCREEN_CHUNKS_ID },
    // A data block, so no browser runs it and script-src does not apply.
    children: JSON.stringify(map).replace(/</g, "\\u003c"),
    injectTo: "head",
  };
}

/** Each screen's files, written into the shell so the app can preload a screen without running it. */
export function screenChunks(screenCodeFile: string): Plugin {
  const modules = new Map<string, string>();
  let base = "/";
  return {
    name: "screen-chunks",
    apply: "build",
    configResolved(config) {
      base = config.base;
    },
    async buildEnd(error) {
      if (error) return;
      modules.clear();
      const specifiers = await screenCodeSpecifiers(
        readFileSync(screenCodeFile, "utf8"),
      );
      for (const [key, specifier] of specifiers) {
        const resolved = await this.resolve(specifier, screenCodeFile);
        if (!resolved)
          this.error(
            `SCREEN_CODE.${key} names ${specifier}, which does not resolve`,
          );
        modules.set(key, resolved.id);
      }
    },
    transformIndexHtml: {
      order: "post",
      handler(_html, context) {
        const { bundle } = context;
        if (!bundle) return;
        const chunks = Object.values(bundle).filter(
          (output): output is Rolldown.OutputChunk => output.type === "chunk",
        );
        const map: ScreenChunks = { files: [], screens: {} };
        for (const [key, id] of modules) {
          const holder = chunks.find((chunk) =>
            Object.hasOwn(chunk.modules, id),
          );
          if (!holder)
            throw new Error(`no chunk holds SCREEN_CODE.${key} (${id})`);
          map.screens[key] = closure(bundle, holder).map((file) => {
            const href = `${base}${file}`;
            if (!map.files.includes(href)) map.files.push(href);
            return map.files.indexOf(href);
          });
        }
        return [screenChunksTag(map)];
      },
    },
  };
}
