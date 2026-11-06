// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Plugin, Rolldown } from "vite";

export function entryChunks(
  name: string,
  entries: readonly string[],
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
      priority: 10,
    },
  };
}
