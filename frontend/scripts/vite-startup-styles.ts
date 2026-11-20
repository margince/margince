// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { HtmlTagDescriptor, Plugin, Rolldown } from "vite";

type Bundle = Rolldown.OutputBundle;

/** A chunk and every chunk it imports statically, imports first: what loading it fetches. */
export function staticChunks(
  bundle: Bundle,
  chunk: Rolldown.OutputChunk,
): Rolldown.OutputChunk[] {
  const seen = new Set<string>();
  const found: Rolldown.OutputChunk[] = [];
  const walk = (current: Rolldown.OutputChunk) => {
    if (seen.has(current.fileName)) return;
    seen.add(current.fileName);
    for (const imported of current.imports) {
      const next = bundle[imported];
      if (next?.type === "chunk") walk(next);
    }
    found.push(current);
  };
  walk(chunk);
  return found;
}

/** The CSS those chunks carry, in the cascade order Vite links an entry's. */
export function stylesheetsOf(
  bundle: Bundle,
  chunk: Rolldown.OutputChunk,
): string[] {
  return [
    ...new Set(
      staticChunks(bundle, chunk).flatMap((current) => [
        ...(current.viteMetadata?.importedCss ?? []),
      ]),
    ),
  ];
}

function startupSheets(bundle: Bundle, entries: readonly string[]): string[] {
  const chunks = Object.values(bundle).filter(
    (output): output is Rolldown.OutputChunk =>
      output.type === "chunk" &&
      entries.some((entry) => Object.hasOwn(output.modules, entry)),
  );
  if (chunks.length === 0)
    throw new Error(`no chunk holds the startup entries ${entries.join(", ")}`);
  return [...new Set(chunks.flatMap((chunk) => stylesheetsOf(bundle, chunk)))];
}

function fontFile(
  bundle: Bundle,
  sheets: readonly string[],
  name: string,
): string {
  const asset = Object.values(bundle).find(
    (output) => output.type === "asset" && output.names.includes(name),
  );
  if (!asset) throw new Error(`the build emitted no font ${name} to preload`);
  const named = sheets.some((sheet) => {
    const output = bundle[sheet];
    return (
      output?.type === "asset" && String(output.source).includes(asset.fileName)
    );
  });
  // A preload no startup stylesheet asks for is fetched twice or never used.
  if (!named)
    throw new Error(
      `no startup stylesheet declares the preloaded font ${name}`,
    );
  return asset.fileName;
}

/** Linked in the shell, the startup stylesheets load with the page, not after bootstrap imports them. */
export function startupStyles(
  entries: readonly string[],
  preloadFonts: readonly string[],
): Plugin {
  let base = "/";
  return {
    name: "startup-styles",
    apply: "build",
    configResolved(config) {
      base = config.base;
    },
    transformIndexHtml: {
      order: "post",
      handler(_html, context): HtmlTagDescriptor[] | undefined {
        const { bundle } = context;
        if (!bundle) return;
        const sheets = startupSheets(bundle, entries);
        const fonts = preloadFonts.map((name) =>
          fontFile(bundle, sheets, name),
        );
        return [
          ...sheets.map((sheet) => ({
            tag: "link",
            attrs: {
              rel: "stylesheet",
              crossorigin: "",
              href: `${base}${sheet}`,
            },
            injectTo: "head" as const,
          })),
          ...fonts.map((font) => ({
            tag: "link",
            attrs: {
              rel: "preload",
              as: "font",
              type: "font/woff2",
              crossorigin: "",
              href: `${base}${font}`,
            },
            injectTo: "head" as const,
          })),
        ];
      },
    },
  };
}
