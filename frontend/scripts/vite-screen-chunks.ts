// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import type * as TS from "typescript";
import type { HtmlTagDescriptor, Plugin, Rolldown } from "vite";
import { SCREEN_CHUNKS_ID, type ScreenChunks } from "../src/lib/screenchunks";
import { staticChunks, stylesheetsOf } from "./vite-startup-styles";

function objectLiteral(
  ts: typeof TS,
  file: TS.SourceFile,
  name: string,
): TS.ObjectLiteralExpression | undefined {
  for (const statement of file.statements) {
    if (!ts.isVariableStatement(statement)) continue;
    for (const declaration of statement.declarationList.declarations) {
      let value = declaration.initializer;
      if (value && ts.isAsExpression(value)) value = value.expression;
      if (
        ts.isIdentifier(declaration.name) &&
        declaration.name.text === name &&
        value &&
        ts.isObjectLiteralExpression(value)
      )
        return value;
    }
  }
  return undefined;
}

function importSpecifier(
  ts: typeof TS,
  property: TS.ObjectLiteralElementLike,
): string | undefined {
  if (
    !ts.isPropertyAssignment(property) ||
    !ts.isArrowFunction(property.initializer)
  )
    return undefined;
  const { body } = property.initializer;
  if (
    !ts.isCallExpression(body) ||
    body.expression.kind !== ts.SyntaxKind.ImportKeyword
  )
    return undefined;
  const [argument] = body.arguments;
  return argument && ts.isStringLiteral(argument) ? argument.text : undefined;
}

/**
 * Each entry of the object literal `name` declares in `file`, with the module
 * its `() => import("…")` names; undefined for an entry of any other shape.
 */
export function importMapEntries(
  ts: typeof TS,
  file: TS.SourceFile,
  name: string,
): Map<string, string | undefined> | undefined {
  const literal = objectLiteral(ts, file, name);
  if (!literal) return undefined;
  const entries = new Map<string, string | undefined>();
  for (const property of literal.properties) {
    if (property.name && ts.isIdentifier(property.name))
      entries.set(property.name.text, importSpecifier(ts, property));
  }
  return entries;
}

export async function screenCodeSpecifiers(
  source: string,
): Promise<Map<string, string>> {
  const { default: ts } = await import("typescript");
  const file = ts.createSourceFile(
    "screencode.ts",
    source,
    ts.ScriptTarget.Latest,
  );
  const specifiers = new Map<string, string>();
  for (const [key, specifier] of importMapEntries(ts, file, "SCREEN_CODE") ??
    []) {
    if (specifier === undefined)
      throw new Error(`SCREEN_CODE.${key} is not \`() => import("…")\``);
    specifiers.set(key, specifier);
  }
  if (specifiers.size === 0) throw new Error("no SCREEN_CODE entry was read");
  return specifiers;
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
        const indexOf = (file: string) => {
          const href = `${base}${file}`;
          if (!map.files.includes(href)) map.files.push(href);
          return map.files.indexOf(href);
        };
        for (const [key, id] of modules) {
          const holder = chunks.find((chunk) =>
            Object.hasOwn(chunk.modules, id),
          );
          if (!holder)
            throw new Error(`no chunk holds SCREEN_CODE.${key} (${id})`);
          map.screens[key] = [
            ...staticChunks(bundle, holder).map((chunk) => chunk.fileName),
            ...stylesheetsOf(bundle, holder),
          ].map(indexOf);
        }
        return [screenChunksTag(map)];
      },
    },
  };
}
