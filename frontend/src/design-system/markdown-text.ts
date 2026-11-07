// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  collapseSpace,
  firstRunText,
  runsOf,
  runText,
} from "./markdown-highlight";
import { readMarkdown } from "./markdown-parse";

/** A body's first paragraph as one line of plain text, for a row's detail. */
export function markdownFirstLine(source: string): string {
  return firstRunText(readMarkdown(source).blocks);
}

/** Every line a reader sees in the rendered body, as plain text. */
export function markdownLines(source: string): string[] {
  return runsOf(readMarkdown(source).blocks)
    .flatMap((run) => runText(run.nodes).split("\n"))
    .map(collapseSpace)
    .filter(Boolean);
}
