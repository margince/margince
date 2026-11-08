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
