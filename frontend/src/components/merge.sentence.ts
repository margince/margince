// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { useT } from "../i18n";

// Apart from merge.tsx, so the Worklist's pair, drawn on Home, names both
// sides without bringing the Merge dialog into Home's chunk.

export type MergeSide = Readonly<{ name: string; detail?: string }>;

// Each side reads as its name and its detail line. So the question before a
// merge, and the answer after it, tell apart two records that share a name.
function mergeSideLabel(t: ReturnType<typeof useT>, side: MergeSide): string {
  return side.detail
    ? t("merge.recordDetail", { name: side.name, detail: side.detail })
    : side.name;
}

// The question asked before a merge, or the answer after it.
export function mergeSentence(
  t: ReturnType<typeof useT>,
  key: "merge.confirm" | "merge.done",
  source: MergeSide,
  target: MergeSide,
): string {
  return t(key, {
    source: mergeSideLabel(t, source),
    target: mergeSideLabel(t, target),
  });
}
