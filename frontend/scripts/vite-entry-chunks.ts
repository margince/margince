// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import type { Plugin, Rolldown } from "vite";

export function initialEntries(root: string) {
  return {
    bootstrap: [join(root, "src/bootstrap.ts")],
    startup: [join(root, "src/main.tsx")],
    analyticsPreparation: [
      join(root, "src/app/analytics-prefetch.ts"),
      join(root, "src/app/queryclient.instance.ts"),
    ],
    analytics: [join(root, "src/App.analytics.tsx")],
    home: [join(root, "src/features/brief/brief.tsx")],
  };
}

export function entryChunks(
  name: string,
  entries: readonly string[],
  priority = 10,
): {
  plugin: Plugin;
  group: Rolldown.CodeSplittingGroup;
} {
  const modules = new Set<string>();
  return {
    plugin: {
      name: `entry-chunks:${name}`,
      apply: "build",
      buildStart() {
        modules.clear();
      },
      buildEnd(error) {
        if (error) return;
        const pending = [...entries];
        while (pending.length > 0) {
          const id = pending.pop();
          if (id === undefined || modules.has(id)) continue;
          modules.add(id);
          pending.push(...(this.getModuleInfo(id)?.importedIds ?? []));
        }
      },
    },
    group: {
      name,
      test: (id) => modules.has(id),
      includeDependenciesRecursively: false,
      priority,
    },
  };
}

export function preloadEntries(entries: readonly string[]): Plugin {
  let base = "/";
  return {
    name: "preload-initial-entries",
    configResolved(config) {
      base = config.base;
    },
    apply: "build",
    transformIndexHtml: {
      order: "post",
      handler(_html, context) {
        if (!context.bundle) return;
        return Object.values(context.bundle).flatMap((output) => {
          if (
            output.type !== "chunk" ||
            !entries.some((entry) => Object.hasOwn(output.modules, entry))
          )
            return [];
          return [
            {
              tag: "link",
              attrs: {
                rel: "modulepreload",
                crossorigin: "",
                href: `${base}${output.fileName}`,
              },
              injectTo: "head" as const,
            },
          ];
        });
      },
    },
  };
}

/**
 * What the build refuses: a budgeted chunk over its bytes, and a budget that
 * names no chunk or several. A renamed chunk would otherwise pass unchecked.
 */
export function chunksOverBudget(
  chunks: readonly { name: string; bytes: number }[],
  budgets: Readonly<Record<string, number>>,
): string[] {
  return Object.entries(budgets).flatMap(([name, budget]) => {
    const named = chunks.filter((chunk) => chunk.name === name);
    if (named.length !== 1)
      return [`the ${name} budget names ${named.length} chunks, not one`];
    const { bytes } = named[0];
    return bytes > budget
      ? [`${name} is ${bytes} bytes, over its ${budget}-byte budget`]
      : [];
  });
}

/**
 * Fails the build when a named chunk outgrows its budget. A cold load on Home
 * waits for Home's chunk. Code Home opens on demand belongs in a lazy import.
 */
export function chunkBudgets(
  budgets: Readonly<Record<string, number>>,
): Plugin {
  // A test build bundles React's development code, so only a production build
  // measures what a reader downloads.
  let production = true;
  return {
    name: "chunk-budgets",
    apply: "build",
    configResolved(config) {
      production = config.isProduction;
    },
    generateBundle(_options, bundle) {
      if (!production) return;
      const over = chunksOverBudget(
        Object.values(bundle).flatMap((output) =>
          output.type === "chunk"
            ? [{ name: output.name, bytes: Buffer.byteLength(output.code) }]
            : [],
        ),
        budgets,
      );
      if (over.length > 0) this.error(over.join("; "));
    },
  };
}
