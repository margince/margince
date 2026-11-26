// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { StandingSources } from "../design-system/howitstands";

type StandingLine = components["schemas"]["StandingLine"];

/**
 * The verdict's sentence once a record has a status line: what its newest
 * activity said, written when that activity landed. The line replaces the
 * sentence the page derives, and the card says a model wrote it. Without a
 * line, the page's own reading stands.
 */
export function withStandingLine<V extends { sentence?: unknown }>(
  verdict: V,
  line: StandingLine | undefined,
): Readonly<{ verdict: V; sources?: StandingSources }> {
  if (!line || line.text.trim() === "") return { verdict };
  return {
    verdict: { ...verdict, sentence: line.text },
    sources: { by: "model", count: line.activity_ids.length },
  };
}
